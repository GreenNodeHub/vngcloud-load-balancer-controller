package vngcloud_repo

import (
	"context"
	"fmt"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/version"
	cuongpigerutils "github.com/cuongpiger/joat/utils"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-go-sdk/v2/client"
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

const (
	RequestIcon = "🌐"
	WaitIcon    = "⏳"
	ReadyIcon   = "👍"

	defaultOffset = 0
	// defaultPageList = 1
	defaultPageSize = 1000
)

func NewVngCloudRepository(ctx context.Context, cfg *config.Config) (repository.IVngCloudRepository, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	vngcloudRepo := &vngCloudRepository{
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

type vngCloudRepository struct {
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

func (m *vngCloudRepository) setupProjectId(ctx context.Context, pmetadataService metadata.IMetadata) error {
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
