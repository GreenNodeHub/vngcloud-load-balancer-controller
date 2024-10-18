package provider

import (
	"context"
	"testing"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
)

func templateCreateLoadBalancerRequest() *loadbalancerv2.CreateLoadBalancerRequest {
	opt := &loadbalancerv2.CreateLoadBalancerRequest{
		Name:         "test",
		PackageID:    "packageID",
		Scheme:       loadbalancerv2.InternetLoadBalancerScheme,
		AutoScalable: true,
		SubnetID:     "subnetID",
		Type:         loadbalancerv2.LoadBalancerTypeLayer4,
		Listener:     nil,
		Pool:         nil,
		Tags:         nil,
		UserAgent: common.UserAgent{
			Agent: []string{"test"},
		},
	}
	return opt
}

func templateCreatePoolRequest() *loadbalancerv2.CreatePoolRequest {
	opt := &loadbalancerv2.CreatePoolRequest{
		Algorithm:     loadbalancerv2.PoolAlgorithmLeastConn,
		PoolName:      "test",
		PoolProtocol:  loadbalancerv2.PoolProtocolTCP,
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: nil,
		Members:       nil,
		// LoadBalancerCommon: common.LoadBalancerCommon{
		// 	LoadBalancerId: "loadBalancerId",
		// },
		UserAgent: common.UserAgent{
			Agent: []string{"test"},
		},
	}
	return opt
}

func templateCreateListenerRequest() *loadbalancerv2.CreateListenerRequest {
	opt := &loadbalancerv2.CreateListenerRequest{
		ListenerName:                "test",
		AllowedCidrs:                "0.0.0.0/0",
		ListenerProtocol:            loadbalancerv2.ListenerProtocolTCP,
		ListenerProtocolPort:        80,
		TimeoutClient:               1000,
		TimeoutConnection:           1000,
		TimeoutMember:               1000,
		CertificateAuthorities:      nil,
		ClientCertificate:           nil,
		DefaultCertificateAuthority: nil,
		Headers:                     nil,
		// LoadBalancerCommon: common.LoadBalancerCommon{
		// 	LoadBalancerId: "loadBalancerId",
		// },
		UserAgent: common.UserAgent{
			Agent: []string{"test"},
		},
		DefaultPoolId: nil,
	}
	return opt
}

func templateHealthMonitorRequest() *loadbalancerv2.HealthMonitor {
	opt := &loadbalancerv2.HealthMonitor{
		HealthCheckProtocol: loadbalancerv2.HealthCheckProtocolTCP,
		HealthyThreshold:    2,
		UnhealthyThreshold:  2,
		Interval:            5,
		Timeout:             5,
		HealthCheckMethod:   nil,
		HttpVersion:         nil,
		HealthCheckPath:     nil,
		DomainName:          nil,
		SuccessCode:         nil,
	}
	return opt
}

func templatePoolMemeberRequest() *loadbalancerv2.Member {
	opt := &loadbalancerv2.Member{
		IpAddress:   "",
		Port:        80,
		Weight:      1,
		Backup:      false,
		MonitorPort: 80,
		Name:        "",
	}
	return opt
}

func TestCreateLoadBalancerMock(t *testing.T) {
	provider := NewMockProvider()
	opt := templateCreateLoadBalancerRequest()
	opt.Pool = templateCreatePoolRequest()
	opt.Pool.WithHealthMonitor(templateHealthMonitorRequest())
	opt.Pool.WithMembers(
		templatePoolMemeberRequest(),
		templatePoolMemeberRequest(),
	)
	opt.Listener = templateCreateListenerRequest()
	provider.CreateLoadBalancer(context.TODO(), opt)
}
