package scenarios

import (
	"testing"
	"time"

	"github.com/dukex/operion/pkg/events"
	"github.com/dukex/operion/pkg/models"
	"github.com/operion-flow/test/integration/builder"
)

func TestE2E_WebhookToAPICall(t *testing.T) {
	builder.NewScenarioBuilder(t).
		// Prepare
		WithWorkflow(&models.Workflow{
			ID:          "c699db0a-0d06-4917-9d6c-fbf7f5d11ae1",
			Name:        "Webhook to API Call",
			Description: "End-to-end test workflow: webhook trigger → HTTP request → transform → log",
			Status:      models.WorkflowStatusPublished,
			Nodes: []*models.WorkflowNode{
				{
					ID:         "webhook_trigger",
					Name:       "Webhook Trigger",
					Type:       "webhook",
					Category:   models.CategoryTypeTrigger,
					ProviderID: "webhook",
					SourceID:   "aca12740-fe4d-4ec9-849b-75c09c089485",
					Config: map[string]any{
						"endpoint": "/test-webhook",
						"method":   "POST",
					},
					EventType: "webhook_received", //
					Enabled:   true,
				},
				{
					ID:       "api_call",
					Name:     "HTTP Request",
					Type:     "httprequest",
					Category: models.CategoryTypeAction,
					Config: map[string]any{
						"url":    "https://jsonplaceholder.typicode.com/posts",
						"method": "POST",
						"body":   `{"title": "{{.trigger_data.webhook.title}}", "body": "Test post from E2E", "userId": 1}`,
						"headers": map[string]string{
							"Content-Type": "application/json",
						},
						"retries": map[string]any{
							"attempts": 2,
							"delay":    1000,
						},
					},
					Enabled: true,
				},
				{
					ID:       "transform_response",
					Name:     "Transform Response",
					Type:     "transform",
					Category: models.CategoryTypeAction,
					Config: map[string]any{
						"expression": `{
							"post_id": {{.step_results.api_call.id}},
							"created": true,
							"original_title": "{{.trigger_data.webhook.title}}",
							"timestamp": "{{now}}"
						}`,
					},
					Enabled: true,
				},
				{
					ID:       "log_result",
					Name:     "Log Result",
					Type:     "log",
					Category: models.CategoryTypeAction,
					Config: map[string]any{
						"message": "Successfully created post with ID: {{.step_results.transform_response.post_id}} for title: {{.step_results.transform_response.original_title}}",
						"level":   "info",
					},
					Enabled: true,
				},
			},
			Connections: []*models.Connection{
				{
					ID:         "conn_1",
					SourcePort: "webhook_trigger:output",
					TargetPort: "api_call:input",
				},
				{
					ID:         "conn_2",
					SourcePort: "api_call:output",
					TargetPort: "transform_response:input",
				},
				{
					ID:         "conn_3",
					SourcePort: "transform_response:output",
					TargetPort: "log_result:input",
				},
			},
			Variables: map[string]any{
				"test_environment": "integration",
				"api_timeout":      "30s",
			},
		}).
		// WithWebhookSource("webhook_trigger", "/test-webhook").

		// Execute
		ExecuteWebhookTrigger("aca12740-fe4d-4ec9-849b-75c09c089485", map[string]any{
			"title":    "Test Post from E2E Integration",
			"category": "testing",
			"priority": "high",
		}).
		Wait(2*time.Second, "allow workflow processing").

		// Assert
		ExpectEventSequence([]events.EventType{
			events.NodeActivationEvent,             // webhook_trigger activates
			events.NodeCompletionEvent,             // webhook_trigger completes
			events.NodeActivationEvent,             // api_call activates
			events.NodeCompletionEvent,             // api_call completes
			events.NodeActivationEvent,             // transform_response activates
			events.NodeCompletionEvent,             // transform_response completes
			events.NodeActivationEvent,             // log_result activates
			events.NodeCompletionEvent,             // log_result completes
			events.WorkflowExecutionCompletedEvent, // Workflow execution completes
		}).
		// ExpectWorkflowCompletion("c699db0a-0d06-4917-9d6c-fbf7f5d11ae1").

		// // Verify all nodes executed
		// ExpectNodeExecution("webhook_trigger").
		// ExpectNodeExecution("api_call").
		// ExpectNodeExecution("transform_response").
		// ExpectNodeExecution("log_result").

		// // Verify event counts
		// ExpectEventCount(events.WorkflowExecutionStartedEvent, 1).
		// ExpectEventCount(events.WorkflowExecutionCompletedEvent, 1).
		// ExpectEventCount(events.NodeActivationEvent, 4). // 4 nodes activated
		// ExpectEventCount(events.NodeCompletionEvent, 4). // 4 nodes completed

		// Set timeout
		WithTimeout(60 * time.Second).

		// Execute the scenario
		Run()
}
