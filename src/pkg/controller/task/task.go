package task

import (
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

type Task struct {
	Action   scheduler.Action
	Resource rs.ResourceSummary
}
