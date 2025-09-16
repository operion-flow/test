package assertions

// // WorkflowCompletionAssertion verifies that a workflow completes successfully
// type WorkflowCompletionAssertion struct {
// 	workflowID string
// }

// func (wca *WorkflowCompletionAssertion) Verify(collector *EventCollector, timeout time.Duration) error {
// 	return collector.WaitForWorkflowCompletion(wca.workflowID, timeout)
// }

// func (wca *WorkflowCompletionAssertion) Description() string {
// 	return fmt.Sprintf("Workflow %s completes successfully", wca.workflowID)
// }
