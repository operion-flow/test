package assertions

import (
	"time"

	"github.com/operion-flow/test/integration/collectors"
)

// Assertion represents an assertion to be verified after test execution.
type Assertion interface {
	Verify(collector *collectors.EventCollector, timeout time.Duration) error
	Description() string
}
