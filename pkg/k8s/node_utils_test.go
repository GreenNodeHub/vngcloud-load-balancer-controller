package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestGetNodeCondition tests the GetNodeCondition function with multiple scenarios.
func TestGetNodeCondition(t *testing.T) {
	tests := []struct {
		name           string
		node           *corev1.Node
		conditionType  corev1.NodeConditionType
		expectedResult *corev1.NodeCondition
	}{
		{
			name: "condition exists",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			conditionType:  corev1.NodeReady,
			expectedResult: &corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		},
		{
			name: "condition does not exist",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			conditionType:  corev1.NodeDiskPressure,
			expectedResult: nil,
		},
		{
			name:           "node has no conditions",
			node:           &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{}}},
			conditionType:  corev1.NodeReady,
			expectedResult: nil,
		},
		{
			name:           "node is nil",
			node:           nil,
			conditionType:  corev1.NodeReady,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNodeCondition(tt.node, tt.conditionType)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestExtractNodeInstanceID tests the ExtractNodeInstanceID function with multiple scenarios.
func TestExtractNodeInstanceID(t *testing.T) {
	tests := []struct {
		name          string
		providerID    string
		expectedID    string
		expectedError bool
	}{
		{
			name:          "valid providerID",
			providerID:    "vngcloud://ins-12345678-1234-1234-1234-123456789012",
			expectedID:    "ins-12345678-1234-1234-1234-123456789012",
			expectedError: false,
		},
		{
			name:          "invalid providerID format",
			providerID:    "invalid-provider-id",
			expectedID:    "",
			expectedError: true,
		},
		{
			name:          "empty providerID",
			providerID:    "",
			expectedID:    "",
			expectedError: true,
		},
		{
			name:          "providerID with no instance ID",
			providerID:    "vngcloud://",
			expectedID:    "",
			expectedError: true,
		},
		{
			name:          "providerID with invalid instance ID format",
			providerID:    "vngcloud://ins-invalid",
			expectedID:    "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{Spec: corev1.NodeSpec{ProviderID: tt.providerID}}
			instanceID, err := ExtractNodeInstanceID(node)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedID, instanceID)
		})
	}
}

// TestFilterNodeWithLabel tests the FilterNodeWithLabel function with multiple scenarios.
func TestFilterNodeWithLabel(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"role": "worker",
					"env":  "prod",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"role": "master",
					"env":  "prod",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					"role": "worker",
					"env":  "dev",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-4",
				Labels: nil,
			},
		},
	}

	tests := []struct {
		name          string
		labels        map[string]string
		expectedNodes []string
	}{
		{
			name:          "filter by role=worker",
			labels:        map[string]string{"role": "worker"},
			expectedNodes: []string{"node-1", "node-3"},
		},
		{
			name:          "filter by env=prod",
			labels:        map[string]string{"env": "prod"},
			expectedNodes: []string{"node-1", "node-2"},
		},
		{
			name:          "filter by non-existent label",
			labels:        map[string]string{"nonexistent": "label"},
			expectedNodes: []string{},
		},
		{
			name:          "filter by multiple labels",
			labels:        map[string]string{"role": "worker", "env": "dev"},
			expectedNodes: []string{"node-3"},
		},
		{
			name:          "filter with empty labels",
			labels:        map[string]string{},
			expectedNodes: []string{"node-1", "node-2", "node-3", "node-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredNodes := FilterNodeWithLabel(nodes, tt.labels)
			filteredNodeNames := []string{}
			for _, node := range filteredNodes {
				filteredNodeNames = append(filteredNodeNames, node.Name)
			}
			assert.Equal(t, tt.expectedNodes, filteredNodeNames)
		})
	}
}

// TestFilterNodeWithoutLabel tests the FilterNodeWithoutLabel function with multiple scenarios.
func TestFilterNodeWithoutLabel(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"role": "worker",
					"env":  "prod",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"role": "master",
					"env":  "prod",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					"role": "worker",
					"env":  "dev",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-4",
				Labels: nil,
			},
		},
	}

	tests := []struct {
		name          string
		labels        map[string]string
		expectedNodes []string
	}{
		{
			name:          "filter out role=worker",
			labels:        map[string]string{"role": "worker"},
			expectedNodes: []string{"node-2", "node-4"},
		},
		{
			name:          "filter out env=prod",
			labels:        map[string]string{"env": "prod"},
			expectedNodes: []string{"node-3", "node-4"},
		},
		{
			name:          "filter out non-existent label",
			labels:        map[string]string{"nonexistent": "label"},
			expectedNodes: []string{"node-1", "node-2", "node-3", "node-4"},
		},
		{
			name:          "filter out with empty labels",
			labels:        map[string]string{},
			expectedNodes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredNodes := FilterNodeWithoutLabel(nodes, tt.labels)
			filteredNodeNames := []string{}
			for _, node := range filteredNodes {
				filteredNodeNames = append(filteredNodeNames, node.Name)
			}
			assert.Equal(t, tt.expectedNodes, filteredNodeNames)
		})
	}
}
