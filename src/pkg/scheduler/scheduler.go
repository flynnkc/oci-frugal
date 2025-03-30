package scheduler

import "errors"

// Actions are int8 and we reserve positive integers so constants are all <0
const (
	OFF         Action = 0
	ON          Action = -1
	NULL_ACTION Action = -2
)

var (
	ErrInvalidInput error = errors.New("error invalid input in scheduler")
	ErrNoScheduler  error = errors.New("error no scheduler set")
)

type Action int8

// Scheduler is an interface for anything that can evaluate a resource and return
// an action.
type Scheduler interface {
	Evaluate(any) (Action, error)
}

type NullScheduler struct{}

func (n *NullScheduler) Evaluate(any) (Action, error) {
	return NULL_ACTION, ErrNoScheduler
}
