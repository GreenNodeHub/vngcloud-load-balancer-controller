package service_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildLoadBalancerName(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		expectedName string
		description  string
	}{
		{
			name: "with_load_balancer_name_annotation",
			annotations: map[string]string{
				consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixLoadBalancerName: "custom-lb-name",
			},
			expectedName: "custom-lb-name",
			description:  "should return the custom load balancer name from annotation",
		},
		{
			name: "without_load_balancer_name_annotation",
			annotations: map[string]string{
				"some.other.annotation": "value",
			},
			expectedName: "vks-TODO-test-names-test-servi-0879b",
			description:  "should return empty when no annotation is provided",
		},
		{
			name: "empty_load_balancer_name_annotation",
			annotations: map[string]string{
				consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixLoadBalancerName: "",
			},
			expectedName: "vks-TODO-test-names-test-servi-0879b",
			description:  "should return empty when annotation is empty",
		},
		{
			name: "with_multiple_annotations",
			annotations: map[string]string{
				consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixLoadBalancerName: "another-custom-name",
				"other.annotation": "value",
			},
			expectedName: "another-custom-name",
			description:  "should return custom name even with other annotations present",
		},
		// { TODO: implement truncation test
		// 	name:         "with_long_service_name",
		// 	annotations:  map[string]string{},
		// 	expectedName: "vks-TODO-test-names-test-servi-0879b",
		// 	description:  "should truncate long service names appropriately",
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a service with annotations
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			annotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
			nameHelper := utils.NewNameHelper("TODO", "service", service.GetNamespace(), service.GetName())

			// Create the task with mocks
			task := &defaultModelBuildTask{
				service:          service,
				annotationParser: annotationParser,
				nameHelper:       nameHelper,
			}

			// Call the function
			result := task.buildLoadBalancerName(context.Background())

			// Assert
			assert.Equal(t, tt.expectedName, result, tt.description)
		})
	}
}
