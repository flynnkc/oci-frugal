package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
)

var (
	_WEEKDAYS map[string]bool = map[string]bool{
		"Monday":    true,
		"Tuesday":   true,
		"Wednesday": true,
		"Thursday":  true,
		"Friday":    true,
	}
	_WEEKENDS map[string]bool = map[string]bool{
		"Saturday": true,
		"Sunday":   true,
	}
)

const (
	_ANYDAY  string = "AnyDay"
	_WEEKDAY string = "WeekDay"
	_WEEKEND string = "Weekend"
)

// AnykeyNL Scheduler inspired by https://github.com/AnykeyNL/OCI-AutoScale and
// aims to have similar ruleset. Intended to run once an hour.
type AnykeyNLScheduler struct {
	loc  *time.Location
	hour int
	day  string
}

// NewAnykeyNLScheduler creates a scheduler using the local system timezone.
func NewAnykeyNLScheduler() Scheduler {
	return NewAnykeyNLSchedulerWithLocation(time.Local)
}

// NewAnykeyNLSchedulerWithLocation creates a scheduler with the provided
// timezone. If loc is nil, time.Local is used.
func NewAnykeyNLSchedulerWithLocation(loc *time.Location) *AnykeyNLScheduler {
	if loc == nil {
		loc = time.Local
	}

	// Determine current time components based on configured scheduler timezone
	now := time.Now().In(loc)

	return &AnykeyNLScheduler{
		loc:  loc,
		hour: now.Hour(),
		day:  now.Weekday().String(),
	}
}

// Evaluate determines an action to take on the resource. Input must be of type
// map[string]string.
func (ts AnykeyNLScheduler) Evaluate(tags any) (Action, error) {

	t, ok := tags.(map[string]string)
	if !ok {
		return NULL_ACTION, ErrInvalidInput
	}

	// Is today the day of the week?
	if tag, ok := t[ts.day]; ok && strings.TrimSpace(tag) != "" {
		return ts.parseSchedule(tag, ts.hour)
	}

	// Is today a weekday?
	if _, ok := _WEEKDAYS[ts.day]; ok {
		if tag, ok := t[_WEEKDAY]; ok && strings.TrimSpace(tag) != "" {
			return ts.parseSchedule(tag, ts.hour)
		}
	}

	// Is today a weekend?
	if _, ok := _WEEKENDS[ts.day]; ok {
		if tag, ok := t[_WEEKEND]; ok && strings.TrimSpace(tag) != "" {
			return ts.parseSchedule(tag, ts.hour)
		}
	}

	// Is today a day?
	if tag, ok := t[_ANYDAY]; ok && strings.TrimSpace(tag) != "" {
		return ts.parseSchedule(tag, ts.hour)
	}

	// No match, no action
	return NULL_ACTION, nil
}

// SetLocation changes the timezone of the scheduler
func (ts *AnykeyNLScheduler) SetLocation(loc *time.Location) (Scheduler, error) {
	if loc == nil {
		return ts, ErrInvalidTimezone
	}

	return NewAnykeyNLSchedulerWithLocation(loc), nil
}

// Type returns the scheduler type
func (ts *AnykeyNLScheduler) Type() string {
	return configuration.ANYKEYNL_SCHEDULER
}

func (ts AnykeyNLScheduler) parseSchedule(sch string, hour int) (Action, error) {
	// Default: null action
	act := NULL_ACTION

	// Empty or whitespace-only schedule
	sch = strings.TrimSpace(sch)
	if sch == "" {
		return act, nil
	}

	tokens := strings.Split(sch, ",")
	// Trim tokens
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	var want string
	switch {
	case len(tokens) == 0:
		want = "*" // nothing declared means no action
	case hour < len(tokens):
		want = tokens[hour]
	default:
		// Fewer than 24 tokens: repeat the last provided token
		want = tokens[len(tokens)-1]
	}

	if want == "*" || strings.TrimSpace(want) == "" {
		return act, nil
	}

	wantInt, err := strconv.Atoi(want)
	if err != nil {
		return act, fmt.Errorf("invalid schedule token %q in %q: %w", want, sch, err)
	}

	// Backward-compatible mapping and safety clamp for Action(int8)
	switch {
	case wantInt <= 0:
		act = OFF
	case wantInt == 1:
		act = ON
	default:
		// Clamp to int8 max to avoid overflow
		if wantInt > 127 {
			wantInt = 127
		}
		act = Action(int8(wantInt))
	}

	return act, nil
}
