package actions

import (
	"fmt"
	"time"

	"github.com/operion-flow/test/integration/environment"
)

// WaitAction waits for a specified duration
type WaitAction struct {
	Duration time.Duration
	Reason   string
}

func (wa *WaitAction) Execute(env *environment.Environment) error {
	time.Sleep(wa.Duration)
	return nil
}

func (wa *WaitAction) Description() string {
	return fmt.Sprintf("Wait %v (%s)", wa.Duration, wa.Reason)
}
