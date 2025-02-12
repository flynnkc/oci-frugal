package scheduler

import (
	"strconv"
	"strings"
	"time"
)

const (
	NULL_ACTION int8 = -1
	OFF         int8 = -2
	ON          int8 = -3
)

type Scheduler interface {
	Evaluate(...string) int8
}

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

func (ts AnykeyNLScheduler) Evaluate(tags map[string]interface{}) int8 {

	// Is today the day of the week?
	if tag, ok := tags[ts.day].(string); ok {
		return ts.parseSchedule(tag)
	}

	// Is today a weekday?
	weekdays := []string{
		"Monday",
		"Tuesday",
		"Wednesday",
		"Thursday",
		"Friday",
	}

	for _, day := range weekdays {
		if ts.day == day {
			if tag, ok := tags["Weekday"].(string); ok {
				return ts.parseSchedule(tag)
			}
		}
	}

	// Is today a weekend?
	weekends := []string{
		"Saturday",
		"Sunday",
	}

	for _, day := range weekends {
		if ts.day == day {
			if tag, ok := tags["Weekend"].(string); ok {
				return ts.parseSchedule(tag)
			}
		}
	}

	// Is today a day?
	if tag, ok := tags["AnyDay"].(string); ok {
		return ts.parseSchedule(tag)
	}

	return NULL_ACTION
}

func (ts AnykeyNLScheduler) parseSchedule(sch string) (act int8) {
	defer func() {
		// If panic then assume off; might be a little bit spiteful for passing
		// a string that causes panic.
		if r := recover(); r != nil {
			act = OFF
		}
	}()

	act = NULL_ACTION

	s := strings.Split(sch, ",")

	want := s[ts.hour]
	if want == "*" {
		return
	}

	wantInt, err := strconv.Atoi(want)
	if err != nil || wantInt < 1 {
		act = OFF
	} else if wantInt == 0 {
		act = ON
	} else {
		act = int8(wantInt)
	}

	return
}
