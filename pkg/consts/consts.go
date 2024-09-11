package consts

// when resource update these annotations, the controller will be ignored
var WhitelistedAnnotations = map[string]struct{}{
	"example.com/whitelist-annotation-1": {},
	"example.com/whitelist-annotation-2": {},
}

const (
	DEFAULT_K8S_MASTER_LABEL        = "node-role.kubernetes.io/master"
	LABEL_NODE_EXCLUDE_LOADBALANCER = "node.kubernetes.io/exclude-from-external-load-balancers"

	IngressClass = "vngcloud"

	ServiceFinalizer = "service.vngcloud.vn/resources"
	IngressFinalizer = "ingress.vngcloud.vn/resources"

	SERVICE_ANNOTATION_PREFIX = "vks.vngcloud.vn"
	INGRESS_ANNOTATION_PREFIX = "vks.vngcloud.vn"
)
