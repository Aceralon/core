package calcium

import (
	"context"
	"fmt"
	"strings"
	"testing"

	enginemocks "github.com/projecteru2/core/engine/mocks"
	enginetypes "github.com/projecteru2/core/engine/types"
	lockmocks "github.com/projecteru2/core/lock/mocks"
	"github.com/projecteru2/core/log"
	"github.com/projecteru2/core/resources"
	resourcemocks "github.com/projecteru2/core/resources/mocks"
	storemocks "github.com/projecteru2/core/store/mocks"
	"github.com/projecteru2/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPodResource(t *testing.T) {
	c := NewTestCluster()
	ctx := context.Background()
	podname := "testpod"
	nodename := "testnode"
	store := &storemocks.Store{}
	c.store = store
	plugin := c.resource.GetPlugins()[0].(*resourcemocks.Plugin)
	plugin.On("GetNodeResourceInfo", mock.Anything, mock.Anything, mock.Anything).Return(&resources.GetNodeResourceInfoResponse{
		ResourceInfo: &resources.NodeResourceInfo{},
		Diffs:        []string{"hhh"},
	}, nil)

	lock := &lockmocks.DistributedLock{}
	lock.On("Lock", mock.Anything).Return(context.TODO(), nil)
	lock.On("Unlock", mock.Anything).Return(nil)
	// failed by GetNodesByPod
	store.On("GetNodesByPod", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, types.ErrNoETCD).Once()
	ch, err := c.PodResource(ctx, podname)
	assert.Error(t, err)
	store.AssertExpectations(t)

	// failed by ListNodeWorkloads
	node := &types.Node{
		NodeMeta: types.NodeMeta{
			Name: nodename,
		},
	}
	store.On("GetNodesByPod", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*types.Node{node}, nil)
	store.On("GetNode", mock.Anything, mock.Anything).Return(node, nil)
	store.On("ListNodeWorkloads", mock.Anything, mock.Anything, mock.Anything).Return(nil, types.ErrNoETCD).Once()
	store.On("CreateLock", mock.Anything, mock.Anything).Return(lock, nil)
	ch, err = c.PodResource(ctx, podname)
	assert.NoError(t, err)
	msg := <-ch
	assert.True(t, strings.Contains(msg.Diffs[0], types.ErrNoETCD.Error()))
	store.AssertExpectations(t)

	workloads := []*types.Workload{
		{
			ResourceArgs: map[string]types.WorkloadResourceArgs{},
		},
		{
			ResourceArgs: map[string]types.WorkloadResourceArgs{},
		},
	}
	store.On("ListNodeWorkloads", mock.Anything, mock.Anything, mock.Anything).Return(workloads, nil)
	engine := &enginemocks.API{}
	engine.On("ResourceValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		fmt.Errorf("%s", "not validate"),
	)
	node.Engine = engine
	// success
	r, err := c.PodResource(ctx, podname)
	assert.NoError(t, err)
	first := <-r
	assert.NotEmpty(t, first.Diffs)
}

func TestNodeResource(t *testing.T) {
	c := NewTestCluster()
	ctx := context.Background()
	nodename := "testnode"
	store := &storemocks.Store{}
	c.store = store
	plugin := c.resource.GetPlugins()[0].(*resourcemocks.Plugin)
	plugin.On("GetNodeResourceInfo", mock.Anything, mock.Anything, mock.Anything).Return(&resources.GetNodeResourceInfoResponse{
		ResourceInfo: &resources.NodeResourceInfo{},
		Diffs:        []string{"hhh"},
	}, nil)
	plugin.On("FixNodeResource", mock.Anything, mock.Anything, mock.Anything).Return(&resources.GetNodeResourceInfoResponse{
		ResourceInfo: &resources.NodeResourceInfo{},
		Diffs:        []string{"hhh"},
	}, nil)
	lock := &lockmocks.DistributedLock{}
	store.On("CreateLock", mock.Anything, mock.Anything).Return(lock, nil)
	lock.On("Lock", mock.Anything).Return(context.TODO(), nil)
	lock.On("Unlock", mock.Anything).Return(nil)
	node := &types.Node{
		NodeMeta: types.NodeMeta{
			Name: nodename,
		},
	}
	engine := &enginemocks.API{}
	engine.On("ResourceValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		fmt.Errorf("%s", "not validate"),
	)
	node.Engine = engine
	// fail by validating
	_, err := c.NodeResource(ctx, "", false)
	assert.Error(t, err)
	// failed by GetNode
	store.On("GetNode", ctx, nodename).Return(nil, types.ErrNoETCD).Once()
	_, err = c.NodeResource(ctx, nodename, false)
	assert.Error(t, err)
	store.On("GetNode", mock.Anything, nodename).Return(node, nil)
	// failed by list node workloads
	store.On("ListNodeWorkloads", mock.Anything, mock.Anything, mock.Anything).Return(nil, types.ErrNoETCD).Once()
	_, err = c.NodeResource(ctx, nodename, false)
	assert.Error(t, err)
	workloads := []*types.Workload{
		{
			ResourceArgs: map[string]types.WorkloadResourceArgs{},
		},
		{
			ResourceArgs: map[string]types.WorkloadResourceArgs{},
		},
	}
	store.On("ListNodeWorkloads", mock.Anything, mock.Anything, mock.Anything).Return(workloads, nil)
	store.On("UpdateNodes", mock.Anything, mock.Anything).Return(nil)
	// success but workload inspect failed
	nr, err := c.NodeResource(ctx, nodename, true)
	assert.NoError(t, err)
	assert.Equal(t, nr.Name, nodename)
	assert.NotEmpty(t, nr.Diffs)
	details := strings.Join(nr.Diffs, ",")
	assert.Contains(t, details, "inspect failed")
}

func TestRemapResource(t *testing.T) {
	c := NewTestCluster()
	store := &storemocks.Store{}
	c.store = store
	plugin := c.resource.GetPlugins()[0].(*resourcemocks.Plugin)
	plugin.On("GetNodeResourceInfo", mock.Anything, mock.Anything, mock.Anything).Return(&resources.GetNodeResourceInfoResponse{
		ResourceInfo: &resources.NodeResourceInfo{},
		Diffs:        []string{"hhh"},
	}, nil)
	plugin.On("GetRemapArgs", mock.Anything, mock.Anything, mock.Anything).Return(&resources.GetRemapArgsResponse{
		EngineArgsMap: map[string]types.EngineArgs{},
	}, nil)
	engine := &enginemocks.API{}
	node := &types.Node{Engine: engine}

	workload := &types.Workload{
		ResourceArgs: map[string]types.WorkloadResourceArgs{},
	}
	store.On("ListNodeWorkloads", mock.Anything, mock.Anything, mock.Anything).Return([]*types.Workload{workload}, nil)
	ch := make(chan enginetypes.VirtualizationRemapMessage, 1)
	ch <- enginetypes.VirtualizationRemapMessage{}
	close(ch)
	engine.On("VirtualizationResourceRemap", mock.Anything, mock.Anything).Return((<-chan enginetypes.VirtualizationRemapMessage)(ch), nil)
	_, err := c.remapResource(context.Background(), node)
	assert.Nil(t, err)

	c.doRemapResourceAndLog(context.TODO(), log.WithField("test", "zc"), node)
}
