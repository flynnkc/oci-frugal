package handler

import (
	"context"
	"errors"
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
	"github.com/oracle/oci-go-sdk/v65/resourcesearch"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
)

const (
	DEFAULT_INTERVAL     time.Duration = 15 * time.Second
	MAX_INTERVAL         time.Duration = 3 * time.Minute
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
	analytics   analytics.AnalyticsClient
	compute     core.ComputeClient
	database    database.DatabaseClient
	integration integration.IntegrationInstanceClient
	search      resourcesearch.ResourceSearchClient
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

	// Search (Required for DbSystems)
	s, err := rs.NewResourceSearchClientWithConfigurationProvider(cp)
	if err != nil {
		return nil, err
	}
	h.search = s

	return &h, nil
}

func (h *ResourceHandler) SetRegion(region string) {
	h.analytics.SetRegion(region)
	h.compute.SetRegion(region)
	h.database.SetRegion(region)
	h.integration.SetRegion(region)
	h.search.SetRegion(region)
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
	case "DbSystem":
		return h.handleDbSystem(t)
	case "AnalyticsInstance":
		return h.handleAnalyticsInstance(t)
	}

	return nil
}

// HandleCompute takes actions on compute resources. Limited to turning instance
// on or off.
func (h *ResourceHandler) handleCompute(t task.Task) error {
	logGroup := getResourceGroup(t)

	h.log.Debug("Handling Compute", logGroup)

	// Turn off if action is off and instance is not already off
	if t.Action == action.OFF && (*t.Resource.LifecycleState != "STOPPED" &&
		*t.Resource.LifecycleState != "STOPPING" &&
		*t.Resource.LifecycleState != "TERMINATING" &&
		*t.Resource.LifecycleState != "TERMINATED") {
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
		h.log.Info("Compute Handled",
			slog.String("Action", "STOP"),
			slog.String("Status Message", resp.RawResponse.Status),
			logGroup)

	} else if t.Action == action.ON && (*t.Resource.LifecycleState != "RUNNING" &&
		*t.Resource.LifecycleState != "STARTING" &&
		*t.Resource.LifecycleState != "TERMINATING" &&
		*t.Resource.LifecycleState != "TERMINATED") {
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
		h.log.Info("Compute Handled",
			slog.String("Action", "START"),
			slog.String("Status Message", resp.RawResponse.Status),
			logGroup)
	} else {
		h.log.Info("Compute Handled - No Action Required",
			slog.String("State", *t.Resource.LifecycleState),
			slog.String("Action", "NONE"), logGroup)
	}

	return nil
}

// handleDbSystem starts or stops Database Node resources.
func (h *ResourceHandler) handleDbSystem(t task.Task) error {
	nodes := h.getDbNodes(t.Resource.Identifier)

	// helper to safely dereference pointers for error messages
	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}

	var errs []error

	for _, node := range nodes {
		logGroup := getResourceGroup(task.NewTask(t.Action, node))
		h.log.Debug("Handling DB Node", logGroup)

		// Turn DB Node Off
		if t.Action == action.OFF && (*node.LifecycleState != "STOPPED" &&
			*node.LifecycleState != "STOPPING" &&
			*node.LifecycleState != "TERMINATING" &&
			*node.LifecycleState != "TERMINATED") {
			ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)

			req := database.DbNodeActionRequest{
				DbNodeId: node.Identifier,
				Action:   database.DbNodeActionActionStop,
			}

			resp, err := h.database.DbNodeAction(ctx, req)
			cancel()
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"stop dbnode %s (dbSystem %s, state %s) failed: %w",
					str(node.Identifier), str(t.Resource.Identifier), str(node.LifecycleState), err,
				))
				continue
			}

			h.log.Info("Handled DB Node",
				slog.String("Action", "STOP"),
				slog.String("Status", resp.RawResponse.Status),
				logGroup)
		} else if t.Action == action.ON && (*node.LifecycleState != "RUNNING" &&
			*node.LifecycleState != "STARTING" &&
			*node.LifecycleState != "TERMINATING" &&
			*node.LifecycleState != "TERMINATED") {
			// Turn DB Node On
			ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)

			req := database.DbNodeActionRequest{
				DbNodeId: node.Identifier,
				Action:   database.DbNodeActionActionStart,
			}

			resp, err := h.database.DbNodeAction(ctx, req)
			cancel()
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"start dbnode %s (dbSystem %s, state %s) failed: %w",
					str(node.Identifier), str(t.Resource.Identifier), str(node.LifecycleState), err,
				))
				continue
			}

			h.log.Info("Handled DB Node",
				slog.String("Action", "START"),
				slog.String("Status", resp.RawResponse.Status),
				logGroup)
		} else {
			h.log.Info("DB Node Handled - No Action Required",
				slog.String("State", *t.Resource.LifecycleState),
				slog.String("Action", "NONE"), logGroup)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(
			"dbSystem %s: one or more DB node actions failed: %w",
			str(t.Resource.Identifier), errors.Join(errs...),
		)
	}

	return nil
}

