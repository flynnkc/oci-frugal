package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
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
	_DAYOFMO string = "DayOfMonth"
)

// AnykeyNL Scheduler inspired by https://github.com/AnykeyNL/OCI-AutoScale and
// aims to have similar ruleset. Intended to run once an hour.
type AnykeyNLScheduler struct {
	loc  *time.Location
	hour int
	dow  string // day of week
	dom  int    // day of month
	dnr  int    // nth day within the month (1..5)
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
		dow:  now.Weekday().String(),
		dom:  now.Day(),
		dnr:  nthInMonth(now),
	}
}

// Evaluate determines an action to take on the resource.
// Input may be either:
//   - string (or []byte / fmt.Stringer): a 24-token schedule, parsed directly
//   - map[string]string or map[string]interface{}: tags to resolve via ActiveSchedule
func (ts AnykeyNLScheduler) Evaluate(input any) (action.Action, error) {
	switch v := input.(type) {
	case string:
		// Direct evaluation of schedule string
		return ts.parseSchedule(v, ts.hour)
	case []byte:
		return ts.parseSchedule(string(v), ts.hour)
	case fmt.Stringer:
		return ts.parseSchedule(v.String(), ts.hour)
	default:
		// Treat as tags and resolve today's active schedule
		active, err := ts.ActiveSchedule(input)
		if err != nil {
			return action.NULL_ACTION, err
		}

		if strings.TrimSpace(active) == "" {
			// No active schedule today
			return action.NULL_ACTION, nil
		}

		return ts.parseSchedule(active, ts.hour)
	}
}

// ActiveSchedule determines the active schedule per AnykeyNL priority
// (least -> most specific), with later matches overriding earlier ones:
// AnyDay -> WeekDay/Weekend -> Day-of-week -> Nth day-of-week in month -> DayOfMonth
func (ts AnykeyNLScheduler) ActiveSchedule(tags any) (string, error) {
	// Normalize tags into map[string]string
	t, err := toStringMap(tags)
	if err != nil {
		return "", err
	}

	active := ""

	if v, ok := t[_ANYDAY]; ok && strings.TrimSpace(v) != "" {
		active = v
	}
	if _, ok := _WEEKDAYS[ts.dow]; ok {
		if v, ok := t[_WEEKDAY]; ok && strings.TrimSpace(v) != "" {
			active = v
		}
	} else if _, ok := _WEEKENDS[ts.dow]; ok {
		if v, ok := t[_WEEKEND]; ok && strings.TrimSpace(v) != "" {
			active = v
		}
	}
	if v, ok := t[ts.dow]; ok && strings.TrimSpace(v) != "" { // exact day name
		active = v
	}
	nthKey := fmt.Sprintf("%s%d", ts.dow, ts.dnr) // e.g., Monday2
	if v, ok := t[nthKey]; ok && strings.TrimSpace(v) != "" {
		active = v
	}
	if v, ok := t[_DAYOFMO]; ok && strings.TrimSpace(v) != "" {
		if rep, ok := dayOfMonthOverride(v, ts.dom); ok {
			active = rep
		}
	}

	return active, nil
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

func (ts AnykeyNLScheduler) parseSchedule(sch string, hour int) (action.Action,
	error) {
	// Default: null action
	act := action.NULL_ACTION

	// Remove inline comment support like "... # comment"
	if idx := strings.Index(sch, "#"); idx >= 0 {
		sch = sch[:idx]
	}

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

	// Enforce exactly 24 tokens
	if len(tokens) != 24 {
		return act, ErrInvalidTokenCount{Expected: 24, Got: len(tokens)}
	}

	want := tokens[hour]
	if want == "" || want == "*" {
		return act, nil
	}

	// Parenthesized tokens are not supported at this layer
	if strings.HasPrefix(want, "(") && strings.HasSuffix(want, ")") {
		return act, ErrUnsupportedToken{Token: want}
	}

	wantInt, err := strconv.Atoi(want)
	if err != nil {
		return act, ErrInvalidToken{Token: want, Reason: err.Error()}
	}

	// Map numeric to action
	switch {
	case wantInt <= 0:
		act = action.OFF
	case wantInt == 1:
		act = action.ON
	default:
		if wantInt > 127 {
			wantInt = 127 // clamp to int8
		}
		act = action.ToAction(wantInt)
	}

	return act, nil
}

// toStringMap normalizes expected OCI defined tag maps into a map[string]string.
func toStringMap(tags any) (map[string]string, error) {
	switch tt := tags.(type) {
	case map[string]string:
		return tt, nil
	case map[string]interface{}:
		out := make(map[string]string, len(tt))
		for k, v := range tt {
			if v == nil {
				continue
			}
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
		return out, nil
	default:
		return nil, ErrInvalidInput{Input: tags}
	}
}

// nthInMonth returns the 1-based occurrence of the weekday within the month for t.
func nthInMonth(t time.Time) int {
	return ((t.Day() - 1) / 7) + 1
}

// dayOfMonthOverride parses a DayOfMonth schedule value like "1:0,15:1" and
// if the current day matches, returns a repeated 24-hour schedule string
// like "1,1,1,..." and true. Otherwise returns "", false.
func dayOfMonthOverride(v string, today int) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			// invalid pair is a hard error in strict mode; keep backward compatible by ignoring here
			// and let overall schedule validation catch issues if chosen.
			continue
		}
		dstr := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		d, err := strconv.Atoi(dstr)
		if err != nil {
			continue
		}
		if d == today {
			// Build a 24-token repeated schedule
			if val == "" {
				return "", false
			}
			tokens := make([]string, 24)
			for i := range tokens {
				tokens[i] = val
			}
			return strings.Join(tokens, ","), true
		}
	}
	return "", false
}
