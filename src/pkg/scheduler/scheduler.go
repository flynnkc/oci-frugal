package scheduler

import (
	"errors"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
)

// Actions are int8 and we reserve positive integers so constants are all <0
const (
	OFF         Action = 0
	ON          Action = -1
	NULL_ACTION Action = -2
)

var (
	ErrInvalidInput    error = errors.New("error invalid input in scheduler")
	ErrNoScheduler     error = errors.New("error no scheduler set")
	ErrInvalidTimezone error = errors.New("error invalid timezone set")
)

type Action int8

// Scheduler is an interface for anything that can evaluate a resource and return
// an action.
type Scheduler interface {
	// Evaluate takes input and returns a decision or error
	Evaluate(any) (Action, error)
	// SetLocation changes the timezone of the scheduler
	SetLocation(*time.Location) (Scheduler, error)
	// Type returns the type of scheduler in use
	Type() string
}

type NullScheduler struct{}

func (n *NullScheduler) Evaluate(any) (Action, error) {
	return NULL_ACTION, ErrNoScheduler
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
