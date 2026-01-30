package controller

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
)

var (
	ErrControllerOptions error = fmt.Errorf("missing one or more required options on controller")
)

type Controller interface {
	SetScheduler(scheduler.Scheduler) *Controller
	Run(sync.WaitGroup)
}

// Options to provide controllers to define behavior. Controller should define
// required and optional attributes.
type ControllerOpts struct {
	ConfigurationProvider common.ConfigurationProvider
	TagNamespace          *string
	Region                *string
	Scheduler             scheduler.Scheduler
	Log                   *slog.Logger
}
