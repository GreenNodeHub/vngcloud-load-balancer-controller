package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	cuongpigerutils "github.com/cuongpiger/joat/utils"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/client"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	portalv1 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/portal/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/version"
)

const (
	icon      = "🌐"
	waitIcon  = "⏳"
	readyIcon = "✅"
)

// 🌱

var _ Provider = &VNGCLOUD_Provider{}

const (
	defaultPageList = 1
	defaultPageSize = 1000
)

type VNGCLOUD_Provider struct {
	Config    *config.Config
	client    client.IClient
	projectID string
	userAgent string

	netID      string
	subnetID   string
	subnetCIDR string
}

func (m *VNGCLOUD_Provider) Init(providerIDs []string) error {
	if m.Config == nil {
		return errs.ErrorNoConfig
	}
	metadator := metadata.GetMetadataProvider(m.Config.Metadata.SearchOrder)
	err := m.setupPortalInfo(metadator)
	if err != nil {
		return err
	}

	sdkConfig := client.NewSdkConfigure().
		// WithZoneId(getValueOfEnv("VNGCLOUD_ZONE_ID")).
		// WithVLBEndpoint(getValueOfEnv("URL_VLB_ENDPOINT")).
		// WithVNetworkEndpoint(getValueOfEnv("URL_VNETWORK_ENDPOINT")).
		WithProjectId(m.projectID).
		WithClientId(m.Config.Global.ClientID).
		WithClientSecret(m.Config.Global.ClientSecret).
		WithIamEndpoint(m.Config.Global.IdentityURL).
		WithVServerEndpoint(cuongpigerutils.NormalizeURL(m.Config.Global.VServerURL) + "vserver-gateway").
		WithVLBEndpoint(cuongpigerutils.NormalizeURL(m.Config.Global.VServerURL) + "vlb-gateway")

	m.client = client.NewClient(context.Background()).WithRetryCount(1).WithSleep(10).Configure(sdkConfig)
	m.userAgent = fmt.Sprintf("vngcloud-loadbalancer-controller/%s (ChartVersion/%s)", version.Version, m.Config.ChartVersion)
	err = m.getNetworkInformation(providerIDs)
	if err != nil {
		return err
	}
	return nil
}

func (m *VNGCLOUD_Provider) setupPortalInfo(pmetadataService metadata.IMetadata) error {
	if m.Config != nil && m.Config.Global.ProjectID != "" {
		m.projectID = m.Config.Global.ProjectID
		return nil
	}
	// [cuongdm3] Get the under project ID from the metadata service
	projectID, err := pmetadataService.GetProjectID()
	if err != nil {
		logrus.Errorf("[ERROR] - setupPortalInfo: failed to get project ID from metadata service: %v", err)
		return err
	}

	// [cuongdm3] Prepare the cloud client
	cloudClient := client.NewClient(context.Background()).Configure(client.NewSdkConfigure().
		WithClientId(m.Config.Global.ClientID).
		WithClientSecret(m.Config.Global.ClientSecret).
		WithIamEndpoint(cuongpigerutils.NormalizeURL(m.Config.Global.IdentityURL)).
		WithVServerEndpoint(cuongpigerutils.NormalizeURL(m.Config.Global.VServerURL) + "vserver-gateway"))

	// [cuongdm3] Get the portal information by this under project ID
	portalResp, sdkErr := cloudClient.VServerGateway().V1().
		PortalService().GetPortalInfo(portalv1.NewGetPortalInfoRequest(projectID))
	if sdkErr != nil {
		logrus.Errorf("[ERROR] - setupPortalInfo: failed to get portal information: %v", sdkErr)
		return sdkErr.GetError()
	}

	// [cuongdm3] Congratulation, everything is OK
	// llog.V(5).Infof("[INFO] - setupPortalInfo: the portal information is %+v", portalResp)
	logrus.Infof("[INFO] - setupPortalInfo: the portal information is %+v", portalResp)
	m.projectID = portalResp.ProjectID
	return nil
}

// get network id, subnet id, subnet cidr
func (m *VNGCLOUD_Provider) getNetworkInformation(providerIDs []string) error {
	// TODO .................................................
	m.netID = "net-eada1b00-1d66-480f-a83f-ffc2218df569"
	m.subnetID = "sub-511ef030-c961-45b5-baac-9d2dadf7e44c"
	m.subnetCIDR = "10.255.0.0/24"
	return nil
}

func (m *VNGCLOUD_Provider) GetProjectID() string {
	return m.projectID
}

func (m *VNGCLOUD_Provider) GetNetworkID() string {
	return m.netID
}

func (m *VNGCLOUD_Provider) GetSubnetID() string {
	return m.subnetID
}

func (m *VNGCLOUD_Provider) GetSubnetCIDR() string {
	return m.subnetCIDR
}

// // --------------------------- Security Group ---------------------------

