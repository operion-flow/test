package assertions

import (
	"fmt"
	"strings"
	"time"

	"github.com/dukex/operion/pkg/events"
	"github.com/operion-flow/test/integration/collectors"
)

// EventSequenceAssertion verifies that events occur in a specific sequence
type EventSequenceAssertion struct {
	EventTypes []events.EventType
}

func (esa *EventSequenceAssertion) Verify(collector *collectors.EventCollector, timeout time.Duration) error {
	return collector.WaitForEventSequence(esa.EventTypes, timeout)
}

func (esa *EventSequenceAssertion) Description() string {
	typeNames := make([]string, len(esa.EventTypes))
	for i, et := range esa.EventTypes {
		typeNames[i] = string(et)
	}
	return fmt.Sprintf("Event sequence: %s", strings.Join(typeNames, " → "))
}
