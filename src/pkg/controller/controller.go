package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/configuration"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/handler"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
)

var ErrControllerOptions = fmt.Errorf("missing or invalid controller options")

type Controller interface {
	Plan(context.Context) (Result, error)
	Execute(context.Context, Result) (Result, error)
	Close()
}

// Result records planning and submission separately from asynchronous completion.
type Result struct {
	Region     string           `json:"region"`
	DryRun     bool             `json:"dry_run"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at"`
	Resources  []ResourceResult `json:"resources"`
	Error      string           `json:"error,omitempty"`
}
type ResourceResult struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Action     string              `json:"action"`
	Status     string              `json:"status"`
	Operations []handler.Operation `json:"operations,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type ControllerOpts struct {
	Rate                  float64 // Zero selects the default.
	Burst                 int     // Zero selects the default.
	ConfigurationProvider common.ConfigurationProvider
	TagNamespace          *string
	Region                *string
	Scheduler             scheduler.Scheduler
	SupportedActions      action.Action
	LogFunc               configuration.LogFunc
	Shutdown              string
}
