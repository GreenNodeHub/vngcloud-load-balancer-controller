package utils

import (
	"context"
	"fmt"
	"slices"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const PodKind = "Pod"

var ErrNotFound = errors.New("backend not found")
var ErrNodeDoesNotHaveInternalAddress = errors.New("node does not have internal address")

// An endpoint provided by pod directly.
type EndpointAddress struct {
	Name string
	IP   string
	Port int
}

// EndpointResolver resolves the endpoints for specific service & service Port.
type EndpointResolver interface {
	ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
		opts ...EndpointResolveOption) ([]EndpointAddress, error)
	ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
		opts ...EndpointResolveOption) ([]EndpointAddress, error)

	// GetListTargetPort returns the list of target ports of a service's port.
	GetListTargetPort(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString) ([]int, error)
}

// NewDefaultEndpointResolver constructs new defaultEndpointResolver
func NewDefaultEndpointResolver(context context.Context, k8sClient client.Client) EndpointResolver {
	return &defaultEndpointResolver{
		k8sClient: k8sClient,
		context:   context,
	}
}

var _ EndpointResolver = &defaultEndpointResolver{}

// default implementation for EndpointResolver
type defaultEndpointResolver struct {
	k8sClient client.Client
	context   context.Context

	// [NodePort Endpoint] if fail-open enabled, then nodes that have `Unknown` ready
	// condition will be included if there is no other node with `True` ready condition.
	// [Pod Endpoint] if fail-open enabled, then containerRead pods on nodes that have
	// `Unknown` ready condition will be included if there is no other pods that are ready.
	failOpenEnabled bool
}

func (r *defaultEndpointResolver) ResolvePodEndpoints(
	ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
	opts ...EndpointResolveOption,
) ([]EndpointAddress, error) {
	logger := contexts.NewContext(ctx).Log()

	_, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, err
	}

	endpoints := &corev1.Endpoints{}
	if err := r.k8sClient.Get(ctx, svcKey, endpoints); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
		}
		return nil, err
	}

	var podEndpoints []EndpointAddress
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != PodKind {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name {
					podEndpoints = append(podEndpoints, EndpointAddress{
						IP:   addr.IP,
						Port: int(port.Port),
						Name: addr.TargetRef.Name,
					})
				}
			}
		}

		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != PodKind {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name {
					podEndpoints = append(podEndpoints, EndpointAddress{
						IP:   addr.IP,
						Port: int(port.Port),
						Name: addr.TargetRef.Name,
					})
				}
			}
		}
	}

	logger.Debugf("resolved %d pod endpoints for service %s", len(podEndpoints), svcKey)

	return podEndpoints, nil
}

func (r *defaultEndpointResolver) ResolveNodePortEndpoints(
	ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
	opts ...EndpointResolveOption,
) ([]EndpointAddress, error) {

	logger := contexts.NewContext(ctx).Log()

	resolveOpts := defaultEndpointResolveOptions()
	resolveOpts.ApplyOptions(opts)

	svc, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, err
	}
	if svc.Spec.Type != corev1.ServiceTypeNodePort && svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil, errors.Errorf("service type must be either 'NodePort' or 'LoadBalancer': %v", svcKey)
	}
	svcNodePort := svcPort.NodePort
	nodeList := &corev1.NodeList{}
	if err := r.k8sClient.List(ctx, nodeList,
		client.MatchingLabelsSelector{Selector: resolveOpts.NodeSelector}); err != nil {
		return nil, err
	}

	logger.Debugf("found %d nodes with selector: %v.", len(nodeList.Items), resolveOpts.NodeSelector)

	candidateNodes := make([]*corev1.Node, 0)
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		candidateNodes = append(candidateNodes, node)
	}

	targetNodes := filterNodesByReadyConditionStatus(candidateNodes, corev1.ConditionTrue)
	if r.failOpenEnabled && len(targetNodes) == 0 {
		targetNodes = filterNodesByReadyConditionStatus(candidateNodes, corev1.ConditionUnknown)
	}

	logger.Debugf("found %d nodes after filtering by ready condition.", len(targetNodes))

	endpoints := make([]EndpointAddress, 0)
	for _, node := range targetNodes {
		nodeIP, err := r.getNodeInternalIP(node)
		if err != nil {
			continue
		}
		endpoints = append(endpoints, r.buildNodePortEndpoint(nodeIP, node.Name, svcNodePort))
	}

	logger.Debugf("resolved %d nodeport endpoints for service %s", len(endpoints), svcKey)

	return endpoints, nil
}

func (r *defaultEndpointResolver) findServiceAndServicePort(
	ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
) (*corev1.Service, corev1.ServicePort, error) {
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, svcKey, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
		}
		return nil, corev1.ServicePort{}, err
	}

	svcPort, err := r.lookupServicePort(svc, port)
	if err != nil {
		return nil, corev1.ServicePort{}, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
	}

	return svc, svcPort, nil
}

