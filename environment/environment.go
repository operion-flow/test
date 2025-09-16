package environment

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dukex/operion/pkg/eventbus"
	"github.com/dukex/operion/pkg/eventbus/kafka"
	"github.com/dukex/operion/pkg/persistence"
	"github.com/dukex/operion/pkg/persistence/postgresql"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

// Environment manages the complete test infrastructure
type Environment struct {
	compose            *compose.DockerCompose
	services           map[string]*ServiceRunner
	EventBus           eventbus.EventBus
	Persistence        persistence.Persistence
	Logger             *slog.Logger
	testProjectRootDir string
	workspaceRootDir   string
	isReady            bool
	mu                 sync.RWMutex
}

// ServiceRunner manages individual Operion service processes
type ServiceRunner struct {
	name        string
	cmd         *exec.Cmd
	env         []string
	envMapping  map[string]string
	logFile     *os.File
	isReady     bool
	healthCheck func() error
	port        int
}

// NewEnvironment creates a new isolated test environment
func NewEnvironment(ctx context.Context) (*Environment, error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	workspaceRootDir := currentDir
	for {
		if _, err := os.Stat(filepath.Join(workspaceRootDir, "go.work")); err == nil {
			break
		}
		parent := filepath.Dir(workspaceRootDir)
		if parent == workspaceRootDir {
			return nil, fmt.Errorf("could not find project root with go.work")
		}
		workspaceRootDir = parent
	}

	te := &Environment{
		services:           make(map[string]*ServiceRunner),
		Logger:             logger,
		workspaceRootDir:   workspaceRootDir,
		testProjectRootDir: workspaceRootDir + "/test",
	}

	return te, nil
}

// StartInfrastructure starts Kafka and PostgreSQL containers
func (te *Environment) StartInfrastructure(ctx context.Context) error {
	te.Logger.Info("Starting infrastructure containers...")

	composeFilePath := filepath.Join(te.testProjectRootDir, "docker-compose.test.yml")
	if _, err := os.Stat(composeFilePath); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.test.yml not found at %s", composeFilePath)
	}

	stack, err := compose.NewDockerCompose(composeFilePath)
	if err != nil {
		return fmt.Errorf("failed to create docker compose stack: %w", err)
	}

	err = stack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true)) // Clean up any existing state
	if err != nil {
		return fmt.Errorf("failed to clean up existing docker compose stack: %w", err)
	}

	te.compose = stack

	// Start the stack
	err = stack.Up(ctx, compose.Wait(true))
	if err != nil {
		return fmt.Errorf("failed to start docker compose stack: %w", err)
	}

	// Wait for services to be ready
	if err := te.waitForInfrastructure(ctx); err != nil {
		return err
	}

	// Give additional time for Kafka to fully stabilize
	te.Logger.Info("Waiting for Kafka to fully stabilize...")
	time.Sleep(5 * time.Second)

	return nil
}

// waitForInfrastructure waits for Kafka and PostgreSQL to be ready
func (te *Environment) waitForInfrastructure(ctx context.Context) error {
	te.Logger.Info("Waiting for infrastructure to be ready...")

	// Wait for PostgreSQL
	if err := te.waitForPostgreSQL(ctx); err != nil {
		return fmt.Errorf("PostgreSQL not ready: %w", err)
	}

	// Wait for Kafka
	if err := te.waitForKafka(ctx); err != nil {
		return fmt.Errorf("Kafka not ready: %w", err)
	}

	// Initialize connections
	if err := te.initializePersistence(); err != nil {
		return fmt.Errorf("failed to initialize persistence: %w", err)
	}

	if err := te.initializeEventBus(ctx); err != nil {
		return fmt.Errorf("failed to initialize event bus: %w", err)
	}

	te.Logger.Info("Infrastructure is ready")
	return nil
}

