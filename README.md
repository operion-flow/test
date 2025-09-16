# Integration Tests

:warning: SHOULD BE RUN USING THE [workspace](https://github.com/operion-flow/workspace)

This directory contains end-to-end integration tests for the Operion workflow automation platform.

## Overview

The integration tests verify the complete system behavior by:

- Starting real infrastructure (Kafka, PostgreSQL) in containers
- Running all Operion microservices (API, Worker, Activator, Source Manager)
- Executing real workflows with actual event processing
- Verifying end-to-end behavior through event collection and assertions

## Structure

```
./
├── docker-compose.test.yml    # Infrastructure containers
├── environment/               # Test environment management
│   └── environment.go         # Environment setup and teardown
├── collectors/                # Event collection utilities
│   └── event_collector.go     # Event collection and verification
├── builder/                   # Test scenario construction
│   └── scenario_builder.go    # Fluent test scenario builder
├── actions/                   # Test action implementations
│   ├── action.go              # Action interface
│   ├── webhook_trigger_action.go # Webhook trigger action
│   └── wait_action.go         # Wait action
├── assertions/                # Test assertion implementations
│   ├── assertion.go           # Assertion interface
│   ├── workflow_completion_assertion.go # Workflow completion checks
│   ├── node_execution_assertion.go # Node execution verification
│   ├── event_sequence_assertion.go # Event ordering checks
│   └── event_count_assertion.go # Event count verification
├── scenarios/                 # Actual test scenarios
│   └── webhook_to_api_test.go # Example webhook → API → transform → log test
└── logs/                     # Service logs (created during tests)
```

## Running Tests

### Prerequisites

- Docker and Docker Compose installed
- Go 1.24+ installed
- Make available for building binaries

### Run All Integration Tests

```bash
# From project root using make
make run

# Run using go
go test -v ./scenarios
```

### Run Specific Test

```bash
# Run specific test scenario
go test -v ./scenarios -run TestE2E_WebhookToAPICall

# Run with race detection
go test -race -v ./scenarios

# Run with longer timeout
go test -timeout 5m -v ./scenarios
```

## Test Environment

### Infrastructure Components

The test environment automatically manages:

- **Kafka**: Message broker for event-driven communication
  - Port: 29092
  - Topics: `operion.events`, `operion.node.activations`, `operion.workflow.executions`
- **PostgreSQL**: Database for workflow and execution state
  - Port: 15432
  - Database: `operion_test`
  - User: `test` / Password: `test`

### Operion Services

All Operion microservices are started automatically:

- **operion-api**: REST API server (port 19091)
- **operion-worker**: Workflow execution worker
- **operion-activator**: Source event to workflow event bridge
- **operion-source-manager**: Source provider orchestrator

## Writing Tests

### Basic Test Structure

```go
func TestE2E_YourScenario(t *testing.T) {
    utils.NewE2EScenarioBuilder(t).
        WithWorkflow(createTestWorkflow()).
        WithWebhookSource("trigger_node", "/test-endpoint").
        ExecuteWebhookTrigger("/test-endpoint", payload).
        ExpectWorkflowCompletion("workflow-id").
        ExpectNodeExecution("node-id").
        WithTimeout(60*time.Second).
        Run()
}
```

### Available Assertions

- `ExpectEventSequence([]events.EventType)` - Verify events occur in order
- `ExpectWorkflowCompletion(workflowID)` - Wait for workflow success
- `ExpectNodeExecution(nodeID)` - Verify node was executed
- `ExpectEventCount(eventType, count)` - Verify event count
- `WithTimeout(duration)` - Set test timeout

### Available Actions

- `ExecuteWebhookTrigger(endpoint, payload)` - Trigger webhook
- `Wait(duration, reason)` - Wait for specified time
- Custom actions can be implemented using the `TestAction` interface

## Test Example

The `webhook_to_api_test.go` demonstrates a complete integration test:

1. **Setup**: Creates a workflow with webhook trigger → HTTP request → transform → log
2. **Execution**: Triggers webhook with test payload
3. **Verification**: Confirms all nodes execute and workflow completes
4. **Assertions**: Verifies event sequence and counts

## Debugging

### Service Logs

Service logs are written to `logs/`:

- `operion-api.log`
- `operion-worker.log`
- `operion-activator.log`
- `operion-source-manager.log`

### Event Debugging

The EventCollector provides detailed event information:

```go
// In test failure, print event summary
summary := eventCollector.GetEventSummary()
for eventType, count := range summary {
    t.Logf("Event %s: %d", eventType, count)
}
```

### Infrastructure Debugging

```bash
# Check container status
docker ps

# View container logs
docker logs operion_kafka_1
docker logs operion_postgres_1

# Connect to test database
psql -h localhost -p 15432 -U test -d operion_test
```

## Performance Considerations

- Tests start fresh containers for isolation
- Typical test runtime: 30-60 seconds
- Infrastructure startup: ~10-15 seconds
- Service startup: ~5-10 seconds

## Best Practices

1. **Isolation**: Each test gets fresh environment
2. **Realistic Data**: Use real API endpoints when possible
3. **Event-Driven Verification**: Assert on events, not implementation details
4. **Timeout Management**: Set appropriate timeouts for complex workflows
5. **Resource Cleanup**: Framework handles cleanup automatically
6. **Debugging**: Include event summaries and service logs for failures
