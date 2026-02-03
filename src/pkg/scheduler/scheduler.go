package scheduler

import (
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
)

// Scheduler is an interface for anything that can evaluate a resource and return
// an action.
type Scheduler interface {
	// Evaluate takes input and returns a decision or error
	Evaluate(any) (action.Action, error)
	// SetLocation changes the timezone of the scheduler
	SetLocation(*time.Location) (Scheduler, error)
	// Type returns the type of scheduler in use
	Type() string
}

type NullScheduler struct{}

func (n *NullScheduler) Evaluate(any) (action.Action, error) {
	return action.NULL_ACTION, ErrNoScheduler
}

func (n *NullScheduler) SetLocation(t *time.Location) (Scheduler, error) {
	return &NullScheduler{}, ErrNoScheduler
}

func (n *NullScheduler) Type() string {
	return configuration.NULL_SCHEDULER
}

// ScheduleFunc returns the function to generate the schedule based on configurations.
// Defaults to AnyKeyNL Scheduler.
func ScheduleFunc(fn string) func() Scheduler {
	switch fn {
	case configuration.ANYKEYNL_SCHEDULER:
		return NewAnykeyNLScheduler
	default:
		return NewAnykeyNLScheduler
	}
}
