package k8s

// TODO: ............................................................
// import (
// 	"github.com/pkg/errors"
// 	"github.com/stretchr/testify/assert"
// 	corev1 "k8s.io/api/core/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"testing"
// )

// func TestGetNodeCondition(t *testing.T) {
// 	type args struct {
// 		node          *corev1.Node
// 		conditionType corev1.NodeConditionType
// 	}
// 	tests := []struct {
// 		name string
// 		args args
// 		want *corev1.NodeCondition
// 	}{
// 		{
// 			name: "node condition found",
// 			args: args{
// 				node: &corev1.Node{
// 					Status: corev1.NodeStatus{
// 						Conditions: []corev1.NodeCondition{
// 							{
// 								Type:   corev1.NodeReady,
// 								Status: corev1.ConditionFalse,
// 							},
// 						},
// 					},
// 				},
// 				conditionType: corev1.NodeReady,
// 			},
// 			want: &corev1.NodeCondition{
// 				Type:   corev1.NodeReady,
// 				Status: corev1.ConditionFalse,
// 			},
// 		},
// 		{
// 			name: "node condition not found",
// 			args: args{
// 				node: &corev1.Node{
// 					Status: corev1.NodeStatus{
// 						Conditions: []corev1.NodeCondition{
// 							{
// 								Type:   corev1.NodeReady,
// 								Status: corev1.ConditionFalse,
// 							},
// 						},
// 					},
// 				},
// 				conditionType: corev1.NodeMemoryPressure,
// 			},
// 			want: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			got := GetNodeCondition(tt.args.node, tt.args.conditionType)
// 			assert.Equal(t, tt.want, got)
// 		})
// 	}
// }

// func TestExtractNodeInstanceID(t *testing.T) {
// 	type args struct {
// 		node *corev1.Node
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		want    string
// 		wantErr error
// 	}{
// 		{
// 			name: "node without providerID",
// 			args: args{
// 				node: &corev1.Node{
// 					ObjectMeta: metav1.ObjectMeta{
// 						Name: "my-node-name",
// 					},
// 					Spec: corev1.NodeSpec{
// 						ProviderID: "",
// 					},
// 				},
// 			},
// 			wantErr: errors.New("providerID is not specified for node: my-node-name"),
// 		},
// 		{
// 			name: "node instance",
// 			args: args{
// 				node: &corev1.Node{
// 					ObjectMeta: metav1.ObjectMeta{
// 						Name: "my-node-name",
// 					},
// 					Spec: corev1.NodeSpec{
// 						ProviderID: "vngcloud://ins-8ff3c1e3-6497-4220-bdeb-7ae70e54128a",
// 					},
// 				},
// 			},
// 			want: "ins-8ff3c1e3-6497-4220-bdeb-7ae70e54128a",
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			got, err := ExtractNodeInstanceID(tt.args.node)
// 			if tt.wantErr != nil {
// 				assert.EqualError(t, err, tt.wantErr.Error())
// 			} else {
// 				assert.NoError(t, err)
// 				assert.Equal(t, tt.want, got)
// 			}
// 		})
// 	}
// }