// waitForPostgreSQL waits for PostgreSQL to accept connections
func (te *Environment) waitForPostgreSQL(ctx context.Context) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for PostgreSQL")
		case <-ticker.C:
			persistence, err := postgresql.NewPersistence(ctx, te.Logger, "postgres://test:test@localhost:15432/operion_test?sslmode=disable")
			if err == nil {
				if err := persistence.HealthCheck(ctx); err == nil {
					te.Logger.Debug("PostgreSQL is ready")
					return nil
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitForKafka waits for Kafka to be ready
func (te *Environment) waitForKafka(ctx context.Context) error {
	timeout := time.After(60 * time.Second)   // Increased timeout
	ticker := time.NewTicker(2 * time.Second) // Less frequent checks
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for Kafka after 60 seconds")
		case <-ticker.C:
			// Try to create event bus connection multiple times for stability
			os.Setenv("KAFKA_BROKERS", "localhost:29092")

			successCount := 0
			for i := 0; i < 3; i++ { // Try 3 times
				eventBus, err := kafka.NewEventBus(ctx, te.Logger)
				if err == nil {
					if closeErr := eventBus.Close(ctx); closeErr == nil {
						successCount++
					}
				}
				time.Sleep(500 * time.Millisecond) // Brief pause between attempts
			}

			if successCount >= 2 { // Require at least 2 successful connections
				te.Logger.Debug("Kafka is ready and stable")
				return nil
			}

			te.Logger.Debug("Kafka connection unstable, retrying...", "successful_attempts", successCount)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// initializePersistence creates the persistence layer
func (te *Environment) initializePersistence() error {
	persistence, err := postgresql.NewPersistence(context.Background(), te.Logger, te.PostgresURL())
	if err != nil {
		return fmt.Errorf("failed to create persistence: %w", err)
	}

	te.Persistence = persistence
	return nil
}

// initializeEventBus creates the event bus connection
func (te *Environment) initializeEventBus(ctx context.Context) error {
	os.Setenv("KAFKA_BROKERS", "localhost:29092")
	os.Setenv("KAFKA_GROUP_ID", "operion-test-group")

	eventBus, err := kafka.NewEventBus(ctx, te.Logger)
	if err != nil {
		return fmt.Errorf("failed to create event bus: %w", err)
	}

	te.EventBus = eventBus
	return nil
}

// StartOperionServices starts all Operion microservices
func (te *Environment) StartOperionServices(ctx context.Context) error {
	te.Logger.Info("Starting Operion services...")

	// Build all binaries first
	if err := te.buildBinaries(); err != nil {
		return fmt.Errorf("failed to build binaries: %w", err)
	}

	// Start services in dependency order
	services := []struct {
		name string
		port int
		env  map[string]string
	}{
		// {
		// 	name: "operion-api",
		// 	port: 19091,
		// 	env: map[string]string{
		// 		"PORT":          "19091",
		// 		"DATABASE_URL":  "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
		// 		"KAFKA_BROKERS": "localhost:29092",
		// 		"LOG_LEVEL":     "debug",
		// 	},
		// },
		{
			name: "operion-source-manager",
			env: map[string]string{
				"DATABASE_URL":              "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
				"KAFKA_BROKERS":             "localhost:29092",
				"SOURCE_PROVIDERS":          "webhook",
				"WEBHOOK_PORT":              "9085",
				"SCHEDULER_PERSISTENCE_URL": "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
				"WEBHOOK_PERSISTENCE_URL":   "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
				"LOG_LEVEL":                 "debug",
				"EVENT_BUS_TYPE":            "kafka",
			},
		},
		{
			name: "operion-activator",
			env: map[string]string{
				"DATABASE_URL":   "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
				"KAFKA_BROKERS":  "localhost:29092",
				"LOG_LEVEL":      "debug",
				"EVENT_BUS_TYPE": "kafka",
			},
		},
		{
			name: "operion-worker",
			env: map[string]string{
				"DATABASE_URL":   "postgres://test:test@localhost:15432/operion_test?sslmode=disable",
				"KAFKA_BROKERS":  "localhost:29092",
				"LOG_LEVEL":      "debug",
				"EVENT_BUS_TYPE": "kafka",
			},
		},
	}

	for _, svc := range services {
		if err := te.startService(ctx, svc.name, svc.port, svc.env); err != nil {
			return fmt.Errorf("failed to start %s: %w", svc.name, err)
		}
	}

	// Wait for all services to be ready
	return te.waitForServicesReady(ctx, 60*time.Second)
}

// buildBinaries builds all required Operion binaries
func (te *Environment) buildBinaries() error {
	te.Logger.Info("Building Operion binaries...")

	cmd := exec.Command("make", "clean", "build")
	cmd.Dir = te.workspaceRootDir + "/operion"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build binaries: %w", err)
	}

	return nil
}

// startService starts an individual Operion service
func (te *Environment) startService(ctx context.Context, serviceName string, port int, envVars map[string]string) error {
	binaryPath := filepath.Join(te.workspaceRootDir, "operion/bin", serviceName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found: %s", binaryPath)
	}

	// Create log file
	logDir := filepath.Join(te.testProjectRootDir, "logs")
	logPath := filepath.Join(logDir, serviceName+".log")
	os.MkdirAll(logDir, 0755)
	os.Remove(logPath)

	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Prepare environment
	env := os.Environ()
	for key, value := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Start the service
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = te.workspaceRootDir

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start %s: %w", serviceName, err)
	}

	// Create health check function

	healthCheck := func() error {
		if port > 0 {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", port))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check failed: %d", resp.StatusCode)
			}
		} else {
			if cmd.Process == nil {
				return fmt.Errorf("process not running")
			}
			// Simple check if process is still alive (Unix-like systems)
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				return fmt.Errorf("process check failed: %w", err)
			}

		}

		return nil
	}

	runner := &ServiceRunner{
		name:        serviceName,
		cmd:         cmd,
		env:         env,
		envMapping:  envVars,
		logFile:     logFile,
		healthCheck: healthCheck,
		port:        port,
	}

	te.mu.Lock()
	te.services[serviceName] = runner
	te.mu.Unlock()

	te.Logger.Info("Started service", "name", serviceName, "pid", cmd.Process.Pid)
	return nil
}

