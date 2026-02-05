package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/integration"
)

const TIMEOUT time.Duration = 5 * time.Second

type Handler interface {
	HandleResource(task.Task) error
	SetRegion(string)
}

type HandlerOpts struct {
	ConfigProvider common.ConfigurationProvider
	Logger         *slog.Logger
}

type ResourceHandler struct {
	compute     core.ComputeClient
	database    database.DatabaseClient
	analytics   analytics.AnalyticsClient
	integration integration.IntegrationInstanceClient
	log         *slog.Logger
}

func NewResourceHandler(opts HandlerOpts) (*ResourceHandler, error) {
	h := ResourceHandler{}

	if opts.Logger != nil {
		h.log = opts.Logger
	}

	if opts.ConfigProvider == nil {
		return nil, fmt.Errorf("error Handler cannot have nil ConfigProvider")
	}

	cp := opts.ConfigProvider

	// Compute
	instance, err := core.NewComputeClientWithConfigurationProvider(cp)
	if err != nil {
		return nil, err
	}
	h.compute = instance

	// Database
	db, err := database.NewDatabaseClientWithConfigurationProvider(cp)
	if err != nil {
		return nil, err
	}
	h.database = db

	// Analytics Cloud
	a, err := analytics.NewAnalyticsClientWithConfigurationProvider(cp)
	if err != nil {
		return nil, err
	}
	h.analytics = a

	// Integration Cloud
	i, err := integration.NewIntegrationInstanceClientWithConfigurationProvider(cp)
	if err != nil {
		return nil, err
	}
	h.integration = i

	return &h, nil
}

// HandleResource routes task to the appropriate handler
func (h *ResourceHandler) HandleResource(t task.Task) error {
	if t.Resource.ResourceType == nil {
		return fmt.Errorf("nil resource")
	}
	switch strings.ToLower(*t.Resource.ResourceType) {
	case "instance":
		return h.handleCompute(t)
	}

	return nil
}

func (h *ResourceHandler) SetRegion(region string) {
	h.analytics.SetRegion(region)
	h.compute.SetRegion(region)
	h.database.SetRegion(region)
	h.integration.SetRegion(region)
}

// HandleCompute takes actions on compute resources. Limited to turning instance
// on or off.
func (h *ResourceHandler) handleCompute(t task.Task) error {
	h.log.Debug("Handling Compute",
		slog.String("Resource ID", *t.Resource.Identifier),
		slog.Int("Action", int(t.Action)))

	// Turn off
	if t.Action == action.OFF {
		ctx, cancel := contextWithTimeout()
		defer cancel()

		req := core.InstanceActionRequest{
			InstanceId: t.Resource.Identifier,
			Action:     core.InstanceActionActionStop,
		}

		resp, err := h.compute.InstanceAction(ctx, req)
		if err != nil {
			return err
		}
		h.log.Debug("Compute Handled",
			slog.String("Resource ID", *t.Resource.Identifier),
			slog.String("Status Message", resp.RawResponse.Status),
			slog.Int("Action", int(t.Action)))

		return nil
	}

	// Else turn on -- no vertical scaling supported at this time
	ctx, cancel := contextWithTimeout()
	defer cancel()

	req := core.InstanceActionRequest{
		InstanceId: t.Resource.Identifier,
		Action:     core.InstanceActionActionStart,
	}

	resp, err := h.compute.InstanceAction(ctx, req)
	if err != nil {
		return err
	}
	h.log.Debug("Compute Handled",
		slog.String("Resource ID", *t.Resource.Identifier),
		slog.String("Status Message", resp.RawResponse.Status),
		slog.Int("Action", int(t.Action)))

	return nil
}

// Get contexts with timeout
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), TIMEOUT)
}
