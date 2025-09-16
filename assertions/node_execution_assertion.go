package assertions

// // NodeExecutionAssertion verifies that a specific node was executed
// type NodeExecutionAssertion struct {
// 	nodeID string
// }

// func (nea *NodeExecutionAssertion) Verify(collector *EventCollector, timeout time.Duration) error {
// 	deadline := time.Now().Add(timeout)

// 	for time.Now().Before(deadline) {
// 		completionEvents := collector.GetEventsByType(events.NodeCompletionEvent)
// 		for _, event := range completionEvents {
// 			if nodeEvent, ok := event.(*events.NodeCompletion); ok {
// 				if nodeEvent.NodeID == nea.nodeID {
// 					return nil
// 				}
// 			}
// 		}
// 		time.Sleep(100 * time.Millisecond)
// 	}

// 	return fmt.Errorf("node %s was not executed within timeout", nea.nodeID)
// }

// func (nea *NodeExecutionAssertion) Description() string {
// 	return fmt.Sprintf("Node %s executed", nea.nodeID)
// }
