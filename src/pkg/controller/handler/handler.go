package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	tokenpool "github.com/flynnkc/token-pool"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/integration"
)

const (
	DEFAULT_INTERVAL     time.Duration = 3 * time.Second
	MAX_INTERVAL         time.Duration = 30 * time.Second
	DEFAULT_MAX_REQUESTS int           = 8
)

type Handler interface {
	HandleResource(task.Task) error
	SetRegion(string)
}

type HandlerOpts struct {
	ConfigProvider  common.ConfigurationProvider
	Logger          *slog.Logger
	MaxRequests     *int
	RequestInterval *time.Duration // 1-30 Seconds
}

type ResourceHandler struct {
	compute     core.ComputeClient
	database    database.DatabaseClient
	analytics   analytics.AnalyticsClient
	integration integration.IntegrationInstanceClient
	log         *slog.Logger
	tp          *tokenpool.TokenPool
}

func NewResourceHandler(opts HandlerOpts) (*ResourceHandler, error) {
	h := ResourceHandler{}

	if opts.Logger != nil {
		h.log = opts.Logger
	} else {
		h.log = slog.Default()
		h.log.Warn("Handler Logger set to Default")
	}

	h.log.Debug("Creating Handler")

	if opts.MaxRequests == nil {
		opts.MaxRequests = common.Int(DEFAULT_MAX_REQUESTS)
	}

	if opts.RequestInterval == nil {
		t := DEFAULT_INTERVAL
		opts.RequestInterval = &t
	}

	h.tp = tokenpool.NewTokenPool(*opts.MaxRequests, *opts.MaxRequests, *opts.RequestInterval)

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

func (h *ResourceHandler) SetRegion(region string) {
	h.analytics.SetRegion(region)
	h.compute.SetRegion(region)
	h.database.SetRegion(region)
	h.integration.SetRegion(region)
}

// HandleResource routes task to the appropriate handler
func (h *ResourceHandler) HandleResource(t task.Task) error {
	h.log.Debug("Handling Resource",
		"Type", *t.Resource.ResourceType)
	if t.Resource.ResourceType == nil {
		return fmt.Errorf("nil resource")
	}

	// Require token for rate limiting
	ctx, cancel := context.WithTimeout(context.Background(), MAX_INTERVAL)
	defer cancel()
	h.tp.Acquire(ctx)

	switch *t.Resource.ResourceType {
	case "Instance":
		return h.handleCompute(t)
	}

	return nil
}

// HandleCompute takes actions on compute resources. Limited to turning instance
// on or off.
func (h *ResourceHandler) handleCompute(t task.Task) error {
	resourceGroup := slog.Group("Resource",
		slog.String("ID", *t.Resource.Identifier),
		slog.Int("Action", int(t.Action)),
		slog.String("State", *t.Resource.LifecycleState),
	)

	h.log.Debug("Handling Compute", resourceGroup)

	// Turn off if action is off and instance is not already off
	if t.Action == action.OFF && (*t.Resource.LifecycleState != "STOPPED" &&
		*t.Resource.LifecycleState != "STOPPING") {
		ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)
		defer cancel()

		req := core.InstanceActionRequest{
			InstanceId: t.Resource.Identifier,
			Action:     core.InstanceActionActionStop,
		}

		resp, err := h.compute.InstanceAction(ctx, req)
		if err != nil {
			return err
		}
		h.log.Debug("Compute Handled", resourceGroup,
			slog.String("Status Message", resp.RawResponse.Status),
			slog.String("Action", "STOP"))

		return nil
	} else if t.Action == action.ON && *t.Resource.LifecycleState != "RUNNING" {
		// Else turn on -- no vertical scaling supported at this time
		ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)
		defer cancel()

		req := core.InstanceActionRequest{
			InstanceId: t.Resource.Identifier,
			Action:     core.InstanceActionActionStart,
		}

		resp, err := h.compute.InstanceAction(ctx, req)
		if err != nil {
			return err
		}
		h.log.Debug("Compute Handled", resourceGroup,
			slog.String("Status Message", resp.RawResponse.Status),
			slog.String("Action", "START"))

		return nil
	}

	h.log.Debug("No Action Required", resourceGroup,
		slog.String("Action", "NONE"))

	return nil
}
