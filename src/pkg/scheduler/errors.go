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

// ErrInvalidTokenCount indicates the schedule did not contain exactly 24
// comma-separated tokens for each hour of the day.
type ErrInvalidTokenCount struct {
	Expected int
	Got      int
}

func (e ErrInvalidTokenCount) Error() string {
	return fmt.Sprintf("invalid schedule: expected %d hourly tokens, got %d", e.Expected, e.Got)
}

// ErrInvalidToken indicates the schedule contained a token that cannot be
// interpreted for the current hour.
type ErrInvalidToken struct {
	Token  string
	Reason string
}

func (e ErrInvalidToken) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("invalid schedule token: %q", e.Token)
	}
	return fmt.Sprintf("invalid schedule token %q: %s", e.Token, e.Reason)
}

// ErrUnsupportedToken indicates a syntactically recognized but unsupported
// token (e.g., parenthesized flex shape sizing).
type ErrUnsupportedToken struct {
	Token string
}

func (e ErrUnsupportedToken) Error() string {
	return fmt.Sprintf("unsupported schedule token: %q", e.Token)
}
