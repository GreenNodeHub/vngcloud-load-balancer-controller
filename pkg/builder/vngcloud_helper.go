package builder

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	corev1 "k8s.io/api/core/v1"
)

// providerID
const (
	// Define the regular expression pattern
	patternPrefix = `vngcloud:\/\/`
	rawPrefix     = `vngcloud://`
	pattern       = "^" + patternPrefix + "ins-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

var (
	vngCloudProviderIDRegex = regexp.MustCompile(pattern)
)

func matchCloudProviderPattern(pproviderID string) bool {
	return vngCloudProviderIDRegex.MatchString(pproviderID)
}

func getProviderID(pnode *corev1.Node) string {
	return pnode.Spec.ProviderID[len(rawPrefix):len(pnode.Spec.ProviderID)]
}

type helperStruct struct{}

var VNGHelper = &helperStruct{}

// GetListProviderID returns the list of provider IDs.
func (h *helperStruct) GetListProviderID(pnodes []*corev1.Node) []string {
	var providerIDs []string
	for _, node := range pnodes {
		if node != nil && (matchCloudProviderPattern(node.Spec.ProviderID)) {
			providerIDs = append(providerIDs, getProviderID(node))
		}
	}

	return providerIDs
}

// MergeTags try to keep the current tags, add new tags, return the final tags.
// Ensure tag contains loadbalancer id.
// If nil is returned, the tags will not be updated.
func (h *helperStruct) MergeTags(ctx context.Context, current, new ModelBuilder) map[string]string {
	logger := contexts.NewContext(ctx).Log()
	currentTags, newTags, mergeTags := make(map[string]string), make(map[string]string), make(map[string]string)
	isNeedUpdate := false
	if current != nil {
		currentTags = current.GetTags()
	}
	if new != nil {
		newTags = new.GetTags()
	}

	for key, value := range currentTags {
		mergeTags[key] = value
	}
	for key, value := range newTags {
		if mergeTags[key] != value {
			isNeedUpdate = true
			mergeTags[key] = value
		}
	}

	// ensure tag contains loadbalancer id
	idValidClusterID := func(_ string) bool {
		return true
	}

	joinVKSTags := func(currentValue, id string) string {
		tags := strings.Split(currentValue, consts.VKS_TAGS_SEPARATOR)
		tagsValid := make(map[string]bool)
		for _, tag := range tags {
			if idValidClusterID(tag) {
				tagsValid[tag] = true
			}
		}
		if idValidClusterID(id) {
			tagsValid[id] = true
		}
		newTags := make([]string, 0)
		for tag := range tagsValid {
			newTags = append(newTags, tag)
		}
		return strings.Join(newTags, consts.VKS_TAGS_SEPARATOR)
	}

	vksClusterValue := joinVKSTags(currentTags[consts.VKS_TAG_KEY], new.GetLoadBalancerID())
	if vksClusterValue != currentTags[consts.VKS_TAG_KEY] {
		isNeedUpdate = true
		mergeTags[consts.VKS_TAG_KEY] = vksClusterValue
	}

	if !isNeedUpdate {
		logger.Info("No need to update tags")
		return nil
	}
	return mergeTags
}

// ParseListenerProtocol parse listener protocol to listener protocol
func (h *helperStruct) ParseListenerProtocol(pPort corev1.ServicePort) loadbalancerv2.ListenerProtocol {
	opt := strings.TrimSpace(strings.ToUpper(string(pPort.Protocol)))
	switch opt {
	case string(loadbalancerv2.ListenerProtocolUDP):
		return loadbalancerv2.ListenerProtocolUDP
	}

	return loadbalancerv2.ListenerProtocolTCP
}

// ParseMonitorProtocol parse monitor protocol to health check protocol
func (h *helperStruct) ParseHealthCheckProtocol(pPoolProtocol corev1.Protocol, pMonitorProtocol string) loadbalancerv2.HealthCheckProtocol {
	switch pPoolProtocol {
	case corev1.ProtocolUDP:
		return loadbalancerv2.HealthCheckProtocolPINGUDP
	}

	switch strings.TrimSpace(strings.ToUpper(pMonitorProtocol)) {
	case string(loadbalancerv2.HealthCheckProtocolHTTP):
		return loadbalancerv2.HealthCheckProtocolHTTP
	case string(loadbalancerv2.HealthCheckProtocolHTTPs):
		return loadbalancerv2.HealthCheckProtocolHTTPs
	case string(loadbalancerv2.HealthCheckProtocolPINGUDP):
		return loadbalancerv2.HealthCheckProtocolPINGUDP
	}

	return loadbalancerv2.HealthCheckProtocolTCP
}

// ParsePoolProtocol parse string to pool protocol
func (h *helperStruct) ParsePoolProtocol(pPoolProtocol string) loadbalancerv2.PoolProtocol {
	opt := strings.TrimSpace(strings.ToUpper(pPoolProtocol))
	switch opt {
	case string(loadbalancerv2.PoolProtocolProxy):
		return loadbalancerv2.PoolProtocolProxy
	case string(loadbalancerv2.PoolProtocolHTTP):
		return loadbalancerv2.PoolProtocolHTTP
	case string(loadbalancerv2.PoolProtocolUDP):
		return loadbalancerv2.PoolProtocolUDP
	}
	return loadbalancerv2.PoolProtocolTCP
}

// ComparePoolBuilder compares two pools.
func (h *helperStruct) ComparePoolBuilder(lbID string, current, new *poolBuilderType) (*loadbalancerv2.UpdatePoolRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	healthMonitor := &loadbalancerv2.HealthMonitor{
		HealthyThreshold:    new.HealthMonitor.HealthyThreshold,
		UnhealthyThreshold:  new.HealthMonitor.UnhealthyThreshold,
		Interval:            new.HealthMonitor.Interval,
		Timeout:             new.HealthMonitor.Timeout,
		HealthCheckProtocol: new.HealthMonitor.HealthCheckProtocol,
		HealthCheckMethod:   new.HealthMonitor.HealthCheckMethod,
		HttpVersion:         new.HealthMonitor.HttpVersion,
		HealthCheckPath:     new.HealthMonitor.HealthCheckPath,
		DomainName:          new.HealthMonitor.DomainName,
		SuccessCode:         new.HealthMonitor.SuccessCode,
	}
	updateOptions := &loadbalancerv2.UpdatePoolRequest{
		PoolCommon: common.PoolCommon{
			PoolId: current.GetID(),
		},
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		Algorithm:     new.Algorithm,
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: healthMonitor,
	}
	if !new.IsL4 {
		updateOptions.Stickiness = &new.Stickiness
		updateOptions.TLSEncryption = &new.TLSEncryption
	}
	if current.Algorithm != new.Algorithm {
		message = append(message, fmt.Sprintf("algorithm (%s -> %s)", current.Algorithm, new.Algorithm))
		isNeedUpdate = true
	}
	if !new.IsL4 && current.Stickiness != new.Stickiness {
		message = append(message, fmt.Sprintf("stickiness (%t -> %t)", current.Stickiness, new.Stickiness))
		isNeedUpdate = true
	}
	if !new.IsL4 && current.TLSEncryption != new.TLSEncryption {
		message = append(message, fmt.Sprintf("tls encryption (%t -> %t)", current.TLSEncryption, new.TLSEncryption))
		isNeedUpdate = true
	}

	if current.HealthMonitor.HealthyThreshold != new.HealthMonitor.HealthyThreshold {
		message = append(message, fmt.Sprintf("healthy threshold (%d -> %d)", current.HealthMonitor.HealthyThreshold, new.HealthMonitor.HealthyThreshold))
		isNeedUpdate = true
	}
	if current.HealthMonitor.UnhealthyThreshold != new.HealthMonitor.UnhealthyThreshold {
		message = append(message, fmt.Sprintf("unhealthy threshold (%d -> %d)", current.HealthMonitor.UnhealthyThreshold, new.HealthMonitor.UnhealthyThreshold))
		isNeedUpdate = true
	}
	if current.HealthMonitor.Interval != new.HealthMonitor.Interval {
		message = append(message, fmt.Sprintf("interval (%d -> %d)", current.HealthMonitor.Interval, new.HealthMonitor.Interval))
		isNeedUpdate = true
	}
	if current.HealthMonitor.Timeout != new.HealthMonitor.Timeout {
		message = append(message, fmt.Sprintf("timeout (%d -> %d)", current.HealthMonitor.Timeout, new.HealthMonitor.Timeout))
		isNeedUpdate = true
	}

	if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {
		// domain may return nil
		if current.HealthMonitor.HealthCheckPath == nil || *current.HealthMonitor.HealthCheckPath != *new.HealthMonitor.HealthCheckPath ||
			current.HealthMonitor.DomainName == nil || *current.HealthMonitor.DomainName != *new.HealthMonitor.DomainName ||
			current.HealthMonitor.HttpVersion == nil || *current.HealthMonitor.HttpVersion != *new.HealthMonitor.HttpVersion ||
			current.HealthMonitor.HealthCheckMethod == nil || *current.HealthMonitor.HealthCheckMethod != *new.HealthMonitor.HealthCheckMethod ||
			current.HealthMonitor.SuccessCode == nil || *current.HealthMonitor.SuccessCode != *new.HealthMonitor.SuccessCode {
			isNeedUpdate = true
		}
	} else if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolTCP {

		healthMonitor.HealthCheckProtocol = loadbalancerv2.HealthCheckProtocolHTTP
		healthMonitor.HealthCheckPath = current.HealthMonitor.HealthCheckPath
		healthMonitor.DomainName = current.HealthMonitor.DomainName
		healthMonitor.HttpVersion = current.HealthMonitor.HttpVersion
		healthMonitor.HealthCheckMethod = current.HealthMonitor.HealthCheckMethod
	} else if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolTCP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {

		healthMonitor.HealthCheckProtocol = loadbalancerv2.HealthCheckProtocolTCP
		healthMonitor.HealthCheckPath = nil
		healthMonitor.DomainName = nil
		healthMonitor.HttpVersion = nil
		healthMonitor.HealthCheckMethod = nil
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// checkIfPoolMemberExist checks if the pool member exists in the pool members.
func (h *helperStruct) checkIfPoolMemberExist(mems []*loadbalancerv2.Member, mem *loadbalancerv2.Member) bool {
	for _, r := range mems {
		if r.IpAddress == mem.IpAddress &&
			r.Port == mem.Port &&
			// r.Backup == mem.Backup &&
			// r.Name == mem.Name &&
			// r.Weight == mem.Weight &&
			r.MonitorPort == mem.MonitorPort {
			return true
		}
	}
	return false
}

// ComparePoolMembers compares two pool members.
// mustBeEqual is true if the two pool members must be equal, otherwise, just check if the pool members exist in the other pool members.
func (h *helperStruct) ComparePoolMembers(parentSet, childSet []*loadbalancerv2.Member, mustBeEqual bool) bool {
	if mustBeEqual && len(parentSet) != len(childSet) {
		return false
	}

	for _, m := range childSet {
		if !h.checkIfPoolMemberExist(parentSet, m) {
			return false
		}
	}
	return true
}

// MergePoolMembers merges the pool members.
func (h *helperStruct) MergePoolMembers(lbID string, oldBuilder OldModelBuilder, currentBuilder, addBuilder *poolBuilderType) (loadbalancerv2.IUpdatePoolMembersRequest, error) {
	currentSet := make([]*loadbalancerv2.Member, 0)
	deleteSet := make([]*loadbalancerv2.Member, 0)
	addSet := make([]*loadbalancerv2.Member, 0)
	if currentBuilder != nil {
		currentSet = currentBuilder.Members
	}
	if oldBuilder != nil {
		deleteSet = oldBuilder.GetDefaultPoolMembers()
	}
	if addBuilder != nil {
		addSet = addBuilder.Members
	}

	resultPoolMembers := make([]*loadbalancerv2.Member, 0)
	for _, member := range currentSet {
		if h.checkIfPoolMemberExist(addSet, member) || !h.checkIfPoolMemberExist(deleteSet, member) {
			resultPoolMembers = append(resultPoolMembers, member)
		}
	}
	for _, member := range addSet {
		if !h.checkIfPoolMemberExist(resultPoolMembers, member) {
			resultPoolMembers = append(resultPoolMembers, member)
		}
	}

	// if the pool members are equal, return nil
	if h.ComparePoolMembers(resultPoolMembers, currentSet, true) {
		return nil, nil
	}

	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range resultPoolMembers {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}

	logrus.Debugf("Merge pool members: %v", convertMembers)
	return loadbalancerv2.NewUpdatePoolMembersRequest(lbID, currentBuilder.GetID()).WithMembers(convertMembers...), nil
}

// CompareListenerBuilder compares two listener options.
func (h *helperStruct) CompareListenerBuilder(lbID string, current, new *ListenerBuilderType) (*loadbalancerv2.UpdateListenerRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdateListenerRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: current.GetID(),
		},
		AllowedCidrs:                new.AllowedCidrs,
		TimeoutClient:               new.TimeoutClient,
		TimeoutMember:               new.TimeoutMember,
		TimeoutConnection:           new.TimeoutConnection,
		DefaultPoolId:               *new.DefaultPoolId,
		DefaultCertificateAuthority: nil,
		CertificateAuthorities:      nil,
		Headers:                     nil,
		ClientCertificate:           nil,
	}

	// set current value
	if !new.IsL4 {
		updateOptions.Headers = new.Headers
		if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {
			updateOptions.ClientCertificate = new.ClientCertificate
			updateOptions.DefaultCertificateAuthority = new.DefaultCertificateAuthority
			updateOptions.CertificateAuthorities = new.CertificateAuthorities
		}
	}

	if current.AllowedCidrs != new.AllowedCidrs {
		message = append(message, fmt.Sprintf("allowed cidrs (%v -> %v)", current.AllowedCidrs, new.AllowedCidrs))
		isNeedUpdate = true
	}

	if current.TimeoutClient != new.TimeoutClient {
		message = append(message, fmt.Sprintf("timeout client (%d -> %d)", current.TimeoutClient, new.TimeoutClient))
		isNeedUpdate = true
	}

	if current.TimeoutMember != new.TimeoutMember {
		message = append(message, fmt.Sprintf("timeout member (%d -> %d)", current.TimeoutMember, new.TimeoutMember))
		isNeedUpdate = true
	}

	if current.TimeoutConnection != new.TimeoutConnection {
		message = append(message, fmt.Sprintf("timeout connection (%d -> %d)", current.TimeoutConnection, new.TimeoutConnection))
		isNeedUpdate = true
	}

	if *current.DefaultPoolId != *new.DefaultPoolId {
		message = append(message, fmt.Sprintf("default pool id (%s -> %s)", *current.DefaultPoolId, *new.DefaultPoolId))
		isNeedUpdate = true
	}

	if !new.IsL4 {

		// headers
		slices.Sort(*current.Headers)
		slices.Sort(*new.Headers)
		if !slices.Equal(*current.Headers, *new.Headers) {
			message = append(message, fmt.Sprintf("headers (%v -> %v)", *current.Headers, *new.Headers))
			isNeedUpdate = true
		}

		if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {

			// client certificate
			if !comparePointer(current.ClientCertificate, new.ClientCertificate) {
				message = append(message, fmt.Sprintf("client certificate (%v -> %v)",
					pointerToString(current.ClientCertificate), pointerToString(new.ClientCertificate)))
				isNeedUpdate = true
			}

			// default certificate authority
			if !comparePointer(current.DefaultCertificateAuthority, new.DefaultCertificateAuthority) {
				message = append(message, fmt.Sprintf("default certificate authority (%s -> %s)",
					pointerToString(current.DefaultCertificateAuthority), pointerToString(new.DefaultCertificateAuthority)))
				isNeedUpdate = true
			}

			// certificate authorities
			if (current.CertificateAuthorities == nil || new.CertificateAuthorities == nil) &&
				current.CertificateAuthorities != new.CertificateAuthorities {
				message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
				isNeedUpdate = true
			} else {
				// CertificateAuthorities is not nil
				if len(*current.CertificateAuthorities) != len(*new.CertificateAuthorities) {
					message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
					isNeedUpdate = true
				} else {
					for _, ca := range *new.CertificateAuthorities {
						if !slices.Contains(*current.CertificateAuthorities, ca) {
							message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
							isNeedUpdate = true
							break
						}
					}
				}
			}
		}
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// ComparePolicyBuilder
func (h *helperStruct) ComparePolicyBuilder(lbID, lisID string, current, new *policyBuilderType) (*loadbalancerv2.UpdatePolicyRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdatePolicyRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: lisID,
		},
		PolicyCommon: common.PolicyCommon{
			PolicyId: current.GetID(),
		},
		Action:           new.Action,
		Rules:            new.Rules,
		KeepQueryString:  new.KeepQueryString,
		RedirectPoolID:   new.RedirectPoolID,
		RedirectURL:      new.RedirectURL,
		RedirectHTTPCode: new.RedirectHTTPCode,
	}
	if current.Action != new.Action {
		message = append(message, fmt.Sprintf("action (%s -> %s)", current.Action, new.Action))
		isNeedUpdate = true
	}

	// options for redirect to pool
	if new.Action == loadbalancerv2.PolicyActionREDIRECTTOPOOL && current.RedirectPoolID != new.RedirectPoolID {
		message = append(message, fmt.Sprintf("redirect pool id (%s -> %s)", current.RedirectPoolID, new.RedirectPoolID))
		isNeedUpdate = true
	}

	// options for redirect to url
	if new.Action == loadbalancerv2.PolicyActionREDIRECTTOURL {
		if current.RedirectURL != new.RedirectURL {
			message = append(message, fmt.Sprintf("redirect url (%s -> %s)", current.RedirectURL, new.RedirectURL))
			isNeedUpdate = true
		}
		if current.RedirectHTTPCode != new.RedirectHTTPCode {
			message = append(message, fmt.Sprintf("redirect http code (%d -> %d)", current.RedirectHTTPCode, new.RedirectHTTPCode))
			isNeedUpdate = true
		}
		if current.KeepQueryString != new.KeepQueryString {
			message = append(message, fmt.Sprintf("keep query string (%t -> %t)", current.KeepQueryString, new.KeepQueryString))
			isNeedUpdate = true
		}
	}

	if len(current.Rules) != len(new.Rules) {
		message = append(message, fmt.Sprintf("len(rules) (%d -> %d)", len(current.Rules), len(new.Rules)))
		isNeedUpdate = true
	} else {
		for _, rule := range new.Rules {
			if !h.checkIfL7RuleExist(current.Rules, rule) {
				message = append(message, fmt.Sprintf("rules (%v -> %v)", current.Rules, new.Rules))
				isNeedUpdate = true
				break
			}
		}
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

func (h *helperStruct) checkIfL7RuleExist(rules []loadbalancerv2.L7RuleRequest, rule loadbalancerv2.L7RuleRequest) bool {
	for _, r := range rules {
		if r.CompareType == rule.CompareType &&
			r.RuleType == rule.RuleType &&
			r.RuleValue == rule.RuleValue {
			return true
		}
	}
	return false
}

func (h *helperStruct) CompareSecgroupRule(current []*entityv2.SecgroupRule, new []*secGroupRuleBuilderType) ([]*entityv2.SecgroupRule, []*secGroupRuleBuilderType, error) {
	// get only ingress rules
	currentIngressRules := make([]*entityv2.SecgroupRule, 0)
	for _, rule := range current {
		if strings.EqualFold(rule.Direction, string(networkv2.SecgroupRuleDirectionIngress)) {
			currentIngressRules = append(currentIngressRules, rule)
		}
	}

	needDelete := make([]*entityv2.SecgroupRule, 0)
	needCreate := make([]*secGroupRuleBuilderType, 0)

	// mark all rule not in use
	ruleInUse := make(map[string]bool)
	for _, rule := range currentIngressRules {
		ruleInUse[rule.Id] = false
	}

	// check if the rule is in new
	for _, rule := range new {
		found := false
		for _, currentRule := range currentIngressRules {
			if rule.Description == currentRule.Description &&
				strings.EqualFold(string(rule.Direction), currentRule.Direction) &&
				strings.EqualFold(string(rule.EtherType), currentRule.EtherType) &&
				strings.EqualFold(string(rule.Protocol), currentRule.Protocol) &&
				rule.PortRangeMax == currentRule.PortRangeMax &&
				rule.PortRangeMin == currentRule.PortRangeMin &&
				rule.RemoteIPPrefix == currentRule.RemoteIPPrefix {

				ruleInUse[currentRule.Id] = true
				found = true
				break
			}
		}

		if !found {
			needCreate = append(needCreate, rule)
		}
	}

	// check if the rule is not in use
	for _, rule := range currentIngressRules {
		if !ruleInUse[rule.Id] {
			needDelete = append(needDelete, rule)
		}
	}

	return needDelete, needCreate, nil
}

func (h *helperStruct) MergeStringArray(ctx context.Context, current, remove, add []string) ([]string, bool) {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("  - current: %v", current)
	logger.Debugf("  - remove:  %v", remove)
	logger.Debugf("  - add:     %v", add)

	mapCurrent := make(map[string]bool)
	for _, c := range current {
		mapCurrent[c] = true
	}
	for _, r := range remove {
		delete(mapCurrent, r)
	}
	for _, a := range add {
		mapCurrent[a] = true
	}
	ret := make([]string, 0)
	for k := range mapCurrent {
		ret = append(ret, k)
	}
	if len(ret) != len(current) {
		return ret, true
	}
	for _, c := range current {
		if !mapCurrent[c] {
			return ret, true
		}
	}
	return ret, false
}

func comparePointer[T comparable](current, new *T) bool {
	if current == nil && new == nil {
		return true
	}
	if current == nil || new == nil {
		return false
	}
	return *current == *new
}

func pointerToString[T any](p *T) string {
	if p == nil {
		return "(nil)"
	}
	return fmt.Sprintf("&(%v)", *p)
}
