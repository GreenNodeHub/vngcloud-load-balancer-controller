package glbc_uc

import (
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

func newHeadersTestTask(t *testing.T) (*defaultModelDeployTask, *repository.MockVngCloudRepository) {
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
	cfg := &config.Config{
		GlobalLoadBalancerOpts: config.GlobalLoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50,
			DefaultTimeoutMember:     50,
			DefaultTimeoutConnection: 5,
		},
	}
	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: mockVngcloudRepo,
		cfg:          cfg,
		lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
			Status: v1alpha1.GlobalLoadBalancerConfigStatus{},
		},
	}
	return task, mockVngcloudRepo
}

// TestBuildListenerUpdateRequest_Headers verifies that differing headers between the current
// listener entity and the spec trigger a listener update (STAT-02).
func TestBuildListenerUpdateRequest_Headers(t *testing.T) {
	task, mockVngcloudRepo := newHeadersTestTask(t)

	currentListener := &entityv2.GlobalListener{
		ID:                "lis-1",
		Port:              80,
		Protocol:          "TCP",
		Headers:           ptr.To("x-old"),
		AllowedCidrs:      "0.0.0.0/0",
		TimeoutClient:     50,
		TimeoutMember:     50,
		TimeoutConnection: 5,
		GlobalPoolID:      "",
	}

	listenerSpec := v1alpha1.GlobalListener{
		Name:         "test",
		ProtocolPort: 80,
		Protocol:     "TCP",
		Headers:      []string{"x-new"},
	}

	updateOptions, message, err := task.buildListenerUpdateRequest(
		context.Background(),
		"glb-123",
		listenerSpec,
		currentListener,
		[]v1alpha1.CreatedGlobalPool{},
	)

	assert.NoError(t, err)
	assert.NotNil(t, updateOptions, "update request must be non-nil when headers differ")

	headersInMessage := false
	for _, m := range message {
		if strings.Contains(strings.ToLower(m), "headers") {
			headersInMessage = true
			break
		}
	}
	assert.True(t, headersInMessage, "message slice must contain a headers-related entry")

	mockVngcloudRepo.AssertExpectations(t)
}

// TestBuildListenerUpdateRequest_HeadersNoChange verifies that equivalent headers (same values)
// do not trigger a spurious listener update (STAT-02).
func TestBuildListenerUpdateRequest_HeadersNoChange(t *testing.T) {
	task, mockVngcloudRepo := newHeadersTestTask(t)

	currentListener := &entityv2.GlobalListener{
		ID:                "lis-1",
		Port:              80,
		Protocol:          "TCP",
		Headers:           ptr.To("x-keep"),
		AllowedCidrs:      "0.0.0.0/0",
		TimeoutClient:     50,
		TimeoutMember:     50,
		TimeoutConnection: 5,
		GlobalPoolID:      "",
	}

	listenerSpec := v1alpha1.GlobalListener{
		Name:         "test",
		ProtocolPort: 80,
		Protocol:     "TCP",
		Headers:      []string{"x-keep"},
	}

	updateOptions, message, err := task.buildListenerUpdateRequest(
		context.Background(),
		"glb-123",
		listenerSpec,
		currentListener,
		[]v1alpha1.CreatedGlobalPool{},
	)

	assert.NoError(t, err)

	// Either the update is nil (nothing changed) or headers are not in the message
	if updateOptions != nil {
		for _, m := range message {
			assert.False(t, strings.Contains(strings.ToLower(m), "headers"),
				"headers must not appear in change message when headers are equivalent")
		}
	}

	mockVngcloudRepo.AssertExpectations(t)
}

// TestBuildListenerUpdateRequest_HeadersNilEntityEmptySpec verifies that nil entity headers and
// empty spec headers do not trigger a spurious listener update (STAT-02).
func TestBuildListenerUpdateRequest_HeadersNilEntityEmptySpec(t *testing.T) {
	task, mockVngcloudRepo := newHeadersTestTask(t)

	currentListener := &entityv2.GlobalListener{
		ID:                "lis-1",
		Port:              80,
		Protocol:          "TCP",
		Headers:           nil, // no headers on entity
		AllowedCidrs:      "0.0.0.0/0",
		TimeoutClient:     50,
		TimeoutMember:     50,
		TimeoutConnection: 5,
		GlobalPoolID:      "",
	}

	listenerSpec := v1alpha1.GlobalListener{
		Name:         "test",
		ProtocolPort: 80,
		Protocol:     "TCP",
		Headers:      []string{}, // empty spec headers
	}

	updateOptions, message, err := task.buildListenerUpdateRequest(
		context.Background(),
		"glb-123",
		listenerSpec,
		currentListener,
		[]v1alpha1.CreatedGlobalPool{},
	)

	assert.NoError(t, err)

	// Either no update, or headers not in message
	if updateOptions != nil {
		for _, m := range message {
			assert.False(t, strings.Contains(strings.ToLower(m), "headers"),
				"nil entity headers and empty spec headers must not trigger a headers update")
		}
	}

	mockVngcloudRepo.AssertExpectations(t)
}
