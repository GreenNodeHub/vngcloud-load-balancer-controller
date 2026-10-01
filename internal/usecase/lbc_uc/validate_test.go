package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func TestValidateCrossListenerPorts(t *testing.T) {
	tests := []struct {
		name    string
		lbId    string
		allLBCs *v1alpha1.LoadBalancerConfigList
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - same port same protocol across multiple LBCs",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - same port different protocols",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolTCP},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "port 80 has multiple protocols",
		},
		{
			name: "valid - same port different protocols but different load balancers",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-456"), // Different LB
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolTCP},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - different ports can have different protocols on same LB",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
								{ProtocolPort: 443, Protocol: loadbalancerv2.ListenerProtocolHTTPS},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 8080, Protocol: loadbalancerv2.ListenerProtocolTCP},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - single LBC with single listener",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - three LBCs with same port but different protocols",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc3", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolTCP},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "port 80 has multiple protocols",
		},
		{
			name: "valid - LBC uses Status.LoadBalancerId instead of Spec",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
							},
						},
						Status: v1alpha1.LoadBalancerConfigStatus{
							LoadBalancerId: ptr.To("lb-123"),
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				logger: logrus.WithField("test", tt.name),
			}

			err := task.validateCrossListenerPorts(context.Background(), tt.lbId, tt.allLBCs)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCrossListenerDefaultPools(t *testing.T) {
	tests := []struct {
		name    string
		lbId    string
		allLBCs *v1alpha1.LoadBalancerConfigList
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - same port same default pool across multiple LBCs",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - same port different default pools",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool2")},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "port 80 has different default pools",
		},
		{
			name: "valid - same port both have nil (no default pool)",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: nil},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: nil},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - same port one has default pool, other has nil",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: nil},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "port 80 has different default pools",
		},
		{
			name: "valid - same port different pools but different load balancers",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-456"), // Different LB
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool2")},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - different ports can have different default pools on same LB",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
								{ProtocolPort: 443, DefaultPoolName: ptr.To("pool2")},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - single LBC with single listener",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - three LBCs, two with pool1, one with pool2",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc3", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool2")},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "port 80 has different default pools",
		},
		{
			name: "valid - LBC uses Status.LoadBalancerId instead of Spec",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
						Status: v1alpha1.LoadBalancerConfigStatus{
							LoadBalancerId: ptr.To("lb-123"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error message contains 'no default pool' when nil",
			lbId: "lb-123",
			allLBCs: &v1alpha1.LoadBalancerConfigList{
				Items: []v1alpha1.LoadBalancerConfig{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: nil},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "lbc2", Namespace: "default"},
						Spec: v1alpha1.LoadBalancerConfigSpec{
							LoadBalancerId: ptr.To("lb-123"),
							Listeners: []v1alpha1.Listener{
								{ProtocolPort: 80, DefaultPoolName: ptr.To("pool1")},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "<no default pool>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				logger: logrus.WithField("test", tt.name),
			}

			err := task.validateCrossListenerDefaultPools(context.Background(), tt.lbId, tt.allLBCs)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSelfListenerPorts(t *testing.T) {
	tests := []struct {
		name     string
		lbConfig *v1alpha1.LoadBalancerConfig
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid - no duplicate ports",
			lbConfig: &v1alpha1.LoadBalancerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
				Spec: v1alpha1.LoadBalancerConfigSpec{
					Listeners: []v1alpha1.Listener{
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
						{ProtocolPort: 443, Protocol: loadbalancerv2.ListenerProtocolHTTPS},
						{ProtocolPort: 8080, Protocol: loadbalancerv2.ListenerProtocolTCP},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - duplicate port same protocol",
			lbConfig: &v1alpha1.LoadBalancerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
				Spec: v1alpha1.LoadBalancerConfigSpec{
					Listeners: []v1alpha1.Listener{
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate listener port 80",
		},
		{
			name: "invalid - duplicate port different protocol",
			lbConfig: &v1alpha1.LoadBalancerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
				Spec: v1alpha1.LoadBalancerConfigSpec{
					Listeners: []v1alpha1.Listener{
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolTCP},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate listener port 80",
		},
		{
			name: "valid - single listener",
			lbConfig: &v1alpha1.LoadBalancerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
				Spec: v1alpha1.LoadBalancerConfigSpec{
					Listeners: []v1alpha1.Listener{
						{ProtocolPort: 80, Protocol: loadbalancerv2.ListenerProtocolHTTP},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - empty listeners",
			lbConfig: &v1alpha1.LoadBalancerConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "lbc1", Namespace: "default"},
				Spec: v1alpha1.LoadBalancerConfigSpec{
					Listeners: []v1alpha1.Listener{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				lbConfig: tt.lbConfig,
				logger:   logrus.WithField("test", tt.name),
			}

			err := task.validateSelfListenerPorts(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCrossListenerAcl(t *testing.T) {
	mk := func(ns, name string, blocked *string) v1alpha1.LoadBalancerConfig {
		return v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
			Spec: v1alpha1.LoadBalancerConfigSpec{LoadBalancerId: ptr.To("lb-1"),
				Listeners: []v1alpha1.Listener{{Name: "http", Protocol: "HTTP", ProtocolPort: 80, BlockedCidrs: blocked}}},
		}
	}
	run := func(self v1alpha1.LoadBalancerConfig, others ...v1alpha1.LoadBalancerConfig) (*defaultModelDeployTask, error) {
		task := &defaultModelDeployTask{logger: logrus.NewEntry(logrus.New()), lbConfig: &self}
		list := &v1alpha1.LoadBalancerConfigList{Items: append([]v1alpha1.LoadBalancerConfig{self}, others...)}
		return task, task.validateCrossListenerAcl(context.Background(), "lb-1", list)
	}

	t.Run("different blocked cidrs conflict", func(t *testing.T) {
		_, err := run(mk("default", "a", ptr.To("192.0.2.1/32")), mk("default", "b", ptr.To("198.51.100.2/32")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "port 80")
		assert.Contains(t, err.Error(), "default/b")
	})
	t.Run("same value from both is fine", func(t *testing.T) {
		_, err := run(mk("default", "a", ptr.To("192.0.2.1/32")), mk("default", "b", ptr.To("192.0.2.1/32")))
		require.NoError(t, err)
	})
	// Existing setups: one Ingress whitelists, the other says nothing. Must keep reconciling.
	t.Run("nil against a value is not a conflict, but the nil side is warned", func(t *testing.T) {
		task, err := run(mk("default", "a", nil), mk("default", "b", ptr.To("192.0.2.1/32")))
		require.NoError(t, err)
		require.Len(t, task.warnings, 1)
		assert.Contains(t, task.warnings[0], "default/b")
		assert.True(t, task.peerDeclaresAcl(80)(aclBlocked))
	})
	t.Run("the declaring side is not warned", func(t *testing.T) {
		task, err := run(mk("default", "a", ptr.To("192.0.2.1/32")), mk("default", "b", nil))
		require.NoError(t, err)
		assert.Empty(t, task.warnings)
	})
	withListener := func(ns, name string, l v1alpha1.Listener) v1alpha1.LoadBalancerConfig {
		lbc := mk(ns, name, nil)
		l.Name, l.Protocol, l.ProtocolPort = "http", "HTTP", 80
		lbc.Spec.Listeners = []v1alpha1.Listener{l}
		return lbc
	}
	// Shared LBs with different inbound-cidrs exist today; they must keep reconciling.
	t.Run("different allowed cidrs only warn", func(t *testing.T) {
		task, err := run(withListener("default", "a", v1alpha1.Listener{AllowedCidrs: ptr.To("192.0.2.0/24")}),
			withListener("default", "b", v1alpha1.Listener{AllowedCidrs: ptr.To("198.51.100.0/24")}))
		require.NoError(t, err)
		require.Len(t, task.warnings, 1)
		for _, want := range []string{"port 80", "allowed cidrs", "default/b", "192.0.2.0/24", "198.51.100.0/24"} {
			assert.Contains(t, task.warnings[0], want)
		}
	})
	t.Run("equal allowed cidrs in another form do not warn", func(t *testing.T) {
		task, err := run(withListener("default", "a", v1alpha1.Listener{AllowedCidrs: ptr.To("192.0.2.1")}),
			withListener("default", "b", v1alpha1.Listener{AllowedCidrs: ptr.To("192.0.2.1/32")}))
		require.NoError(t, err)
		assert.Empty(t, task.warnings)
	})
	t.Run("different default actions conflict", func(t *testing.T) {
		_, err := run(withListener("default", "a", v1alpha1.Listener{DefaultAction: ptr.To("drop")}),
			withListener("default", "b", v1alpha1.Listener{DefaultAction: ptr.To("accept")}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default action")
		assert.Contains(t, err.Error(), "default/b")
	})
	t.Run("other load balancers are ignored", func(t *testing.T) {
		other := mk("default", "b", ptr.To("198.51.100.2/32"))
		other.Spec.LoadBalancerId = ptr.To("lb-2")
		_, err := run(mk("default", "a", ptr.To("192.0.2.1/32")), other)
		require.NoError(t, err)
	})
}
