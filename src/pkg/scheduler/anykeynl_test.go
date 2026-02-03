package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act != action.NULL_ACTION {
		t.Fatalf("expected NULL_ACTION, got %v", act)
	}
}

func TestEvaluate_Enforce24Tokens(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	// Too few tokens
	act, err := sch.Evaluate(map[string]string{"AnyDay": "0,1,1"})
	if err == nil {
		t.Fatalf("expected error for invalid token count, got nil")
	}
	if _, ok := err.(ErrInvalidTokenCount); !ok {
		t.Fatalf("expected ErrInvalidTokenCount, got %T: %v", err, err)
	}
	if act != action.NULL_ACTION {
		t.Fatalf("expected NULL_ACTION on error, got %v", act)
	}

	// Too many tokens (25)
	act, err = sch.Evaluate(map[string]string{"AnyDay": repeat24("1") + ",1"})
	if err == nil {
		t.Fatalf("expected error for invalid token count (too many), got nil")
	}
	if _, ok := err.(ErrInvalidTokenCount); !ok {
		t.Fatalf("expected ErrInvalidTokenCount, got %T: %v", err, err)
	}
	if act != action.NULL_ACTION {
		t.Fatalf("expected NULL_ACTION on error, got %v", act)
	}
}

func TestEvaluate_StarMeansNoop(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("*")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act != action.NULL_ACTION {
		t.Fatalf("expected NULL_ACTION for '*', got %v", act)
	}
}

func TestEvaluate_NumericMapping(t *testing.T) {
	sch := NewAnykeyNLScheduler()

	// all zeros => OFF
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("0")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act != action.OFF {
		t.Fatalf("expected OFF, got %v", act)
	}

	// all ones => ON
	act, err = sch.Evaluate(map[string]string{"AnyDay": repeat24("1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act != action.ON {
		t.Fatalf("expected ON, got %v", act)
	}

	// value > 1 => custom Action
	act, err = sch.Evaluate(map[string]string{"AnyDay": repeat24("3")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(act) != 3 {
		t.Fatalf("expected custom action 3, got %v", act)
	}
}

func TestEvaluate_UnsupportedParenthesis(t *testing.T) {
	sch := NewAnykeyNLScheduler()
	act, err := sch.Evaluate(map[string]string{"AnyDay": repeat24("(1:2)")})
	if err == nil {
		t.Fatalf("expected error for unsupported token, got nil")
	}
	if _, ok := err.(ErrUnsupportedToken); !ok {
		t.Fatalf("expected ErrUnsupportedToken, got %T: %v", err, err)
	}
	if act != action.NULL_ACTION {
		t.Fatalf("expected NULL_ACTION on error, got %v", act)
	}
}

func TestEvaluate_DayOfMonth(t *testing.T) {
	// Build DayOfMonth string matching today's day
	today := time.Now().In(time.Local).Day()
	sch := NewAnykeyNLScheduler()
	domStr := fmt.Sprintf("%d:1", today)
	act, err := sch.Evaluate(map[string]string{"DayOfMonth": domStr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act != action.ON {
		t.Fatalf("expected ON from DayOfMonth match, got %v", act)
	}
}
