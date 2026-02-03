package task

import (
	"github.com/flynnkc/oci-frugal/src/pkg/action"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

type Task struct {
	Action   action.Action
	Resource rs.ResourceSummary
}

func NewTask(act action.Action, item rs.ResourceSummary) Task {
	return Task{
		Action:   act,
		Resource: item,
	}
}
