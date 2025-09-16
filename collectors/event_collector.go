package collectors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dukex/operion/pkg/eventbus"
	"github.com/dukex/operion/pkg/events"
)

// EventCollector captures and organizes events during test execution
type EventCollector struct {
	events           []eventbus.Event
	eventsByType     map[events.EventType][]eventbus.Event
	eventsByWorkflow map[string][]eventbus.Event
	mu               sync.RWMutex
	eventBus         eventbus.EventBus
	handlers         map[events.EventType]bool
	isRunning        bool
	stopChan         chan struct{}
}

// NewEventCollector creates a new event collector
func NewEventCollector(eventBus eventbus.EventBus) *EventCollector {
	return &EventCollector{
		events:           make([]eventbus.Event, 0),
		eventsByType:     make(map[events.EventType][]eventbus.Event),
		eventsByWorkflow: make(map[string][]eventbus.Event),
		eventBus:         eventBus,
		handlers:         make(map[events.EventType]bool),
		stopChan:         make(chan struct{}),
	}
}

// Start begins collecting events
func (ec *EventCollector) Start(ctx context.Context) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.isRunning {
		return fmt.Errorf("event collector is already running")
	}

	// Register handlers for all event types we want to track
	eventTypes := []events.EventType{
		events.WorkflowTriggeredEvent,
		events.WorkflowFinishedEvent,
		events.WorkflowFailedEvent,
		events.NodeActivationEvent,
		events.NodeCompletionEvent,
		events.NodeExecutionFinishedEvent,
		events.NodeExecutionFailedEvent,
		events.WorkflowExecutionStartedEvent,
		events.WorkflowExecutionCompletedEvent,
		events.WorkflowExecutionFailedEvent,
		events.WorkflowExecutionCancelledEvent,
		events.WorkflowExecutionTimeoutEvent,
		events.WorkflowExecutionPausedEvent,
		events.WorkflowExecutionResumedEvent,
		events.WorkflowVariablesUpdatedEvent,
	}

	for _, eventType := range eventTypes {
		err := ec.eventBus.Handle(ctx, eventType, ec.createEventHandler(eventType))
		if err != nil {
			return fmt.Errorf("failed to register handler for %s: %w", eventType, err)
		}
		ec.handlers[eventType] = true
	}

	// Start subscribing to events
	err := ec.eventBus.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	ec.isRunning = true
	return nil
}

// createEventHandler creates a handler function for a specific event type
func (ec *EventCollector) createEventHandler(eventType events.EventType) eventbus.EventHandler {
	return func(ctx context.Context, event any) error {
		if e, ok := event.(eventbus.Event); ok {
			ec.addEvent(e)
		}
		return nil
	}
}

// addEvent adds an event to the collector
func (ec *EventCollector) addEvent(event eventbus.Event) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	fmt.Println("EventCollector: Captured event", "type", event.Type())

	ec.events = append(ec.events, event)

	eventType := event.Type()
	if ec.eventsByType[eventType] == nil {
		ec.eventsByType[eventType] = make([]eventbus.Event, 0)
	}
	ec.eventsByType[eventType] = append(ec.eventsByType[eventType], event)

	if workflowEvent, ok := event.(interface{ WorkflowID() string }); ok {
		workflowID := workflowEvent.WorkflowID()
		if workflowID != "" {
			if ec.eventsByWorkflow[workflowID] == nil {
				ec.eventsByWorkflow[workflowID] = make([]eventbus.Event, 0)
			}
			ec.eventsByWorkflow[workflowID] = append(ec.eventsByWorkflow[workflowID], event)
		}
	}
}

// Stop stops collecting events
func (ec *EventCollector) Stop() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.isRunning {
		return
	}

	close(ec.stopChan)
	ec.isRunning = false
}

// // GetAllEvents returns all collected events
// func (ec *EventCollector) GetAllEvents() []eventbus.Event {
// 	ec.mu.RLock()
// 	defer ec.mu.RUnlock()

// 	result := make([]eventbus.Event, len(ec.events))
// 	copy(result, ec.events)
// 	return result
// }

// EventsByType returns all events of a specific type
func (ec *EventCollector) EventsByType(eventType events.EventType) []eventbus.Event {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	if events, exists := ec.eventsByType[eventType]; exists {
		result := make([]eventbus.Event, len(events))
		copy(result, events)
		return result
	}
	return []eventbus.Event{}
}

// // GetEventsByWorkflow returns all events for a specific workflow
// func (ec *EventCollector) GetEventsByWorkflow(workflowID string) []eventbus.Event {
// 	ec.mu.RLock()
// 	defer ec.mu.RUnlock()

