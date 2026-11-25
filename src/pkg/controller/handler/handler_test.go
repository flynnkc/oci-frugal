package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/oracle/oci-go-sdk/v65/analytics"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type computeMock struct {
	ComputeAPI
	state         core.InstanceLifecycleStateEnum
	reads, writes int
	sent          core.InstanceActionActionEnum
	err           error
}

func (m *computeMock) GetInstance(
	context.Context,
	core.GetInstanceRequest,
) (core.GetInstanceResponse, error) {
	m.reads++
	return core.GetInstanceResponse{Instance: core.Instance{LifecycleState: m.state}}, nil
}
func (m *computeMock) InstanceAction(
	_ context.Context,
	r core.InstanceActionRequest,
) (core.InstanceActionResponse, error) {
	m.writes++
	m.sent = r.Action
	return core.InstanceActionResponse{OpcRequestId: common.String("request")}, m.err
}

type analyticsMock struct {
	AnalyticsAPI
	state         analytics.AnalyticsInstanceLifecycleStateEnum
	starts, stops int
}

func (m *analyticsMock) GetAnalyticsInstance(
	context.Context,
	analytics.GetAnalyticsInstanceRequest,
) (analytics.GetAnalyticsInstanceResponse, error) {
	return analytics.GetAnalyticsInstanceResponse{
		AnalyticsInstance: analytics.AnalyticsInstance{
			LifecycleState: m.state,
		},
	}, nil
}
func (m *analyticsMock) StartAnalyticsInstance(
	context.Context,
	analytics.StartAnalyticsInstanceRequest,
) (analytics.StartAnalyticsInstanceResponse, error) {
	m.starts++
	return analytics.StartAnalyticsInstanceResponse{}, nil
}
func (m *analyticsMock) StopAnalyticsInstance(
	context.Context,
	analytics.StopAnalyticsInstanceRequest,
) (analytics.StopAnalyticsInstanceResponse, error) {
	m.stops++
	return analytics.StopAnalyticsInstanceResponse{}, nil
}

type dbMock struct {
	DatabaseAPI
	pages, actions int
	failPage       int
	failNode       string
	state          database.DbNodeLifecycleStateEnum
	shape          string
}

func (m *dbMock) GetDbSystem(
	context.Context,
	database.GetDbSystemRequest,
) (database.GetDbSystemResponse, error) {
	shape := m.shape
	if shape == "" {
		shape = "VM.Standard.E5.Flex"
	}
	return database.GetDbSystemResponse{DbSystem: database.DbSystem{Shape: &shape}}, nil
}
func (m *dbMock) ListDbNodes(
	_ context.Context,
	r database.ListDbNodesRequest,
) (database.ListDbNodesResponse, error) {
	m.pages++
	if m.failPage == m.pages {
		return database.ListDbNodesResponse{}, errors.New("node search denied")
	}
	if r.DbSystemId == nil || *r.DbSystemId != "db" || r.CompartmentId == nil {
		panic("missing DB scope")
	}
	if r.Page == nil {
		return database.ListDbNodesResponse{
			Items: []database.DbNodeSummary{
				{
					Id:             common.String("one"),
					LifecycleState: database.DbNodeSummaryLifecycleStateStopped,
				},
			},
			OpcNextPage: common.String("next"),
		}, nil
	}
	return database.ListDbNodesResponse{
		Items: []database.DbNodeSummary{
			{
				Id:             common.String("two"),
				LifecycleState: database.DbNodeSummaryLifecycleStateStopped,
			},
		},
	}, nil
}
func (m *dbMock) GetDbNode(
	context.Context,
	database.GetDbNodeRequest,
) (database.GetDbNodeResponse, error) {
	return database.GetDbNodeResponse{DbNode: database.DbNode{LifecycleState: m.state}}, nil
}
func (m *dbMock) DbNodeAction(
	_ context.Context,
	r database.DbNodeActionRequest,
) (database.DbNodeActionResponse, error) {
	m.actions++
	if *r.DbNodeId == m.failNode {
		return database.DbNodeActionResponse{}, errors.New("node action failed")
	}
	return database.DbNodeActionResponse{}, nil
}

func resource(kind, id string) rs.ResourceSummary {
	return rs.ResourceSummary{
		Identifier:    &id,
		ResourceType:  &kind,
		CompartmentId: common.String("compartment"),
	}
}

