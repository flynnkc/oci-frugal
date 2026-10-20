package scheduler

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to make a 24-token schedule of the same value
func repeat24(val string) string {
	s := make([]byte, 0, len(val)*24+23)
	for i := 0; i < 24; i++ {
		if i > 0 {
			s = append(s, ',')
		}
		s = append(s, val...)
	}
	return string(s)
}

func TestEvaluate_NoActiveSchedule(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(map[string]string{"SomeOther": repeat24("1")})
	require.NoError(t, err)
	assert.Equal(t, action.NULL_ACTION, act)
}

func TestEvaluate_Enforce24Tokens(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	// Too few tokens
	act, err := sch.Evaluate(map[string]string{"AnyDay": "0,1,1"})
	require.Error(t, err)
	assert.IsType(t, ErrInvalidTokenCount{}, err)
	assert.Equal(t, action.NULL_ACTION, act)

	// Too many tokens (25)
	act, err = sch.Evaluate(map[string]string{"AnyDay": repeat24("1") + ",1"})
	require.Error(t, err)
	assert.IsType(t, ErrInvalidTokenCount{}, err)
	assert.Equal(t, action.NULL_ACTION, act)
}

func TestEvaluate_StarMeansNoop(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("*")})
	require.NoError(t, err)
	assert.Equal(t, action.NULL_ACTION, act)
}

func TestEvaluate_NumericMapping(t *testing.T) {
	sch := NewAnykeyNLScheduler()

	// all zeros => OFF
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("0")})
	require.NoError(t, err)
	assert.Equal(t, action.OFF, act)

	// all ones => ON
	act, err = sch.Evaluate(map[string]string{"AnyDay": repeat24("1")})
	require.NoError(t, err)
	assert.Equal(t, action.ON, act)

	// Capacity values must not alias power flags before scaling is implemented.
	for _, token := range []string{"2", "3", "4", "128", "-1", "", "(4:8)"} {
		act, err = sch.Evaluate(repeat24(token))
		assert.Error(t, err, "token %q", token)
		assert.Equal(t, action.NULL_ACTION, act, "token %q", token)
	}

}

func TestEvaluate_UnsupportedParenthesis(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("(1:2)")})
	require.Error(t, err)
	assert.IsType(t, ErrUnsupportedToken{}, err)
	assert.Equal(t, action.NULL_ACTION, act)
}

func TestEvaluate_DayOfMonth(t *testing.T) {
	// Build DayOfMonth string matching today's day
	today := time.Now().In(time.Local).Day()
	sch := NewAnykeyNLScheduler()
	domStr := fmt.Sprintf("%d:1", today)
	act, err := sch.Evaluate(map[string]string{"DayOfMonth": domStr})
	require.NoError(t, err)
	assert.Equal(t, action.ON, act)
}

func TestEvaluate_DirectScheduleString_OK(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(repeat24("1"))
	require.NoError(t, err)
	assert.Equal(t, action.ON, act)
}

func TestEvaluate_DirectScheduleString_InvalidCount(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate("0,1,1")
	require.Error(t, err)
	assert.IsType(t, ErrInvalidTokenCount{}, err)
	assert.Equal(t, action.NULL_ACTION, act)
}

func TestTimezoneAndPrecedence(t *testing.T) {
	// UTC Monday becomes Sunday evening in New York, including DST.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	s := NewAnykeyNLSchedulerAt(time.Date(2026, 3, 9, 1, 0, 0, 0, time.UTC), loc)
	assert.Equal(t, "Sunday", s.dow)
	assert.Equal(t, 21, s.hour)
	assert.Equal(t, 8, s.dom)
	assert.Equal(t, 2, s.dnr)
	tags := map[string]string{
		"AnyDay":     repeat24("0"),
		"Weekend":    repeat24("1"),
		"Sunday":     repeat24("0"),
		"Sunday2":    repeat24("1"),
		"DayOfMonth": "8:0",
	}
	for _, key := range []string{"DayOfMonth", "Sunday2", "Sunday", "Weekend", "AnyDay"} {
		want := tags[key]
		if key == "DayOfMonth" {
			want = repeat24("0")
		}
		got, err := s.ActiveSchedule(tags)
		require.NoError(t, err, "key %s", key)
		assert.Equal(t, want, got, "key %s", key)
		delete(tags, key)
	}
}

func TestInvalidScheduleFailsClosed(t *testing.T) {
	s := NewAnykeyNLSchedulerAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	// A malformed later hour must prevent a valid current-hour stop.
	tokens := strings.Split(repeat24("0"), ",")
	tokens[23] = "bad"
	invalid := strings.Join(tokens, ",")
	a, err := s.Evaluate(invalid)
	assert.Error(t, err)
	assert.Equal(t, action.NULL_ACTION, a)
	for _, v := range []string{"bad", "0:1", "32:1", "1:0,1:1", "1:2", "2:"} {
		a, err := s.Evaluate(map[string]string{"AnyDay": repeat24("0"), "DayOfMonth": v})
		assert.Error(t, err, "DayOfMonth %q", v)
		assert.Equal(t, action.NULL_ACTION, a, "DayOfMonth %q", v)
	}
	a, err = s.Evaluate(repeat24("1") + " # test comment")
	require.NoError(t, err)
	assert.Equal(t, action.ON, a)
}
