package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/flynnkc/oci-frugal/src/pkg/ociapi"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
)

// Operation describes one concrete power request. Submitted means OCI accepted
// the request; it does not imply that the asynchronous transition has completed.
type Operation struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id,omitempty"`
	Type      string `json:"type"`
	Action    string `json:"action"`
	State     string `json:"state,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type Handler interface {
	Plan(context.Context, task.Task) ([]Operation, error)
	Execute(context.Context, Operation) (Operation, error)
	SetRegion(string)
}

type ComputeAPI interface {
	GetInstance(context.Context, core.GetInstanceRequest) (core.GetInstanceResponse, error)
	InstanceAction(
		context.Context,
		core.InstanceActionRequest,
	) (core.InstanceActionResponse, error)
	SetRegion(string)
}
type DatabaseAPI interface {
	GetDbSystem(
		context.Context,
		database.GetDbSystemRequest,
	) (database.GetDbSystemResponse, error)
	ListDbNodes(
		context.Context,
		database.ListDbNodesRequest,
	) (database.ListDbNodesResponse, error)
	GetDbNode(context.Context, database.GetDbNodeRequest) (database.GetDbNodeResponse, error)
	DbNodeAction(
		context.Context,
		database.DbNodeActionRequest,
	) (database.DbNodeActionResponse, error)
	SetRegion(string)
}
type AnalyticsAPI interface {
	GetAnalyticsInstance(
		context.Context,
		analytics.GetAnalyticsInstanceRequest,
	) (analytics.GetAnalyticsInstanceResponse, error)
	StartAnalyticsInstance(
		context.Context,
		analytics.StartAnalyticsInstanceRequest,
	) (analytics.StartAnalyticsInstanceResponse, error)
	StopAnalyticsInstance(
		context.Context,
		analytics.StopAnalyticsInstanceRequest,
	) (analytics.StopAnalyticsInstanceResponse, error)
	SetRegion(string)
}

type ResourceHandler struct {
	compute   ComputeAPI
	database  DatabaseAPI
	analytics AnalyticsAPI
	shutdown  core.InstanceActionActionEnum
}

type HandlerOpts struct {
	Log            *slog.Logger
	ConfigProvider common.ConfigurationProvider
	Limiter        ociapi.Limiter
	Shutdown       string
}

func NewResourceHandler(opts HandlerOpts) (*ResourceHandler, error) {
	if opts.ConfigProvider == nil || opts.Limiter == nil {
		return nil, fmt.Errorf("provider and limiter are required")
	}
	shutdown := core.InstanceActionActionSoftstop
	switch opts.Shutdown {
	case "", "soft":
	case "hard":
		shutdown = core.InstanceActionActionStop
	default:
		return nil, fmt.Errorf("invalid shutdown policy %q", opts.Shutdown)
	}
	compute, err := core.NewComputeClientWithConfigurationProvider(opts.ConfigProvider)
	if err != nil {
		return nil, err
	}
	db, err := database.NewDatabaseClientWithConfigurationProvider(opts.ConfigProvider)
	if err != nil {
		return nil, err
	}
	a, err := analytics.NewAnalyticsClientWithConfigurationProvider(opts.ConfigProvider)
	if err != nil {
		return nil, err
	}
	compute.HTTPClient = ociapi.NewDispatcher(compute.HTTPClient, opts.Limiter, opts.Log)
	db.HTTPClient = ociapi.NewDispatcher(db.HTTPClient, opts.Limiter, opts.Log)
	a.HTTPClient = ociapi.NewDispatcher(a.HTTPClient, opts.Limiter, opts.Log)
	return &ResourceHandler{
		compute:   &compute,
		database:  &db,
		analytics: &a,
		shutdown:  shutdown,
	}, nil
}

func (h *ResourceHandler) SetRegion(region string) {
	h.compute.SetRegion(region)
	h.database.SetRegion(region)
	h.analytics.SetRegion(region)
}

func Supported(kind string) bool {
	return kind == "Instance" || kind == "DbSystem" || kind == "AnalyticsInstance"
}

func (h *ResourceHandler) Plan(ctx context.Context, t task.Task) ([]Operation, error) {
	if t.Action != action.ON && t.Action != action.OFF {
		return nil, fmt.Errorf("invalid power action %v", t.Action)
	}
	if t.Resource.Identifier == nil ||
		*t.Resource.Identifier == "" ||
		t.Resource.ResourceType == nil {
		return nil, fmt.Errorf("resource ID and type are required")
	}
	op := Operation{
		ID:     *t.Resource.Identifier,
		Type:   *t.Resource.ResourceType,
		Action: t.Action.String(),
	}
	if !Supported(op.Type) {
		return nil, fmt.Errorf("unsupported resource type %q", op.Type)
	}
	if op.Type != "DbSystem" {
		op, err := h.inspect(ctx, op)
		if err != nil {
			return nil, err
		}
		return []Operation{op}, nil
	}
	if t.Resource.CompartmentId == nil || *t.Resource.CompartmentId == "" {
		return nil, fmt.Errorf("DB system compartment is required")
	}
	system, err := ociapi.Call(
		ctx,
		func(ctx context.Context) (database.GetDbSystemResponse, error) {
			return h.database.GetDbSystem(ctx, database.GetDbSystemRequest{DbSystemId: &op.ID})
		},
	)
	if err != nil {
		return nil, fmt.Errorf("get DB system: %w", err)
	}
	if system.Shape == nil || !strings.HasPrefix(*system.Shape, "VM.") {
		return nil, fmt.Errorf(
			"power scheduling supports VM DB systems only; shape %q is unsupported",
			value(system.Shape),
		)
	}
	req := database.ListDbNodesRequest{
		CompartmentId: t.Resource.CompartmentId,
		DbSystemId:    t.Resource.Identifier,
		Limit:         common.Int(1000),
	}
	ops := make([]Operation, 0)
	seenPages := make(map[string]bool)
	seenNodes := make(map[string]bool)
	for {
		resp, err := ociapi.Call(
			ctx,
			func(ctx context.Context) (database.ListDbNodesResponse, error) {
				return h.database.ListDbNodes(ctx, req)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("list DB nodes: %w", err)
		}
		for _, node := range resp.Items {
			if node.Id == nil || *node.Id == "" {
				return nil, fmt.Errorf("DB node missing ID")
			}
			if seenNodes[*node.Id] {
				continue
			}
			seenNodes[*node.Id] = true
			n := Operation{
				ID:       *node.Id,
				ParentID: op.ID,
				Type:     "DbNode",
				Action:   op.Action,
				State:    string(node.LifecycleState),
			}
			n, err = classify(n)
			if err != nil {
				return nil, err
			}
			ops = append(ops, n)
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		if seenPages[*resp.OpcNextPage] {
			return nil, fmt.Errorf("repeated DB node pagination token")
		}
		seenPages[*resp.OpcNextPage] = true
		req.Page = resp.OpcNextPage
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("DB system %s has no discoverable nodes", op.ID)
	}
	return ops, nil
}

func (h *ResourceHandler) inspect(ctx context.Context, op Operation) (Operation, error) {
	switch op.Type {
	case "Instance":
		r, err := ociapi.Call(
			ctx,
			func(ctx context.Context) (core.GetInstanceResponse, error) {
				return h.compute.GetInstance(ctx, core.GetInstanceRequest{InstanceId: &op.ID})
			},
		)
		if err != nil {
			return op, err
		}
		op.State = string(r.LifecycleState)
	case "DbNode":
		r, err := ociapi.Call(
			ctx,
			func(ctx context.Context) (database.GetDbNodeResponse, error) {
				return h.database.GetDbNode(ctx, database.GetDbNodeRequest{DbNodeId: &op.ID})
			},
		)
		if err != nil {
			return op, err
		}
		op.State = string(r.LifecycleState)
	case "AnalyticsInstance":
		r, err := ociapi.Call(
			ctx,
			func(ctx context.Context) (analytics.GetAnalyticsInstanceResponse, error) {
				return h.analytics.GetAnalyticsInstance(
					ctx,
					analytics.GetAnalyticsInstanceRequest{
						AnalyticsInstanceId: &op.ID,
					},
				)
			},
		)
		if err != nil {
			return op, err
		}
		op.State = string(r.LifecycleState)
	default:
		return op, fmt.Errorf("unsupported resource type %q", op.Type)
	}
	return classify(op)
}

func classify(op Operation) (Operation, error) {
	if op.ID == "" || (op.Action != "on" && op.Action != "off") {
		return op, fmt.Errorf("invalid power operation")
	}
	var on, off string
	switch op.Type {
	case "Instance":
		on, off = "RUNNING", "STOPPED"
	case "DbNode":
		on, off = "AVAILABLE", "STOPPED"
	case "AnalyticsInstance":
		on, off = "ACTIVE", "INACTIVE"
	default:
		return op, fmt.Errorf("unsupported resource type %q", op.Type)
	}
	op.Message = ""
	if (op.Action == "on" && op.State == on) || (op.Action == "off" && op.State == off) {
		op.Status = "noop"
		op.Message = "already in desired state"
	} else if op.State == on || op.State == off {
		op.Status = "planned"
	} else {
		return op, fmt.Errorf(
			"resource %s is in non-actionable state %q; "+
				"retry after the transition or resolve the resource state",
			op.ID,
			op.State,
		)
	}
	return op, nil
}

func (h *ResourceHandler) Execute(ctx context.Context, op Operation) (Operation, error) {
	// Re-read state before every write; discovery and dry-run states can be stale.
	current, err := h.inspect(ctx, op)
	if err != nil {
		op.Status = "failed"
		op.Message = err.Error()
		return op, err
	}
	op = current
	if op.Status == "noop" {
		return op, nil
	}
	switch op.Type {
	case "Instance":
		a := h.shutdown
		if op.Action == "on" {
			a = core.InstanceActionActionStart
		}
		r, e := ociapi.Call(
			ctx,
			func(ctx context.Context) (core.InstanceActionResponse, error) {
				return h.compute.InstanceAction(ctx, core.InstanceActionRequest{
					InstanceId: &op.ID,
					Action:     a,
				})
			},
		)
		err = e
		op.RequestID = value(r.OpcRequestId)
	case "DbNode":
		a := database.DbNodeActionActionStop
		if op.Action == "on" {
			a = database.DbNodeActionActionStart
		}
		r, e := ociapi.Call(
			ctx,
			func(ctx context.Context) (database.DbNodeActionResponse, error) {
				return h.database.DbNodeAction(ctx, database.DbNodeActionRequest{
					DbNodeId: &op.ID,
					Action:   a,
				})
			},
		)
		err = e
		op.RequestID = value(r.OpcRequestId)
	case "AnalyticsInstance":
		if op.Action == "on" {
			r, e := ociapi.Call(
				ctx,
				func(ctx context.Context) (analytics.StartAnalyticsInstanceResponse, error) {
					return h.analytics.StartAnalyticsInstance(
						ctx,
						analytics.StartAnalyticsInstanceRequest{
							AnalyticsInstanceId: &op.ID,
						},
					)
				},
			)
			err = e
			op.RequestID = value(r.OpcRequestId)
		} else {
			r, e := ociapi.Call(
				ctx,
				func(ctx context.Context) (analytics.StopAnalyticsInstanceResponse, error) {
					return h.analytics.StopAnalyticsInstance(
						ctx,
						analytics.StopAnalyticsInstanceRequest{
							AnalyticsInstanceId: &op.ID,
						},
					)
				},
			)
			err = e
			op.RequestID = value(r.OpcRequestId)
		}
	}
	if err != nil {
		op.Status = "failed"
		op.Message = err.Error()
		return op, err
	}
	op.Status = "submitted"
	return op, nil
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