// 	if events, exists := ec.eventsByWorkflow[workflowID]; exists {
// 		result := make([]eventbus.Event, len(events))
// 		copy(result, events)
// 		return result
// 	}
// 	return []eventbus.Event{}
// }

// // WaitForEvent waits for a specific event type to be received
// func (ec *EventCollector) WaitForEvent(eventType events.EventType, timeout time.Duration) (eventbus.Event, error) {
// 	deadline := time.Now().Add(timeout)
// 	ticker := time.NewTicker(100 * time.Millisecond)
// 	defer ticker.Stop()

// 	for {
// 		// Check if we already have the event
// 		events := ec.GetEventsByType(eventType)
// 		if len(events) > 0 {
// 			return events[0], nil // Return the first matching event
// 		}

// 		// Check timeout
// 		if time.Now().After(deadline) {
// 			return nil, fmt.Errorf("timeout waiting for event %s after %v", eventType, timeout)
// 		}

// 		// Wait a bit before checking again
// 		select {
// 		case <-ticker.C:
// 			continue
// 		case <-ec.stopChan:
// 			return nil, fmt.Errorf("event collector stopped")
// 		}
// 	}
// }

// // WaitForWorkflowCompletion waits for a workflow to complete (successfully or with failure)
// func (ec *EventCollector) WaitForWorkflowCompletion(workflowID string, timeout time.Duration) error {
// 	deadline := time.Now().Add(timeout)
// 	ticker := time.NewTicker(100 * time.Millisecond)
// 	defer ticker.Stop()

// 	for {
// 		// Check for completion events
// 		completedEvents := ec.GetEventsByType(events.WorkflowExecutionCompletedEvent)
// 		for _, event := range completedEvents {
// 			if workflowEvent, ok := event.(interface{ WorkflowID() string }); ok {
// 				if workflowEvent.WorkflowID() == workflowID {
// 					return nil // Workflow completed successfully
// 				}
// 			}
// 		}

// 		// Check for failure events
// 		failedEvents := ec.GetEventsByType(events.WorkflowExecutionFailedEvent)
// 		for _, event := range failedEvents {
// 			if workflowEvent, ok := event.(interface{ WorkflowID() string }); ok {
// 				if workflowEvent.WorkflowID() == workflowID {
// 					return fmt.Errorf("workflow failed")
// 				}
// 			}
// 		}

// 		// Check timeout
// 		if time.Now().After(deadline) {
// 			return fmt.Errorf("timeout waiting for workflow %s to complete after %v", workflowID, timeout)
// 		}

// 		// Wait a bit before checking again
// 		select {
// 		case <-ticker.C:
// 			continue
// 		case <-ec.stopChan:
// 			return fmt.Errorf("event collector stopped")
// 		}
// 	}
// }

// WaitForEventSequence waits for a specific sequence of events to occur
func (ec *EventCollector) WaitForEventSequence(eventTypes []events.EventType, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	expectedIndex := 0

	for {
		if expectedIndex >= len(eventTypes) {
			return nil
		}

		// Check if we have the next expected event
		events := ec.EventsByType(eventTypes[expectedIndex])
		if len(events) > expectedIndex {
			expectedIndex++
			continue
		}

		// Check timeout
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for event sequence at step %d (%s) after %v",
				expectedIndex, eventTypes[expectedIndex], timeout)
		}

		// Wait a bit before checking again
		select {
		case <-ticker.C:
			continue
		case <-ec.stopChan:
			return fmt.Errorf("event collector stopped")
		}
	}
}

// // HasEventOfType checks if an event of the specified type has been collected
// func (ec *EventCollector) HasEventOfType(eventType events.EventType) bool {
// 	events := ec.GetEventsByType(eventType)
// 	return len(events) > 0
// }

// // CountEventsByType returns the number of events of a specific type
// func (ec *EventCollector) CountEventsByType(eventType events.EventType) int {
// 	events := ec.GetEventsByType(eventType)
// 	return len(events)
// }

// // GetEventCount returns the total number of events collected
// func (ec *EventCollector) GetEventCount() int {
// 	ec.mu.RLock()
// 	defer ec.mu.RUnlock()
// 	return len(ec.events)
// }

// // Clear removes all collected events
// func (ec *EventCollector) Clear() {
// 	ec.mu.Lock()
// 	defer ec.mu.Unlock()

// 	ec.events = make([]eventbus.Event, 0)
// 	ec.eventsByType = make(map[events.EventType][]eventbus.Event)
// 	ec.eventsByWorkflow = make(map[string][]eventbus.Event)
// }

// EventSummary returns a summary of collected events
func (ec *EventCollector) EventSummary() map[events.EventType]int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	summary := make(map[events.EventType]int)
	for eventType, events := range ec.eventsByType {
		summary[eventType] = len(events)
	}
	return summary
}