// lookupServicePort returns the ServicePort structure for specific port on service.
func (r *defaultEndpointResolver) lookupServicePort(
	svc *corev1.Service, port intstr.IntOrString,
) (corev1.ServicePort, error) {
	if port.Type == intstr.String {
		for _, p := range svc.Spec.Ports {
			if p.Name == port.StrVal {
				return p, nil
			}
		}
	} else {
		for _, p := range svc.Spec.Ports {
			if p.Port == port.IntVal {
				return p, nil
			}
		}
	}

	return corev1.ServicePort{}, errors.Errorf(
		"unable to find port %s on service %s", port.String(), k8s.NamespacedName(svc),
	)
}

func (r *defaultEndpointResolver) buildNodePortEndpoint(IP, instanceID string, nodePort int32) EndpointAddress {
	return EndpointAddress{
		Name: instanceID,
		Port: int(nodePort),
		IP:   IP,
	}
}

func (r *defaultEndpointResolver) getNodeInternalIP(node *corev1.Node) (string, error) {
	addrs := node.Status.Addresses
	if len(addrs) == 0 {
		return "", ErrNodeDoesNotHaveInternalAddress
	}

	for _, addr := range addrs {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return "", ErrNodeDoesNotHaveInternalAddress
}

func (r *defaultEndpointResolver) GetListTargetPort(
	ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString,
) ([]int, error) {
	logger := contexts.NewContext(ctx).Log()

	_, svcPort, err := r.findServiceAndServicePort(ctx, svcKey, port)
	if err != nil {
		return nil, err
	}

	endpoints := &corev1.Endpoints{}
	if err := r.k8sClient.Get(ctx, svcKey, endpoints); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err.Error())
		}
		return nil, err
	}

	var ports []int
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != PodKind {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name && !slices.Contains(ports, int(port.Port)) {
					ports = append(ports, int(port.Port))
				}
			}
		}

		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != PodKind {
				continue
			}
			for _, port := range subset.Ports {
				if port.Name == svcPort.Name && !slices.Contains(ports, int(port.Port)) {
					ports = append(ports, int(port.Port))
				}
			}
		}
	}

	logger.Debugf("found %d target ports for service %s: %v", len(ports), svcKey, ports)
	return ports, nil
}

// ----------------------------------------------------------------------------

// options for Endpoints resolve APIs
type EndpointResolveOptions struct {
	// [NodePort Endpoint] only nodes that are matched by nodeSelector will be included.
	// By default, no node will be selected.
	NodeSelector labels.Selector

	// [Pod Endpoint] if pod readinessGates is defined, then pods from unready addresses
	// with any of these readinessGates and containersReady condition will be included.
	// By default, no readinessGate is specified.
	PodReadinessGates []corev1.PodConditionType
}

func (opts *EndpointResolveOptions) ApplyOptions(options []EndpointResolveOption) {
	for _, option := range options {
		option(opts)
	}
}

type EndpointResolveOption func(opts *EndpointResolveOptions)

// WithNodeSelector is a option that sets nodeSelector.
func WithNodeSelector(nodeSelector labels.Selector) EndpointResolveOption {
	return func(opts *EndpointResolveOptions) {
		opts.NodeSelector = nodeSelector
	}
}

// WithPodReadinessGate is a option that appends podReadinessGate into EndpointResolveOptions.
func WithPodReadinessGate(cond corev1.PodConditionType) EndpointResolveOption {
	return func(opts *EndpointResolveOptions) {
		opts.PodReadinessGates = append(opts.PodReadinessGates, cond)
	}
}

// defaultEndpointResolveOptions returns the default value for EndpointResolveOptions.
func defaultEndpointResolveOptions() EndpointResolveOptions {
	return EndpointResolveOptions{
		NodeSelector:      labels.Nothing(),
		PodReadinessGates: nil,
	}
}

// ----------------------------------------------------------------------------

// filterNodesByReadyConditionStatus will filter out nodes that matches specified ready condition status
func filterNodesByReadyConditionStatus(nodes []*corev1.Node, readyCondStatus corev1.ConditionStatus) []*corev1.Node {
	var nodesWithMatchingReadyStatus []*corev1.Node
	for _, node := range nodes {
		if readyCond := GetNodeCondition(node, corev1.NodeReady); readyCond != nil && readyCond.Status == readyCondStatus {
			nodesWithMatchingReadyStatus = append(nodesWithMatchingReadyStatus, node)
		}
	}
	return nodesWithMatchingReadyStatus
}

// GetNodeCondition will get pointer to Node's existing condition.
// returns nil if no matching condition found.
func GetNodeCondition(node *corev1.Node, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == conditionType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}
