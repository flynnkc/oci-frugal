package action

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFiltersNeverInterpretCapacityAsPower(t *testing.T) {
	for _, filter := range []Action{ALL, ON, OFF, NULL_ACTION} {
		for _, decision := range []Action{ON, OFF, NULL_ACTION, 3, 4, 127, 255} {
			want := (decision == ON || decision == OFF) && (filter == ALL || filter == decision)
			assert.Equal(
				t,
				want,
				Compare(filter, decision),
				"filter %d decision %d",
				filter,
				decision,
			)
		}
	}
}