// func (m *VNGCLOUD_Provider) ListSecurityGroups(ctx context.Context,) ([]*objects.Secgroup, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) UpdateSecGroupsOfServer(ctx context.Context,instanceID string, secgroups []string) (*objects.Server, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) GetSecurityGroup(ctx context.Context,secgroupID string) (*objects.Secgroup, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) DeleteSecurityGroup(ctx context.Context,secgroupID string) error {
// 	logger.Error("not implemented yet")
// 	return errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) CreateSecurityGroup(ctx context.Context,name string, description string) (*objects.Secgroup, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }

// func (m *VNGCLOUD_Provider) CreateSecurityGroupRule(ctx context.Context,secgroupID string, opts *secgroup_rule.CreateOpts) (*objects.SecgroupRule, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) DeleteSecurityGroupRule(ctx context.Context,secgroupID string, ruleID string) error {
// 	logger.Error("not implemented yet")
// 	return errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) ListSecurityGroupRules(ctx context.Context,secgroupID string) ([]*objects.SecgroupRule, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }

// // --------------------------- Tags ---------------------------

func (m *VNGCLOUD_Provider) ListTags(ctx context.Context, resourceID string) (*entityv2.ListTags, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListTagsRequest(resourceID)
	tags, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListTags(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - ListTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return tags, nil
}

func (m *VNGCLOUD_Provider) CreateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create tags for resource %s", icon, resourceID)
	opt := loadbalancerv2.NewCreateTagsRequest(resourceID)
	arr := make([]string, 0)
	for k, v := range tags {
		arr = append(arr, k)
		arr = append(arr, v)
	}
	opt.WithTags(arr...)

	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateTags(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - CreateTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VNGCLOUD_Provider) UpdateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewUpdateTagsRequest(resourceID)
	arr := make([]string, 0)
	for k, v := range tags {
		arr = append(arr, k)
		arr = append(arr, v)
	}
	opt.WithTags(arr...)

	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdateTags(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdateTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

// func (m *VNGCLOUD_Provider) GetSubnet(ctx context.Context,subnetID string) (*objects.Subnet, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }

// // --------------------------- Server ---------------------------

// func (m *VNGCLOUD_Provider) GetServerByID(ctx context.Context,serverID string) (*objects.Server, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) ListServerByProviderIDs(ctx context.Context,providerIDs []string) ([]*objects.Server, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) WaitForServerActive(ctx context.Context,serverID string) {

// }

// --------------------------- Load Balancer ---------------------------

func (m *VNGCLOUD_Provider) ListLoadBalancers(ctx context.Context) (*entityv2.ListLoadBalancers, error) {
	logger := contexts.NewContext(ctx).Log()
	lbs, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		ListLoadBalancers(loadbalancerv2.NewListLoadBalancersRequest(defaultPageList, defaultPageSize))
	if sdkErr != nil {
		logger.Error("[ERROR] - ListLoadBalancers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lbs, nil
}
func (m *VNGCLOUD_Provider) GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	lb, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		GetLoadBalancerById(loadbalancerv2.NewGetLoadBalancerByIdRequest(lbID))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetLoadBalancerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lb, nil
}

func (m *VNGCLOUD_Provider) GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers(ctx)
	if err != nil {
		return nil, err
	}
	for _, lb := range allLBs.Items {
		if lb.Name == name {
			return lb, nil
		}
	}
	return nil, nil
}

func (m *VNGCLOUD_Provider) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create load balancer.", icon)
	newLB, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		CreateLoadBalancer(lbOptions)
	if sdkErr != nil {
		logger.Error("[ERROR] - CreateLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return newLB, nil
}
func (m *VNGCLOUD_Provider) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete load balancer %s", icon, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		DeleteLoadBalancerById(loadbalancerv2.NewDeleteLoadBalancerByIdRequest(lbID))
	if sdkErr != nil {
		logger.Error("[ERROR] - DeleteLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VNGCLOUD_Provider) ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Waiting for load balancer %s to be ready", waitIcon, lbID)
	var resultLb *entityv2.LoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetLoadBalancerByID(ctx, lbID)
		if err != nil {
			logger.Errorf("Error getting load balancer %s when wait active: %v", lbID, err)
			return false, err
		}
		if strings.ToUpper(lb.DisplayStatus) == consts.ACTIVE_LOADBALANCER_STATUS &&
			strings.ToUpper(lb.ProgressStatus) == consts.CREATED_LOADBALANCER_STATUS {
			logger.Infof("%s Load balancer %s is ready", readyIcon, lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Errorf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, errs.ErrorLoadBalancerStatusError
		}

		logger.Infof("%s Load balancer %s is not ready yet, waiting...", waitIcon, lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

// --------------------------- Listener ---------------------------

//	func (m *VNGCLOUD_Provider) GetListenerByName(ctx context.Context,lbID, name string) (*objects.Listener, error) {
//		logger.Error("not implemented yet")
//		return nil, errs.ErrorNotImplemented
//	}
//
//	func (m *VNGCLOUD_Provider) GetListenerByPort(ctx context.Context,lbID string, port int) (*objects.Listener, error) {
//		logger.Error("not implemented yet")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *VNGCLOUD_Provider) CreateListener(ctx context.Context, lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create listener of load balancer %s", icon, lbID)
	listener, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateListener(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - CreateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listener, nil
}
func (m *VNGCLOUD_Provider) ListListenerOfLB(ctx context.Context, lbID string) (*entityv2.ListListeners, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListListenersByLoadBalancerIdRequest(lbID)
	listeners, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListListenersByLoadBalancerId(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - ListListenerOfLB: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listeners, nil
}
func (m *VNGCLOUD_Provider) DeleteListener(ctx context.Context, lbID, listenerID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete listener %s of load balancer %s", icon, listenerID, lbID)
	opt := loadbalancerv2.NewDeleteListenerByIdRequest(lbID, listenerID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeleteListenerById(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - DeleteListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VNGCLOUD_Provider) UpdateListener(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update listener %s of load balancer %s", icon, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdateListener(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

// --------------------------- Policy ---------------------------

//	func (m *VNGCLOUD_Provider) GetPolicyByName(ctx context.Context,lbID, listenerID, name string) (*objects.Policy, error) {
//		logger.Error("not implemented yet")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *VNGCLOUD_Provider) CreatePolicy(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create policy of listener %s of load balancer %s", icon, listenerID, lbID)
	policy, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreatePolicy(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - CreatePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return policy, nil
}
func (m *VNGCLOUD_Provider) ListPolicyOfListener(ctx context.Context, lbID, listenerID string) (*entityv2.ListPolicies, error) {
	logger := contexts.NewContext(ctx).Log()
	listPolicies, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPolicies(loadbalancerv2.NewListPoliciesRequest(lbID, listenerID))
	if sdkErr != nil {
		logger.Error("[ERROR] - ListPolicyOfListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listPolicies, nil
}

//	func (m *VNGCLOUD_Provider) GetPolicyByID(ctx context.Context,policyID string) (*objects.Policy, error) {
//		logger.Error("not implemented yet")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *VNGCLOUD_Provider) UpdatePolicy(ctx context.Context, lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update policy %s of listener %s of load balancer %s", icon, policyID, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePolicy(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdatePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VNGCLOUD_Provider) DeletePolicy(ctx context.Context, lbID, listenerID, policyID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete policy %s of listener %s of load balancer %s", icon, policyID, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeletePolicyById(loadbalancerv2.NewDeletePolicyByIdRequest(lbID, listenerID, policyID))
	if sdkErr != nil {
		logger.Error("[ERROR] - DeletePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

// --------------------------- Pool ---------------------------

//	func (m *VNGCLOUD_Provider) GetPoolByName(ctx context.Context,lbID, name string) (*objects.Pool, error) {
//		logger.Error("not implemented yet")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *VNGCLOUD_Provider) CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create pool of load balancer %s", icon, lbID)
	pool, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreatePool(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - CreatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pool, nil
}
func (m *VNGCLOUD_Provider) ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListPoolsByLoadBalancerIdRequest(lbID)
	pools, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolsByLoadBalancerId(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - ListPool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pools, nil
}
func (m *VNGCLOUD_Provider) UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) GetPoolByID(ctx context.Context, lbID, poolID string) (*entityv2.Pool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListPoolMembersRequest(lbID, poolID)
	members, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolMembers(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - GetPoolMembers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return members, nil
}

func (m *VNGCLOUD_Provider) DeletePool(ctx context.Context, lbID, poolID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete pool %s of load balancer %s", icon, poolID, lbID)
	opt := loadbalancerv2.NewDeletePoolByIdRequest(lbID, poolID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeletePoolById(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - DeletePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VNGCLOUD_Provider) UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update pool %s of load balancer %s", icon, poolID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePool(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VNGCLOUD_Provider) GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewGetPoolHealthMonitorByIdRequest(lbID, poolID)
	monitor, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().GetPoolHealthMonitorById(opt)
	if sdkErr != nil {
		logger.Error("[ERROR] - GetPoolHealthMonitorById: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return monitor, nil
}

// // --------------------------- Certificate ---------------------------

// func (m *VNGCLOUD_Provider) ImportCertificate(ctx context.Context,opt *certificates.ImportOpts) (*objects.Certificate, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) ListCertificates(ctx context.Context,) ([]*objects.Certificate, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) GetCertificateByID(ctx context.Context,certID string) (*objects.Certificate, error) {
// 	logger.Error("not implemented yet")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *VNGCLOUD_Provider) DeleteCertificate(ctx context.Context,certID string) error {
// 	logger.Error("not implemented yet")
// 	return errs.ErrorNotImplemented
// }