// handleAnalyticsInstance activates/deactivates OAC instances
func (h *ResourceHandler) handleAnalyticsInstance(t task.Task) error {
	logGroup := getResourceGroup(t)

	h.log.Debug("Handling Analytics Instance", logGroup)

	// Deactivate Analytics Instance
	if t.Action == action.OFF && *t.Resource.LifecycleState != "Inactive" &&
		*t.Resource.LifecycleState != "DELETED" {
		ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)
		defer cancel()

		req := analytics.StopAnalyticsInstanceRequest{
			AnalyticsInstanceId: t.Resource.Identifier,
		}

		resp, err := h.analytics.StopAnalyticsInstance(ctx, req)
		if err != nil {
			return err
		}

		h.log.Info("Stopped Analytics Instance",
			slog.String("Action", "STOP"),
			slog.String("Status", resp.RawResponse.Status),
			logGroup)
	} else if t.Action == action.ON && *t.Resource.LifecycleState != "RUNNING" &&
		*t.Resource.LifecycleState != "DELETED" {
		ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)
		defer cancel()

		req := analytics.StartAnalyticsInstanceRequest{
			AnalyticsInstanceId: t.Resource.Identifier,
		}

		resp, err := h.analytics.StartAnalyticsInstance(ctx, req)
		if err != nil {
			return err
		}

		h.log.Info("Started Analytics Instance",
			slog.String("Action", "START"),
			slog.String("Status", resp.RawResponse.Status),
			logGroup)
	} else {
		h.log.Info("Analytics Instance Handled - No Action Required",
			slog.String("Action", "NONE"),
			slog.String("State", *t.Resource.LifecycleState),
			logGroup)
	}

	return nil
}

func getResourceGroup(t task.Task) slog.Attr {
	return slog.Group("Resource",
		slog.String("ID", *t.Resource.Identifier),
		slog.String("Type", *t.Resource.ResourceType),
		slog.Int("Action", int(t.Action)),
		slog.String("State", *t.Resource.LifecycleState),
	)
}

func (h *ResourceHandler) getDbNodes(id *string) []rs.ResourceSummary {
	nodes := make([]rs.ResourceSummary, 0)

	query := fmt.Sprintf(
		"query dbnode resources return alladditionalfields where dbSystemId = '%s'",
		*id)

	h.log.Debug("Searching for DbSystem Nodes",
		slog.String("Query", query),
		slog.String("DbSystem ID", *id))

	details := rs.StructuredSearchDetails{
		Query: common.String(query),
	}

	req := rs.SearchResourcesRequest{
		SearchDetails: details,
	}

	ctx, cancel := context.WithTimeout(context.Background(), DEFAULT_INTERVAL)
	defer cancel()
	resp, err := h.search.SearchResources(ctx, req)
	if err != nil {
		h.log.Error("Error searching for DB Nodes",
			"error", err)
		return nodes
	} else if resp.RawResponse.StatusCode > 299 || resp.RawResponse.StatusCode < 200 {
		h.log.Error("Error searching for DB nodes: invalid status in response",
			slog.Int("code", resp.RawResponse.StatusCode))
	}

	for _, item := range resp.Items {
		h.log.Debug("appending item",
			slog.String("DB System", *id),
			slog.String("Node", *item.Identifier))
		nodes = append(nodes, item)
	}

	h.log.Debug("Returning search for DB Nodes",
		slog.String("DB System", *id),
		slog.Int("Count", len(nodes)))

	return nodes
}
