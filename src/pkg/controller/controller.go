package controller

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
)

const (
	numWorkers int8 = 16
)

var (
	ErrControllerOptions error = fmt.Errorf("missing one or more required options on controller")
)

type Controller interface {
	SetScheduler(scheduler.Scheduler) *Controller
	Run(sync.WaitGroup)
}

type ControllerOpts struct {
	ConfigurationProvider common.ConfigurationProvider // Required
	TagNamespace          *string                      // Required
	Scheduler             scheduler.Scheduler          // Optional
	Log                   *slog.Logger                 // Optional
}
