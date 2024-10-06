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
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/certificates"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/policy"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/network/v2/extensions/secgroup_rule"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/version"
)

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
	logger    *logrus.Entry

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
	m.logger = contexts.NewContext(context.Background()).Log()
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

// --------------------------- Security Group ---------------------------

func (m *VNGCLOUD_Provider) ListSecurityGroups() ([]*objects.Secgroup, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) UpdateSecGroupsOfServer(instanceID string, secgroups []string) (*objects.Server, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) GetSecurityGroup(secgroupID string) (*objects.Secgroup, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) DeleteSecurityGroup(secgroupID string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) CreateSecurityGroup(name string, description string) (*objects.Secgroup, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) CreateSecurityGroupRule(secgroupID string, opts *secgroup_rule.CreateOpts) (*objects.SecgroupRule, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) DeleteSecurityGroupRule(secgroupID string, ruleID string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) ListSecurityGroupRules(secgroupID string) ([]*objects.SecgroupRule, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}

// --------------------------- Tags ---------------------------

func (m *VNGCLOUD_Provider) GetTags(resourceID string) ([]*objects.ResourceTag, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) UpdateTags(resourceID string, tags map[string]string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) GetSubnet(subnetID string) (*objects.Subnet, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}

// --------------------------- Server ---------------------------

func (m *VNGCLOUD_Provider) GetServerByID(serverID string) (*objects.Server, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) ListServerByProviderIDs(providerIDs []string) ([]*objects.Server, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) WaitForServerActive(serverID string) {

}

// --------------------------- Load Balancer ---------------------------

func (m *VNGCLOUD_Provider) ListLoadBalancers() (*entityv2.ListLoadBalancers, error) {
	lbs, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		ListLoadBalancers(loadbalancerv2.NewListLoadBalancersRequest(defaultPageList, defaultPageSize))
	if sdkErr != nil {
		m.logger.Error("[ERROR] - ListLoadBalancers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lbs, nil
}
func (m *VNGCLOUD_Provider) GetLoadBalancerByID(lbID string) (*entityv2.LoadBalancer, error) {
	lb, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		GetLoadBalancerById(loadbalancerv2.NewGetLoadBalancerByIdRequest(lbID))
	if sdkErr != nil {
		m.logger.Error("[ERROR] - GetLoadBalancerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lb, nil
}

func (m *VNGCLOUD_Provider) GetLoadBalancerByName(name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers()
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

func (m *VNGCLOUD_Provider) CreateLoadBalancer(lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	newLB, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		CreateLoadBalancer(lbOptions)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - CreateLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return newLB, nil
}
func (m *VNGCLOUD_Provider) DeleteLoadBalancer(lbID string) error {
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().
		DeleteLoadBalancerById(loadbalancerv2.NewDeleteLoadBalancerByIdRequest(lbID))
	if sdkErr != nil {
		m.logger.Error("[ERROR] - DeleteLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VNGCLOUD_Provider) ResizeLoadBalancer(lbID, packageID string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) WaitForLBActive(lbID string) (*entityv2.LoadBalancer, error) {
	m.logger.Infof("Waiting for load balancer %s to be ready", lbID)
	var resultLb *entityv2.LoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetLoadBalancerByID(lbID)
		if err != nil {
			m.logger.Errorf("Error getting load balancer %s when wait active: %v", lbID, err)
			return false, err
		}
		if strings.ToUpper(lb.DisplayStatus) == consts.ACTIVE_LOADBALANCER_STATUS &&
			strings.ToUpper(lb.ProgressStatus) == consts.CREATED_LOADBALANCER_STATUS {
			m.logger.Infof("Load balancer %s is ready", lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			m.logger.Errorf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, errs.ErrorLoadBalancerStatusError
		}

		m.logger.Infof("Load balancer %s is not ready yet, waiting...", lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		m.logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

// --------------------------- Listener ---------------------------

func (m *VNGCLOUD_Provider) GetListenerByName(lbID, name string) (*objects.Listener, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) GetListenerByPort(lbID string, port int) (*objects.Listener, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) CreateListener(lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
	listener, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateListener(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - CreateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listener, nil
}
func (m *VNGCLOUD_Provider) ListListenerOfLB(lbID string) (*entityv2.ListListeners, error) {
	opt := loadbalancerv2.NewListListenersByLoadBalancerIdRequest(lbID)
	listeners, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListListenersByLoadBalancerId(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - ListListenerOfLB: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listeners, nil
}
func (m *VNGCLOUD_Provider) DeleteListener(lbID, listenerID string) error {
	opt := loadbalancerv2.NewDeleteListenerByIdRequest(lbID, listenerID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeleteListenerById(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - DeleteListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VNGCLOUD_Provider) UpdateListener(lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error {
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdateListener(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - UpdateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

// --------------------------- Policy ---------------------------

func (m *VNGCLOUD_Provider) GetPolicyByName(lbID, listenerID, name string) (*objects.Policy, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) CreatePolicy(lbID, listenerID string, opt *policy.CreateOptsBuilder) (*objects.Policy, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) ListPolicyOfListener(lbID, listenerID string) ([]*objects.Policy, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) GetPolicyByID(policyID string) (*objects.Policy, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) UpdatePolicy(lbID, listenerID, policyID string, opt *policy.UpdateOptsBuilder) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) DeletePolicy(lbID, listenerID, policyID string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}

// --------------------------- Pool ---------------------------

func (m *VNGCLOUD_Provider) GetPoolByName(lbID, name string) (*objects.Pool, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) CreatePool(lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
	pool, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreatePool(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - CreatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pool, nil
}
func (m *VNGCLOUD_Provider) ListPool(lbID string) (*entityv2.ListPools, error) {
	opt := loadbalancerv2.NewListPoolsByLoadBalancerIdRequest(lbID)
	pools, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolsByLoadBalancerId(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - ListPool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pools, nil
}
func (m *VNGCLOUD_Provider) UpdatePoolMembers(lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) GetPoolByID(lbID, poolID string) (*objects.Pool, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}

func (m *VNGCLOUD_Provider) GetPoolMembers(lbID, poolID string) (*entityv2.ListMembers, error) {
	opt := loadbalancerv2.NewListPoolMembersRequest(lbID, poolID)
	members, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolMembers(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - GetPoolMembers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return members, nil
}

func (m *VNGCLOUD_Provider) DeletePool(lbID, poolID string) error {
	opt := loadbalancerv2.NewDeletePoolByIdRequest(lbID, poolID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeletePoolById(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - DeletePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VNGCLOUD_Provider) UpdatePool(lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error {
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePool(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - UpdatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VNGCLOUD_Provider) GetPoolHealthMonitorById(lbID, poolID string) (*entityv2.HealthMonitor, error) {
	opt := loadbalancerv2.NewGetPoolHealthMonitorByIdRequest(lbID, poolID)
	monitor, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().GetPoolHealthMonitorById(opt)
	if sdkErr != nil {
		m.logger.Error("[ERROR] - GetPoolHealthMonitorById: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return monitor, nil
}

// --------------------------- Certificate ---------------------------

func (m *VNGCLOUD_Provider) ImportCertificate(opt *certificates.ImportOpts) (*objects.Certificate, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) ListCertificates() ([]*objects.Certificate, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) GetCertificateByID(certID string) (*objects.Certificate, error) {
	m.logger.Error("not implemented yet")
	return nil, errs.ErrorNotImplemented
}
func (m *VNGCLOUD_Provider) DeleteCertificate(certID string) error {
	m.logger.Error("not implemented yet")
	return errs.ErrorNotImplemented
}
