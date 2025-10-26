package vngcloud_repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/version"
	cuongpigerutils "github.com/cuongpiger/joat/utils"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-go-sdk/v2/client"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	computev2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/compute/v2"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	portalv1 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/portal/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
)

var (
	ErrorInvalidInput            = errors.New("invalid input")
	ErrorNotImplemented          = errors.New("not implemented yet")
	ErrorNotFound                = errors.New("not found")
	ErrorLoadBalancerStatusError = errors.New("load balancer status is error")
)

const (
	icon      = "🌐"
	waitIcon  = "⏳"
	readyIcon = "👍"

	defaultOffset = 0
	// defaultPageList = 1
	defaultPageSize = 1000
)

func NewVngCloudRepository(ctx context.Context, cfg *config.Config) (repository.IVngCloudRepository, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	vngcloudRepo := &VngCloudRepository{
		cfg: cfg,
	}

	metadator := metadata.GetMetadataProvider(vngcloudRepo.cfg.Metadata.SearchOrder)
	err := vngcloudRepo.setupProjectId(ctx, metadator)
	if err != nil {
		return nil, err
	}

	sdkConfig := client.NewSdkConfigure().
		// WithZoneId(getValueOfEnv("VNGCLOUD_ZONE_ID")).
		// WithVLBEndpoint(getValueOfEnv("URL_VLB_ENDPOINT")).
		// WithVNetworkEndpoint(getValueOfEnv("URL_VNETWORK_ENDPOINT")).
		WithProjectId(vngcloudRepo.projectId).
		WithClientId(vngcloudRepo.cfg.Global.ClientID).
		WithClientSecret(vngcloudRepo.cfg.Global.ClientSecret).
		WithIamEndpoint(vngcloudRepo.cfg.Global.IdentityURL).
		WithVServerEndpoint(cuongpigerutils.NormalizeURL(vngcloudRepo.cfg.Global.VServerURL) + "vserver-gateway").
		WithVLBEndpoint(cuongpigerutils.NormalizeURL(vngcloudRepo.cfg.Global.VServerURL) + "vlb-gateway").
		WithGLBEndpoint("https://glb.console.vngcloud.vn/glb-controller/")

	vngcloudRepo.client = client.NewClient(context.Background()).WithRetryCount(1).WithSleep(10).Configure(sdkConfig)
	vngcloudRepo.userAgent = fmt.Sprintf("vngcloud-loadbalancer-controller/%s (ChartVersion/%s)", version.Version, vngcloudRepo.cfg.ChartVersion)
	// TODO init network info provider

	return vngcloudRepo, nil
}

type VngCloudRepository struct {
	cfg       *config.Config
	client    client.IClient
	userAgent string
	projectId string

	// zoneID     common.Zone
	// netID      string
	// netCIDR    string
	// subnetID   string
	// subnetCIDR string

	// // TODO: add cache for network info
	// cacheInstanceIDToSubnetID sync.Map // map[string]string
	// cacheSubnetIDToCIDR       sync.Map // map[string]string
	// cacheSubnetIDToNetworkID  sync.Map // map[string]string
	// cacheNetworkIDToCIDR      sync.Map // map[string]string
	// cacheSubnetIDToZoneID     sync.Map // map[string]common.Zone
}

func (m *VngCloudRepository) setupProjectId(ctx context.Context, pmetadataService metadata.IMetadata) error {
	if m.cfg != nil && m.cfg.Global.ProjectID != "" {
		m.projectId = m.cfg.Global.ProjectID
		return nil
	}

	logger := contexts.NewContext(ctx).Log()

	// [cuongdm3] Get the under project ID from the metadata service
	projectID, err := pmetadataService.GetProjectID()
	if err != nil {
		logger.Errorf("[ERROR] - setupProjectId: failed to get project ID from metadata service: %v", err)
		return err
	}

	// [cuongdm3] Prepare the cloud client
	cloudClient := client.NewClient(ctx).Configure(client.NewSdkConfigure().
		WithClientId(m.cfg.Global.ClientID).
		WithClientSecret(m.cfg.Global.ClientSecret).
		WithIamEndpoint(cuongpigerutils.NormalizeURL(m.cfg.Global.IdentityURL)).
		WithVServerEndpoint(cuongpigerutils.NormalizeURL(m.cfg.Global.VServerURL) + "vserver-gateway"))

	// [cuongdm3] Get the portal information by this under project ID
	portalResp, sdkErr := cloudClient.VServerGateway().V1().
		PortalService().GetPortalInfo(portalv1.NewGetPortalInfoRequest(projectID))
	if sdkErr != nil {
		logger.Errorf("[ERROR] - setupProjectId: failed to get portal information: %v", sdkErr)
		return sdkErr.GetError()
	}

	// [cuongdm3] Congratulation, everything is OK
	// llog.V(5).Infof("[INFO] - setupProjectId: the portal information is %+v", portalResp)
	logger.Infof("[INFO] - setupProjectId: the portal information is %+v", portalResp)
	m.projectId = portalResp.ProjectID
	return nil
}

// --------------------------- Load Balancer ---------------------------

