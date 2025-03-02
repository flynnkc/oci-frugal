package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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
	now  time.Time
	hour int
	day  string
}

func NewAnykeyNLScheduler() AnykeyNLScheduler {
	// TODO add timezone support
	t := time.Now()
	return AnykeyNLScheduler{
		now:  t,
		hour: t.Hour(),
		day:  t.Weekday().String(),
	}
}

func (ts AnykeyNLScheduler) Evaluate(tags AnykeyNLInput) Action {

	// Is today the day of the week?
	if tag, ok := tags[ts.day]; ok {
		return ts.parseSchedule(tag)
	}

	// Is today a weekday?
	if _, ok := _WEEKDAYS[ts.day]; ok {
		return ts.parseSchedule(tags[_WEEKDAY])
	}

	// Is today a weekend?
	if _, ok := _WEEKENDS[ts.day]; ok {
		return ts.parseSchedule(tags[_WEEKEND])
	}

	// Is today a day?
	if tag, ok := tags[_ANYDAY]; ok {
		return ts.parseSchedule(tag)
	}

	// No match, no action
	return NULL_ACTION
}

func (ts AnykeyNLScheduler) parseSchedule(sch string) (act Action) {
	defer func() {
		// If panic then assume off; might be a little bit spiteful for passing
		// a string that causes panic.
		if r := recover(); r != nil {
			act = OFF
			fmt.Printf("Panic parsing schedule %v: %v\n", sch, r)
		}
	}()

	// return null action by default
	act = NULL_ACTION

	s := strings.Split(sch, ",")

	want := s[ts.hour]
	// No action requested; return default null action
	if want == "*" {
		return
	}

	wantInt, err := strconv.Atoi(want)
	if err != nil {
		fmt.Printf("Error decoding schedule %v: %v", sch, err)
		return
	}

	switch {
	case wantInt < 1:
		act = OFF
	case wantInt == 1:
		act = ON
	case wantInt > 1:
		act = Action(wantInt)
	}

	return
}

type AnykeyNLInput map[string]string

func NewAnykeyNLInput(in map[string]any) AnykeyNLInput {
	m := make(AnykeyNLInput)

	for key := range in {
		if t, ok := in[key].(string); ok {
			m[key] = t
		}
	}

	return m
}

func (a *AnykeyNLInput) Parse() string {
	s := ""

	for key, val := range *a {
		s += fmt.Sprintf("%s - %s\n", key, val)
	}

	return s
}
