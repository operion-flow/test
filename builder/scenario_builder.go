package builder

import (
	"context"
	"testing"
	"time"

	"github.com/dukex/operion/pkg/events"
	"github.com/dukex/operion/pkg/models"
	"github.com/operion-flow/test/integration/actions"
	"github.com/operion-flow/test/integration/assertions"
	"github.com/operion-flow/test/integration/collectors"
	"github.com/operion-flow/test/integration/environment"
	"github.com/stretchr/testify/require"
)

type ScenarioBuilder struct {
	t              *testing.T
	workflows      []*models.Workflow
	sources        []*models.Source
	actions        []actions.Action
	assertions     []assertions.Assertion
	environment    *environment.Environment
	eventCollector *collectors.EventCollector
	timeout        time.Duration
}

// NewScenarioBuilder creates a new scenario builder.
func NewScenarioBuilder(t *testing.T) *ScenarioBuilder {
	return &ScenarioBuilder{
		t:         t,
		workflows: make([]*models.Workflow, 0),
		sources:   make([]*models.Source, 0),
		actions:   make([]actions.Action, 0),
		// assertions: make([]TestAssertion, 0),
		timeout: 30 * time.Second,
	}
}

// WithWorkflow adds a workflow to the scenario
func (esb *ScenarioBuilder) WithWorkflow(workflow *models.Workflow) *ScenarioBuilder {
	esb.workflows = append(esb.workflows, workflow)
	return esb
}

// // WithSource adds a source to the scenario
// func (esb *ScenarioBuilder) WithSource(source *models.Source) *ScenarioBuilder {
// 	esb.sources = append(esb.sources, source)
// 	return esb
// }

// // WithWebhookSource adds a webhook source configuration
// func (esb *ScenarioBuilder) WithWebhookSource(triggerNodeID, endpoint string) *ScenarioBuilder {
// 	source := &models.Source{
// 		ID:         fmt.Sprintf("webhook_source_%s", triggerNodeID),
// 		ProviderID: "webhook",
// 		OwnerID:    "test_owner",
// 		Configuration: map[string]any{
// 			"endpoint": endpoint,
// 			"method":   "POST",
// 		},
// 	}
// 	return esb.WithSource(source)
// }

// // WithSchedulerSource adds a scheduler source configuration
// func (esb *ScenarioBuilder) WithSchedulerSource(triggerNodeID, schedule string) *ScenarioBuilder {
// 	source := &models.Source{
// 		ID:         fmt.Sprintf("scheduler_source_%s", triggerNodeID),
// 		ProviderID: "scheduler",
// 		OwnerID:    "test_owner",
// 		Configuration: map[string]any{
// 			"schedule": schedule,
// 		},
// 	}
// 	return esb.WithSource(source)
// }

// ExecuteWebhookTrigger adds a webhook trigger action
func (esb *ScenarioBuilder) ExecuteWebhookTrigger(sourceId string, payload map[string]any) *ScenarioBuilder {
	action := &actions.WebhookTriggerAction{
		SourceId: sourceId,
		Payload:  payload,
		Method:   "POST",
	}
	esb.actions = append(esb.actions, action)
	return esb
}

// Wait adds a wait action
func (esb *ScenarioBuilder) Wait(duration time.Duration, reason string) *ScenarioBuilder {
	action := &actions.WaitAction{
		Duration: duration,
		Reason:   reason,
	}
	esb.actions = append(esb.actions, action)
	return esb
}

// ExpectEventSequence adds an event sequence assertion
func (esb *ScenarioBuilder) ExpectEventSequence(eventTypes []events.EventType) *ScenarioBuilder {
	assertion := &assertions.EventSequenceAssertion{
		EventTypes: eventTypes,
	}
	esb.assertions = append(esb.assertions, assertion)
	return esb
}

// // ExpectWorkflowCompletion adds a workflow completion assertion
// func (esb *ScenarioBuilder) ExpectWorkflowCompleID: workflowID,
// 	}
// 	esb.assertions = append(esb.assertions, assertion)
// 	return esb
// }

