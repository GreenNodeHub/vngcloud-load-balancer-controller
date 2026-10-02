package vngcloud_mocks

import (
	"context"
	"testing"

	loadbalancerv2 "github.com/GreenNodeHub/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real vLB API reports drop for a listener created without a default action (measured
// 2026-10-01); the mock must not report "" there.
func TestCreateListenerDefaultsToDrop(t *testing.T) {
	ctx, m := context.Background(), NewMockProvider()
	create := func(req loadbalancerv2.ICreateListenerRequest) string {
		created, err := m.CreateListener(ctx, "lb-mock", req)
		require.NoError(t, err)
		l, err := m.GetListenerById(ctx, "lb-mock", created.UUID)
		require.NoError(t, err)
		return l.DefaultAction
	}

	assert.Equal(t, "drop", create(loadbalancerv2.NewCreateListenerRequest("l80", loadbalancerv2.ListenerProtocolTCP, 80)))
	assert.Equal(t, "accept", create(loadbalancerv2.NewCreateListenerRequest("l81", loadbalancerv2.ListenerProtocolTCP, 81).
		WithDefaultAction(loadbalancerv2.ListenerDefaultAction("accept"))), "an explicit default action is kept")
}