// waitForServicesReady waits for all services to pass health checks
func (te *Environment) waitForServicesReady(ctx context.Context, timeout time.Duration) error {
	te.Logger.Info("Waiting for services to be ready...")

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for services to be ready")
			}

			allReady := true
			te.mu.RLock()
			for name, runner := range te.services {
				if !runner.isReady {
					if err := runner.healthCheck(); err != nil {
						te.Logger.Debug("Service not ready", "service", name, "error", err)
						allReady = false
					} else {
						runner.isReady = true
						te.Logger.Info("Service is ready", "service", name)
					}
				}
			}
			te.mu.RUnlock()

			if allReady && len(te.services) > 0 {
				te.isReady = true
				te.Logger.Info("All services are ready")
				return nil
			}
		}
	}
}

// // GetServiceLogs returns logs for a specific service
// func (te *Environment) GetServiceLogs(serviceName string) (string, error) {
// 	te.mu.RLock()
// 	runner, exists := te.services[serviceName]
// 	te.mu.RUnlock()

// 	if !exists {
// 		return "", fmt.Errorf("service %s not found", serviceName)
// 	}

// 	if runner.logFile == nil {
// 		return "", fmt.Errorf("no log file for service %s", serviceName)
// 	}

// 	logPath := runner.logFile.Name()
// 	data, err := os.ReadFile(logPath)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to read log file: %w", err)
// 	}

// 	return string(data), nil
// }

// IsReady returns true if the test environment is fully ready
func (te *Environment) IsReady() bool {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.isReady
}

// WebhookURL returns the base URL for the Webhook service
func (te *Environment) WebhookURL() string {
	te.mu.RLock()
	defer te.mu.RUnlock()

	runner, exists := te.services["operion-source-manager"]
	portStr := runner.envMapping["WEBHOOK_PORT"]

	if exists && portStr != "" {
		portInt, err := strconv.Atoi(portStr)
		if err != nil {
			panic(fmt.Sprintf("invalid WEBHOOK_PORT value: %s", portStr))
		}
		return fmt.Sprintf("http://localhost:%d/webhook", portInt)
	}

	panic("operion-source-manager service not found or WEBHOOK_PORT not set")
}

func (te *Environment) PostgresURL() string {
	return "postgres://test:test@localhost:15432/operion_test?sslmode=disable"
}

// Cleanup shuts down all services and containers
func (te *Environment) Cleanup() error {
	te.Logger.Info("Cleaning up test environment...")

	var errors []string

	// Stop Operion services
	te.mu.Lock()
	for name, runner := range te.services {
		if runner.cmd != nil && runner.cmd.Process != nil {
			te.Logger.Info("Stopping service", "name", name, "pid", runner.cmd.Process.Pid)
			if err := runner.cmd.Process.Kill(); err != nil {
				errors = append(errors, fmt.Sprintf("failed to kill %s: %v", name, err))
			}
			runner.cmd.Wait() // Wait for process to exit
		}
		if runner.logFile != nil {
			runner.logFile.Close()
		}
	}
	te.services = make(map[string]*ServiceRunner)
	te.mu.Unlock()

	if te.EventBus != nil {
		if err := te.EventBus.Close(context.Background()); err != nil {
			errors = append(errors, fmt.Sprintf("failed to close event bus: %v", err))
		}
	}

	if te.compose != nil {
		if err := te.compose.Down(context.Background(), compose.RemoveOrphans(true), compose.RemoveImagesLocal); err != nil {
			errors = append(errors, fmt.Sprintf("failed to stop containers: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	te.Logger.Info("Cleanup completed")
	return nil
}
