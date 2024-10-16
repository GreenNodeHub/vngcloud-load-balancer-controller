package builder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

func TestNewOldModelBuilder(t *testing.T) {
	const prefix = "mock-prefix"
	// Prepare test data
	annos := map[string]string{
		fmt.Sprintf("%s/%s", prefix, annotations.SuffixIgnore):          "true",
		fmt.Sprintf("%s/%s", prefix, annotations.SuffixLoadBalancerID):  "lb-12345",
		fmt.Sprintf("%s/%s", prefix, annotations.SuffixManagePools):     "pool-1:pool-one,pool-2:pool-two",
		fmt.Sprintf("%s/%s", prefix, annotations.SuffixManageListeners): "listener-1:listener-one:[policy-one],listener-2:listener-two:[policy-two|policy-three],l3:l3:[]",
	}

	oldAnnotations := map[string]string{
		fmt.Sprintf("%s/%s", prefix, annotations.SuffixTags): "aaa=bbb,c=d",
	}

	annosParser := annotations.NewSuffixAnnotationParser(prefix)

	// Call the function
	model := NewOldModelBuilder(annos, oldAnnotations, annosParser)

	// Assertions
	assert.NotNil(t, model)
	assert.True(t, model.IsIgnored())                      // isIgnored should be true
	assert.Equal(t, "lb-12345", model.GetLoadBalancerID()) // lbID should be set

	// // Check if pools were built correctly
	// pools := model.GetOldPools()
	// assert.Len(t, pools, 2)
	// assert.Equal(t, "pool-1", pools[0].ID())
	// assert.Equal(t, "pool-one", pools[0].Name())
	// assert.Equal(t, "pool-2", pools[1].ID())
	// assert.Equal(t, "pool-two", pools[1].Name())

	// Check if listeners were built correctly
	listeners := model.GetOldListeners()
	assert.Len(t, listeners, 3)
	assert.Equal(t, "listener-1", listeners[0].GetID())
	assert.Equal(t, "listener-one", listeners[0].GetName())
	assert.Len(t, listeners[0].GetOldPolicies(), 1)
	assert.Equal(t, "policy-one", listeners[0].GetOldPolicies()[0].GetName())

	assert.Equal(t, "listener-2", listeners[1].GetID())
	assert.Equal(t, "listener-two", listeners[1].GetName())
	assert.Len(t, listeners[1].GetOldPolicies(), 2)
	assert.Equal(t, "policy-two", listeners[1].GetOldPolicies()[0].GetName())
	assert.Equal(t, "policy-three", listeners[1].GetOldPolicies()[1].GetName())

	assert.Equal(t, "l3", listeners[2].GetID())
	assert.Equal(t, "l3", listeners[2].GetName())
	assert.Len(t, listeners[2].GetOldPolicies(), 0)

	// Check if tags were built correctly
	tags := model.GetOldTags()
	assert.Len(t, tags, 2)
	assert.Equal(t, "bbb", tags["aaa"])
	assert.Equal(t, "d", tags["c"])
}