func TestComputePlanAndExecute(t *testing.T) {
	for _, policy := range []core.InstanceActionActionEnum{
		core.InstanceActionActionSoftstop,
		core.InstanceActionActionStop,
	} {
		m := &computeMock{state: core.InstanceLifecycleStateRunning}
		h := &ResourceHandler{compute: m, shutdown: policy}
		ops, err := h.Plan(
			context.Background(),
			task.NewTask(action.OFF, resource("Instance", "vm")),
		)
		require.NoError(t, err)
		require.Len(t, ops, 1)
		assert.Equal(t, "planned", ops[0].Status)
		assert.Zero(t, m.writes)
		op, err := h.Execute(context.Background(), ops[0])
		require.NoError(t, err)
		assert.Equal(t, "submitted", op.Status)
		assert.Equal(t, "request", op.RequestID)
		assert.Equal(t, policy, m.sent)
		// Planning reads, execution rechecks state, and submission each consume a token.
	}
}
func TestExecuteRechecksStateAndPropagatesErrors(t *testing.T) {
	m := &computeMock{state: core.InstanceLifecycleStateStopped}
	h := &ResourceHandler{
		compute:  m,
		shutdown: core.InstanceActionActionSoftstop,
	}
	// The instance stopped after planning; execution must trust the fresh OCI state.
	op := Operation{
		ID:     "vm",
		Type:   "Instance",
		Action: "off",
		Status: "planned",
		State:  "RUNNING",
	}
	updated, err := h.Execute(context.Background(), op)
	require.NoError(t, err)
	assert.Equal(t, "noop", updated.Status)
	assert.Zero(t, m.writes, "stale state should not cause a write")
	m.state = core.InstanceLifecycleStateStarting
	_, err = h.Execute(context.Background(), op)
	assert.Error(t, err)
	assert.Zero(t, m.writes, "acted on transitional state")
	m.state = core.InstanceLifecycleStateRunning
	m.err = errors.New("OCI 429")
	updated, err = h.Execute(context.Background(), op)
	assert.ErrorIs(t, err, m.err)
	assert.Equal(t, "failed", updated.Status)
	assert.NotEmpty(t, updated.Message)
}
func TestAnalyticsStateChecks(t *testing.T) {
	for _, tt := range []struct {
		state         analytics.AnalyticsInstanceLifecycleStateEnum
		action        string
		status        string
		starts, stops int
	}{
		{analytics.AnalyticsInstanceLifecycleStateActive, "on", "noop", 0, 0},
		{analytics.AnalyticsInstanceLifecycleStateInactive, "off", "noop", 0, 0},
		{analytics.AnalyticsInstanceLifecycleStateActive, "off", "submitted", 0, 1},
		{analytics.AnalyticsInstanceLifecycleStateInactive, "on", "submitted", 1, 0},
	} {
		m := &analyticsMock{state: tt.state}
		h := &ResourceHandler{analytics: m}
		op, err := h.Execute(context.Background(), Operation{
			ID:     "oac",
			Type:   "AnalyticsInstance",
			Action: tt.action,
		})
		require.NoError(t, err, "case: %+v", tt)
		assert.Equal(t, tt.status, op.Status)
		assert.Equal(t, tt.starts, m.starts)
		assert.Equal(t, tt.stops, m.stops)
	}
}
func TestDbNodePaginationAndFailures(t *testing.T) {
	for _, fail := range []int{0, 2} {
		db := &dbMock{failPage: fail, state: database.DbNodeLifecycleStateStopped}
		h := &ResourceHandler{database: db}
		ops, err := h.Plan(
			context.Background(),
			task.NewTask(action.ON, resource("DbSystem", "db")),
		)
		if fail != 0 {
			assert.Error(t, err)
			assert.Empty(t, ops, "partial discovery produced executable plan")
			continue
		}
		require.NoError(t, err)
		require.Len(t, ops, 2)
		assert.Equal(t, 2, db.pages)
		for _, op := range ops {
			_, err := h.Execute(context.Background(), op)
			require.NoError(t, err)
		}
		assert.Equal(t, 2, db.actions, "missed RAC node")
		// DB nodes use AVAILABLE for the running state, unlike Compute's RUNNING.
		db.state = database.DbNodeLifecycleStateAvailable
		op, err := h.Execute(context.Background(), ops[0])
		require.NoError(t, err)
		assert.Equal(t, "noop", op.Status, "AVAILABLE not recognized")
		assert.Equal(t, 2, db.actions)
	}
	db := &dbMock{shape: "BM.DenseIO2.52"}
	h := &ResourceHandler{database: db}
	_, err := h.Plan(context.Background(), task.NewTask(action.ON, resource("DbSystem", "db")))
	assert.ErrorContains(t, err, "VM DB systems only")
	assert.Zero(t, db.pages)
}
func TestInvalidInputAndCancellationNeverWrite(t *testing.T) {
	m := &computeMock{state: core.InstanceLifecycleStateStopped}
	h := &ResourceHandler{compute: m}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.Execute(ctx, Operation{ID: "vm", Type: "Instance", Action: "on"})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, m.reads)
	for _, r := range []rs.ResourceSummary{
		{},
		resource("AutonomousDatabase", "adb"),
		resource("IntegrationInstance", "oic"),
	} {
		_, err := h.Plan(context.Background(), task.NewTask(action.ON, r))
		assert.Error(t, err, "accepted unsupported input: %+v", r)
	}
}