func (m *VngCloudRepository) ListLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListLoadBalancers, error) {
	logger := contexts.NewContext(ctx).Log()
	lbs, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListLoadBalancers(loadbalancerv2.NewListLoadBalancersRequest(defaultOffset, defaultPageSize).WithTags(tags...).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - ListLoadBalancers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lbs, nil
}
func (m *VngCloudRepository) GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	lb, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().GetLoadBalancerById(loadbalancerv2.NewGetLoadBalancerByIdRequest(lbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetLoadBalancerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return lb, nil
}

func (m *VngCloudRepository) GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers(ctx, nil)
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

func (m *VngCloudRepository) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create load balancer.", icon)
	newLB, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateLoadBalancer(lbOptions.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - CreateLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return newLB, nil
}
func (m *VngCloudRepository) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete load balancer %s", icon, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeleteLoadBalancerById(loadbalancerv2.NewDeleteLoadBalancerByIdRequest(lbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - DeleteLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VngCloudRepository) ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request resize load balancer %s to package %s", icon, lbID, packageID)

	opt := loadbalancerv2.NewResizeLoadBalancerRequest(lbID, packageID)
	_, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ResizeLoadBalancer(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - ResizeLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *VngCloudRepository) WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
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
		if strings.ToUpper(lb.DisplayStatus) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Errorf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, ErrorLoadBalancerStatusError
		}

		logger.Infof("%s Load balancer %s is not ready yet, waiting...", waitIcon, lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

////////////////////////////////////////////////////////////////////////

func (r *VngCloudRepository) GetSubnetByID(ctx context.Context, networkID, subnetID string) (*entityv2.Subnet, error) {
	logger := contexts.NewContext(ctx).Log()
	subnet, sdkErr := r.client.VServerGateway().V2().NetworkService().GetSubnetById(networkv2.NewGetSubnetByIdRequest(networkID, subnetID).AddUserAgent(r.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetSubnetByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return subnet, nil
}

func (m *VngCloudRepository) GetServerNetworkInfo(ctx context.Context, instanceID string) (zoneID common.Zone, networkID, subnetID, subnetCIDR string, err error) {
	logger := contexts.NewContext(ctx).Log()

	if instanceID == "" {
		logger.Error("[ERROR] - GetServerNetworkInfo: serverID is empty")
		return "", "", "", "", ErrorInvalidInput
	}

	server, sdkErr := m.GetServerByID(ctx, instanceID)
	if sdkErr != nil {
		return "", "", "", "", sdkErr
	}
	if server == nil {
		return "", "", "", "", ErrorNotFound
	}

	networkID = server.InternalInterfaces[0].NetworkUuid
	subnetID = server.InternalInterfaces[0].SubnetUuid
	zoneID = common.Zone(server.ZoneId)

	if subnetID == "" {
		logger.Errorf("[ERROR] - GetServerNetworkInfo: failed to get network information, subnetID: %s", subnetID)
		return "", "", "", "", ErrorNotFound
	}

	subnetCIDR, err = m.getSubnetCIDR(ctx, networkID, subnetID)
	if err != nil {
		logger.Errorf("[ERROR] - GetServerNetworkInfo: failed to get subnet CIDR: %v", err)
		return "", "", "", "", err
	}

	return common.Zone(zoneID), networkID, subnetID, subnetCIDR, nil
}

func (m *VngCloudRepository) GetServerByID(ctx context.Context, serverID string) (*entityv2.Server, error) {
	logger := contexts.NewContext(ctx).Log()

	opt := computev2.NewGetServerByIdRequest(serverID)
	server, sdkErr := m.client.VServerGateway().V2().ComputeService().GetServerById(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetServerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return server, nil
}

func (m *VngCloudRepository) getSubnetCIDR(ctx context.Context, networkId, subnetID string) (string, error) {
	subnet, err := m.GetSubnetByID(ctx, networkId, subnetID)
	if err != nil {
		return "", err
	}
	if subnet == nil {
		return "", ErrorNotFound
	}
	return subnet.Cidr, nil
}

// --------------------------- Pool ---------------------------

//	func (m *VngCloudRepository) GetPoolByName(ctx context.Context,lbID, name string) (*objects.Pool, error) {
//		logger.Error("not implemented yet")
//		return nil, ErrorNotImplemented
//	}
func (m *VngCloudRepository) CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create pool of load balancer %s", icon, lbID)
	pool, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreatePool(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - CreatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pool, nil
}
func (m *VngCloudRepository) ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListPoolsByLoadBalancerIdRequest(lbID)
	pools, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolsByLoadBalancerId(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - ListPool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return pools, nil
}
func (m *VngCloudRepository) UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update pool members of pool %s of load balancer %s", icon, poolID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePoolMembers(members.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdatePoolMembers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VngCloudRepository) GetPoolByID(ctx context.Context, lbID, poolID string) (*entityv2.Pool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Error("not implemented yet")
	return nil, ErrorNotImplemented
}

func (m *VngCloudRepository) GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListPoolMembersRequest(lbID, poolID)
	members, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPoolMembers(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetPoolMembers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return members, nil
}

func (m *VngCloudRepository) DeletePool(ctx context.Context, lbID, poolID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete pool %s of load balancer %s", icon, poolID, lbID)
	opt := loadbalancerv2.NewDeletePoolByIdRequest(lbID, poolID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeletePoolById(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - DeletePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VngCloudRepository) UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update pool %s of load balancer %s", icon, poolID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePool(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdatePool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}

func (m *VngCloudRepository) GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewGetPoolHealthMonitorByIdRequest(lbID, poolID)
	monitor, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().GetPoolHealthMonitorById(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - GetPoolHealthMonitorById: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return monitor, nil
}