// // ExpectEventCount adds an event count assertion
// func (esb *ScenarioBuilder) ExpectEventCount(eventTyp = append(esb.assertions, assertion)
// 	return esb
// }

// // ExpectNodeExecution adds a node execution assertion
// func (esb *ScenarioBuilder) ExpectNodeExecution(nodeID string) *ScenarioBuilder {
// 	assertion := &NodeExecutionAssertion{
// 		nodeID: nodeID,
// 	}
// 	esb.assertions = append(esb.assertions, assertion)
// 	return esb
// }

// WithTimeout sets the overall test timeout
func (esb *ScenarioBuilder) WithTimeout(timeout time.Duration) *ScenarioBuilder {
	esb.timeout = timeout
	return esb
}

// Run executes the complete test scenario
func (esb *ScenarioBuilder) Run() {
	ctx, cancel := context.WithTimeout(esb.t.Context(), esb.timeout+time.Minute)
	defer cancel()

	// 1. Setup test environment
	esb.t.Log("Setting up test environment...")
	env, err := environment.NewEnvironment(ctx)
	require.NoError(esb.t, err, "Failed to create test environment")
	esb.environment = env

	// Ensure cleanup happens even if test fails
	esb.t.Cleanup(func() {
		if env != nil {
			if err := env.Cleanup(); err != nil {
				esb.t.Logf("Cleanup failed: %v", err)
			}
		}
	})

	// 2. Start infrastructure
	esb.t.Log("Starting infrastructure...")
	err = env.StartInfrastructure(ctx)
	require.NoError(esb.t, err, "Failed to start infrastructure")

	// 3. Deploy workflows and sources
	esb.t.Log("Deploying test workflows and sources...")
	for _, workflow := range esb.workflows {
		err := env.Persistence.WorkflowRepository().Save(ctx, workflow)
		require.NoError(esb.t, err, "Failed to create workflow %s", workflow.ID)
		esb.t.Logf("Created workflow: %s", workflow.ID)
	}

	// 4. Start Operion services
	esb.t.Log("Starting Operion services...")
	err = env.StartOperionServices(ctx)
	require.NoError(esb.t, err, "Failed to start Operion services")

	// 5. Wait for environment to be ready
	esb.t.Log("Waiting for environment to be ready...")
	timeout := 60 * time.Second
	deadline := time.Now().Add(timeout)
	for !env.IsReady() && time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
	}
	require.True(esb.t, env.IsReady(), "Environment not ready within timeout")

	// 6. Setup event collection
	esb.t.Log("Starting event collection...")
	eventCollector := collectors.NewEventCollector(env.EventBus)
	err = eventCollector.Start(ctx)
	require.NoError(esb.t, err, "Failed to start event collector")
	esb.eventCollector = eventCollector

	defer eventCollector.Stop()

	// 7. Wait a bit for services to pick up new configurations
	time.Sleep(2 * time.Second)

	// 8. Execute test actions
	esb.t.Log("Executing test actions...")
	for i, action := range esb.actions {
		esb.t.Logf("Executing action %d: %s", i+1, action.Description())
		err := action.Execute(env)
		require.NoError(esb.t, err, "Failed to execute action: %s", action.Description())
	}

	summary := eventCollector.EventSummary()
	esb.t.Log("Event summary:")
	for eventType, count := range summary {
		esb.t.Logf("  %s: %d", eventType, count)
	}

	// 9. Verify assertions
	esb.t.Log("Verifying assertions...")
	for i, assertion := range esb.assertions {
		esb.t.Logf("Verifying assertion %d: %s", i+1, assertion.Description())
		err := assertion.Verify(eventCollector, esb.timeout)
		require.NoError(esb.t, err, "Assertion failed: %s", assertion.Description())
	}

	// 10. Print event summary for debugging
	// summary := eventCollector.EventSummary()
	// esb.t.Log("Event summary:")
	// for eventType, count := range summary {
	// 	esb.t.Logf("  %s: %d", eventType, count)
	// }

	esb.t.Log("Test scenario completed successfully!")
}
