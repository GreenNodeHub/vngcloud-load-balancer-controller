package ingress_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

func TestBuildSubnetAndZone(t *testing.T) {
	defaultZone := common.Zone("zone-default")
	defaultNetworkId := "net-default"
	defaultSubnetId := "subnet-default"
	defaultSubnetCIDR := "10.0.0.0/24"

	tests := []struct {
		name             string
		annotations      map[string]string
		existingSubnetId string
		setupMocks       func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository)
		expectedZone     common.Zone
		expectedSubnetId string
		expectedCIDR     string
		expectError      bool
	}{
		{
			name:             "no_annotations_no_existing_lbc_returns_defaults",
			annotations:      map[string]string{},
			existingSubnetId: "",
			setupMocks:       func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {},
			expectedZone:     defaultZone,
			expectedSubnetId: defaultSubnetId,
			expectedCIDR:     defaultSubnetCIDR,
			expectError:      false,
		},
		{
			name:             "existing_lbc_with_different_subnet_uses_existing",
			annotations:      map[string]string{},
			existingSubnetId: "subnet-existing",
			setupMocks: func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {
				vngcloud.EXPECT().
					GetSubnetByID(mock.Anything, defaultNetworkId, "subnet-existing").
					Return(&entity.Subnet{
						Id:     "subnet-existing",
						ZoneID: "zone-existing",
						Cidr:   "10.0.8.0/24",
					}, nil)
			},
			expectedZone:     common.Zone("zone-existing"),
			expectedSubnetId: "subnet-existing",
			expectedCIDR:     "10.0.8.0/24",
			expectError:      false,
		},
		{
			name:             "existing_lbc_with_same_default_subnet_returns_defaults",
			annotations:      map[string]string{},
			existingSubnetId: defaultSubnetId,
			setupMocks:       func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {},
			expectedZone:     defaultZone,
			expectedSubnetId: defaultSubnetId,
			expectedCIDR:     defaultSubnetCIDR,
			expectError:      false,
		},
		{
			name: "lb_id_annotation_takes_priority_over_existing_lbc",
			annotations: map[string]string{
				domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixLoadBalancerID: "lb-123",
			},
			existingSubnetId: "subnet-existing",
			setupMocks: func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {
				vngcloud.EXPECT().
					GetLoadBalancerByID(mock.Anything, "lb-123").
					Return(&entity.LoadBalancer{
						BackendSubnetID: "subnet-from-lb",
					}, nil)
				vngcloud.EXPECT().
					GetSubnetByID(mock.Anything, defaultNetworkId, "subnet-from-lb").
					Return(&entity.Subnet{
						Id:     "subnet-from-lb",
						ZoneID: "zone-from-lb",
						Cidr:   "10.0.16.0/24",
					}, nil)
			},
			expectedZone:     common.Zone("zone-from-lb"),
			expectedSubnetId: "subnet-from-lb",
			expectedCIDR:     "10.0.16.0/24",
			expectError:      false,
		},
		{
			name: "existing_lbc_takes_priority_over_prefer_subnet_annotation",
			annotations: map[string]string{
				domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixPreferSubnetID: "subnet-prefer",
			},
			existingSubnetId: "subnet-existing",
			setupMocks: func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {
				vngcloud.EXPECT().
					GetSubnetByID(mock.Anything, defaultNetworkId, "subnet-existing").
					Return(&entity.Subnet{
						Id:     "subnet-existing",
						ZoneID: "zone-existing",
						Cidr:   "10.0.8.0/24",
					}, nil)
			},
			expectedZone:     common.Zone("zone-existing"),
			expectedSubnetId: "subnet-existing",
			expectedCIDR:     "10.0.8.0/24",
			expectError:      false,
		},
		{
			name: "prefer_subnet_annotation_used_when_no_existing_lbc",
			annotations: map[string]string{
				domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixPreferSubnetID: "subnet-prefer",
			},
			existingSubnetId: "",
			setupMocks: func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository) {
				vngcloud.EXPECT().
					GetSubnetByID(mock.Anything, defaultNetworkId, "subnet-prefer").
					Return(&entity.Subnet{
						Id:     "subnet-prefer",
						ZoneID: "zone-prefer",
						Cidr:   "10.0.4.0/24",
					}, nil)
			},
			expectedZone:     common.Zone("zone-prefer"),
			expectedSubnetId: "subnet-prefer",
			expectedCIDR:     "10.0.4.0/24",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloud := repository.NewMockVngCloudRepository(t)
			mockK8s := repository.NewMockK8sRepository(t)
			tt.setupMocks(mockVngcloud, mockK8s)

			logger := logrus.NewEntry(logrus.New())
			ing := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			task := &defaultModelBuildTask{
				ingress:           ing,
				annotationParser:  annotations.NewSuffixAnnotationParser(domain.INGRESS_ANNOTATION_PREFIX),
				vngcloudRepo:      mockVngcloud,
				k8sRepo:           mockK8s,
				logger:            logger,
				defaultZone:       defaultZone,
				defaultNetworkId:  defaultNetworkId,
				defaultSubnetId:   defaultSubnetId,
				defaultSubnetCIDR: defaultSubnetCIDR,
			}

			zone, _, subnetId, cidr, err := task.buildSubnetAndZone(context.Background(), tt.existingSubnetId)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedZone, zone)
				assert.Equal(t, tt.expectedSubnetId, subnetId)
				assert.Equal(t, tt.expectedCIDR, cidr)
			}
		})
	}
}
