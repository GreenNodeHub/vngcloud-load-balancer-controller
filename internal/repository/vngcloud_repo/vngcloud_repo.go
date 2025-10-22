package vngcloud_repo

import (
	"context"
	"fmt"

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

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
)

var (
	ErrorInvalidInput            = errors.New("invalid input")
	ErrorNotImplemented          = errors.New("not implemented yet")
	ErrorNotFound                = errors.New("not found")
	ErrorLoadBalancerStatusError = errors.New("load balancer status is error")
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

func (r *VngCloudRepository) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	return nil, nil
}

func (r *VngCloudRepository) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	return nil
}

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
