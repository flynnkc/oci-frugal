package scheduler

import (
	"errors"
	"fmt"
)

var (
	ErrNoScheduler     error = errors.New("error no scheduler set")
	ErrInvalidTimezone error = errors.New("error invalid timezone set")
)

type ErrInvalidInput struct {
	Input any
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("error invalid input to scheduler: %v", e.Input)
}
