package consts

import (
	"fmt"
	"time"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

// when resource update these annotations, the controller will be ignored
var WhitelistedAnnotations = map[string]struct{}{
	"example.com/whitelist-annotation-1": {},
	"example.com/whitelist-annotation-2": {},

	fmt.Sprintf("%s/%s", SERVICE_ANNOTATION_PREFIX, annotations.SuffixManageListeners):  {},
	fmt.Sprintf("%s/%s", SERVICE_ANNOTATION_PREFIX, annotations.SuffixManagePools):      {},
	fmt.Sprintf("%s/%s", SERVICE_ANNOTATION_PREFIX, annotations.SuffixManageDFPMembers): {},

	fmt.Sprintf("%s/%s", INGRESS_ANNOTATION_PREFIX, annotations.SuffixManageListeners):  {},
	fmt.Sprintf("%s/%s", INGRESS_ANNOTATION_PREFIX, annotations.SuffixManagePools):      {},
	fmt.Sprintf("%s/%s", INGRESS_ANNOTATION_PREFIX, annotations.SuffixManageDFPMembers): {},
}

const (
	LABEL_NODE_EXCLUDE_LOADBALANCER = "node.kubernetes.io/exclude-from-external-load-balancers"

	// ServiceFinalizer = "service.vngcloud.vn/resources"
	ServiceFinalizer = "service.kubernetes.io/load-balancer-cleanup"

	IngressFinalizer = "ingress.vngcloud.vn/resources"
	GLBFinalizer     = "glb.vngcloud.vn/resources"

	SERVICE_ANNOTATION_PREFIX = "vks.vngcloud.vn"
	INGRESS_ANNOTATION_PREFIX = "vks.vngcloud.vn"
)
const (
	DEFAULT_LB_PREFIX_NAME = "vks" // "vks" is abbreviated of "cluster"

	DEFAULT_MEMBER_BACKUP_ROLE = false

	DEFAULT_PORTAL_NAME_LENGTH        = 50  // All the name must be less than 50 characters
	DEFAULT_PORTAL_DESCRIPTION_LENGTH = 255 // All the description must be less than 255 characters
	DEFAULT_MEMBER_WEIGHT             = 1
	DEFAULT_VLB_ID_PIECE_START_INDEX  = 8
	DEFAULT_VLB_ID_PIECE_LENGTH       = 8
	DEFAULT_HASH_NAME_LENGTH          = 5
	DEFAULT_NAME_DEFAULT_POOL         = "vks_default_pool"
	DEFAULT_HTTPS_LISTENER_NAME       = "vks_https_listener"
	DEFAULT_HTTP_LISTENER_NAME        = "vks_http_listener"
	VKS_TAG_KEY                       = "vks-cluster-ids"
	VKS_TAGS_SEPARATOR                = "_"
	VKS_CLUSTER_ID_PREFIX             = "k8s-"
	VKS_CLUSTER_ID_LENGTH             = 40

	// DeprecatedLabelNodeRoleMaster specifies that a node is a master
	// It's copied over to kubeadm until it's merged in core: https://github.com/kubernetes/kubernetes/pull/39112
	// Deprecated in favor of LabelNodeExcludeLB
	DEFAULT_K8S_MASTER_LABEL = "node-role.kubernetes.io/master"

	// LabelNodeExcludeLB specifies that a node should not be used to create a Loadbalancer on
	// https://github.com/kubernetes/cloud-provider/blob/25867882d509131a6fdeaf812ceebfd0f19015dd/controllers/service/controller.go#L673
	LabelNodeExcludeLB = "node.kubernetes.io/exclude-from-external-load-balancers"

	// IngressSecretCertName is certificate key name defined in the secret data.
	IngressSecretCertName = "tls.crt"
	// IngressSecretKeyName is private key name defined in the secret data.
	IngressSecretKeyName = "tls.key"

	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	DefaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	DefaultBurst = 1e6

	MaxRetries = 5

	// IngressKey picks a specific "class" for the Ingress.
	// The controller only processes Ingresses with this annotation either
	// unset, or set to either the configured value or the empty string.
	IngressKey = "kubernetes.io/ingress.class"

	// IngressClass specifies which Ingress class we accept
	IngressClass = "vngcloud"
)

const (
	WaitLoadbalancerInitDelay   = 5 * time.Second
	WaitLoadbalancerFactor      = 1.2
	WaitLoadbalancerActiveSteps = 30
	WaitLoadbalancerDeleteSteps = 12
)

const (
	PROVIDER_NAME               = "vngcloud"
	ACTIVE_LOADBALANCER_STATUS  = "ACTIVE"
	CREATED_LOADBALANCER_STATUS = "CREATED"
	ERROR_LOADBALANCER_STATUS   = "ERROR"

	RESOURCE_TYPE_SERVICE = "service"
	RESOURCE_TYPE_INGRESS = "ingress"
)

const (
	FleetServiceNameLabel      = "fleet.vngcloud.vn/service-name"
	FleetServiceNamespaceLabel = "fleet.vngcloud.vn/service-namespace"
	FleetIDLabel               = "fleet.vngcloud.vn/fleet-id"
	FleetServiceIDLabel        = "fleet.vngcloud.vn/fleet-service-id"
	FleetClusterIDLabel        = "fleet.vngcloud.vn/cluster-id"

	ConfigClusterIdAnnotation = "fleet.vngcloud.vn/config-cluster-id"
	TriggerAnnotation         = "fleet.vngcloud.vn/trigger"
)
