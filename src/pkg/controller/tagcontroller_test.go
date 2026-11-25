package controller

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flynnkc/oci-frugal/src/pkg/action"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/handler"
	"github.com/flynnkc/oci-frugal/src/pkg/controller/task"
	"github.com/flynnkc/oci-frugal/src/pkg/scheduler"
	"github.com/oracle/oci-go-sdk/v65/common"
	rs "github.com/oracle/oci-go-sdk/v65/resourcesearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type searchMock struct {
	// Unstubbed interface methods deliberately fail if a test calls them unexpectedly.
	SearchAPI
	items    []rs.ResourceSummary
	calls    int
	failPage bool
	repeat   bool
}

func (m *searchMock) SearchResources(
	_ context.Context,
	r rs.SearchResourcesRequest,
) (rs.SearchResourcesResponse, error) {
	m.calls++
	if m.calls == 1 {
		return rs.SearchResourcesResponse{
			ResourceSummaryCollection: rs.ResourceSummaryCollection{
				Items: m.items,
			},
			OpcNextPage: common.String("next"),
		}, nil
	}
	if m.failPage {
		return rs.SearchResourcesResponse{}, errors.New("search failed")
	}
	if m.repeat {
		return rs.SearchResourcesResponse{OpcNextPage: common.String("next")}, nil
	}
	// A duplicate crossing page boundaries must not execute twice.
	return rs.SearchResourcesResponse{
		ResourceSummaryCollection: rs.ResourceSummaryCollection{
			Items: m.items,
		},
	}, nil
}

type handlerMock struct {
	plans, writes      atomic.Int32
	failPlan, failNode string
}

func (m *handlerMock) SetRegion(string) {}
func (m *handlerMock) Plan(_ context.Context, t task.Task) ([]handler.Operation, error) {
	m.plans.Add(1)
	if *t.Resource.Identifier == m.failPlan {
		return nil, errors.New("planning failed")
	}
	return []handler.Operation{
		{
			ID:     *t.Resource.Identifier,
			Type:   *t.Resource.ResourceType,
			Action: t.Action.String(),
			Status: "planned",
		},
	}, nil
}
func (m *handlerMock) Execute(
	_ context.Context,
	op handler.Operation,
) (handler.Operation, error) {
	m.writes.Add(1)
	if op.ID == m.failNode {
		op.Status = "failed"
		op.Message = "write failed"
		return op, errors.New("write failed")
	}
	op.Status = "submitted"
	return op, nil
}
func item(id, kind, token string) rs.ResourceSummary {
	return rs.ResourceSummary{
		Identifier:   &id,
		ResourceType: &kind,
		DefinedTags: map[string]map[string]interface{}{
			"Schedule": {
				"AnyDay": strings.TrimSuffix(strings.Repeat(token+",", 24), ","),
			},
		},
	}
}
func testController(items ...rs.ResourceSummary) (*TagController, *handlerMock, *searchMock) {
	h := &handlerMock{}
	s := &searchMock{items: items}
	return &TagController{
		tagNamespace: "Schedule",
		region:       "test",
		scheduler: scheduler.NewAnykeyNLSchedulerAt(
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			time.UTC,
		),
		action:  action.ALL,
		handler: h,
		search:  s,
	}, h, s
}
func TestPlanIsReadOnlyAndDeduplicates(t *testing.T) {
	c, h, s := testController(item("vm", "Instance", "1"))
	plan, err := c.Plan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, s.calls)
	require.Len(t, plan.Resources, 1)
	assert.Equal(t, int32(1), h.plans.Load())
	assert.Zero(t, h.writes.Load())
	assert.True(t, plan.DryRun)
	result, err := c.Execute(context.Background(), plan)
	require.NoError(t, err)
	assert.False(t, result.DryRun)
	require.Len(t, result.Resources, 1)
	assert.Equal(t, "submitted", result.Resources[0].Status)
	assert.Equal(t, int32(1), h.writes.Load())
	// Results contain nested slices; a shallow copy would overwrite the original plan.
	assert.Equal(t, "planned", plan.Resources[0].Status, "execution mutated plan")
	require.Len(t, plan.Resources[0].Operations, 1)
	assert.Equal(t, "planned", plan.Resources[0].Operations[0].Status, "execution mutated plan")
}
func TestPlanningErrorsBlockWrites(t *testing.T) {
	for _, bad := range []rs.ResourceSummary{
		item("adb", "AutonomousDatabase", "1"),
		item("oic", "IntegrationInstance", "0"),
		item("bad", "Instance", "3"),
		item("bad", "Instance", "-1"),
		item("bad", "Instance", "broken"),
	} {
		c, h, _ := testController(item("good", "Instance", "1"), bad)
		plan, err := c.Plan(context.Background())
		require.Error(t, err, "resource: %+v", bad)
		assert.NotEmpty(t, plan.Error)
		_, err = c.Execute(context.Background(), plan)
		assert.Error(t, err)
		assert.Zero(t, h.writes.Load(), "failed plan was executed")
	}
	c, h, s := testController(item("vm", "Instance", "1"))
	s.failPage = true
	// A later page failing must discard the partial inventory before planning starts.
	plan, err := c.Plan(context.Background())
	assert.Error(t, err)
	assert.Empty(t, plan.Resources)
	assert.Zero(t, h.plans.Load(), "partial discovery processed")
	s.failPage = false
	s.repeat = true
	s.calls = 0
	_, err = c.Plan(context.Background())
	assert.ErrorContains(t, err, "repeated resource search pagination token")
}
func TestSkippedResourcesAndActionFilter(t *testing.T) {
	untagged := item("untagged", "Instance", "0")
	untagged.DefinedTags = nil
	c, h, _ := testController(
		untagged,
		item("wildcard", "Instance", "*"),
		item("off", "Instance", "0"),
		item("on", "Instance", "1"),
	)
	c.action = action.ON
	plan, err := c.Plan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), h.plans.Load())
	assert.Len(t, plan.Resources, 4)
}
func TestNodeFailureDoesNotHideOtherResults(t *testing.T) {
	c, h, _ := testController(item("vm", "Instance", "1"))
	h.failNode = "one"
	// Model a two-node DB system: the first failure must not hide the second outcome.
	plan := Result{
		Region: "test",
		Resources: []ResourceResult{
			{
				ID:     "db",
				Type:   "DbSystem",
				Status: "planned",
				Operations: []handler.Operation{
					{
						ID:     "one",
						Type:   "DbNode",
						Action: "on",
						Status: "planned",
					},
					{
						ID:     "two",
						Type:   "DbNode",
						Action: "on",
						Status: "planned",
					},
				},
			},
		},
	}
	result, err := c.Execute(context.Background(), plan)
	assert.Error(t, err)
	assert.NotEmpty(t, result.Error)
	assert.Equal(t, int32(2), h.writes.Load())
	require.Len(t, result.Resources, 1)
	require.Len(t, result.Resources[0].Operations, 2)
	assert.Equal(
		t,
		"submitted",
		result.Resources[0].Operations[1].Status,
		"lost partial execution results",
	)
}
