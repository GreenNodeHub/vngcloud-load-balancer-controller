package builder

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

type BuilderType string

const (
	BuilderTypeLoadService BuilderType = "service"
	BuilderTypeLoadIngress BuilderType = "ingress"
)

type AnnotationConfig struct {
	builderType      BuilderType
	annotationParser annotations.Parser
	logger           *logrus.Entry

	basicInfoHelper
	// annotation configuration
	isIgnored                  bool
	idleTimeoutClient          int
	idleTimeoutMember          int
	idleTimeoutConnection      int
	inboundCIDRs               []string
	healthcheckProtocol        loadbalancerv2.HealthCheckProtocol
	healthcheckPath            string
	successCodes               string
	healthcheckHttpVersion     loadbalancerv2.HealthCheckHttpVersion
	healthcheckHttpDomainName  string
	healthyThresholdCount      int
	unhealthyThresholdCount    int
	poolAlgorithm              loadbalancerv2.PoolAlgorithm
	targetNodeLabels           map[string]string
	healthcheckPort            int
	healthcheckHttpMethod      loadbalancerv2.HealthCheckMethod
	healthcheckTimeoutSeconds  int
	healthcheckIntervalSeconds int
	enableAutoscale            bool
	targetType                 TargetType
	isPOC                      bool

	// annotation configuration for L4 only
	enableProxyProtocol []string

	// annotation configuration for L7 only
	enableStickySession           bool
	enableTLSEncryption           bool
	certificateIDs                []string
	implementationSpecificConfigs []implementationSpecificConfig
	insertHeaders                 insertHeadersConfig
	clientCertificateID           string
	certBuilders                  []*certificateBuilderType
	autoReorderPolicies           bool

	// if user pass the security group annotation, don't create any security group, just use the given security group
	isAutoCreateSecurityGroup bool
	securityGroups            []string
	secGroupRuleBuilders      []*secGroupRuleBuilderType

	preferZoneID   common.Zone
	preferSubnetID string

	ZoneID         common.Zone
	SubnetID       string
	SubnetCIDR     string
	AllSubnetCIDRs []string // all subnet CIDRs of this cluster
}

func NewAnnotationConfig(
	ctx context.Context,
	annotationParser annotations.Parser,
	annos map[string]string,
	builderType BuilderType,
) (*AnnotationConfig, error) {
	config := &AnnotationConfig{
		builderType:      builderType,
		annotationParser: annotationParser,
		logger:           contexts.NewContext(ctx).Log(),

		basicInfoHelper: basicInfoHelper{
			loadBalancerID:   "",
			loadBalancerName: "",
			loadBalancerType: func(builderType BuilderType) loadbalancerv2.LoadBalancerType {
				switch builderType {
				case BuilderTypeLoadService:
					return loadbalancerv2.LoadBalancerTypeLayer4
				default:
					return loadbalancerv2.LoadBalancerTypeLayer7
				}
			}(builderType),

			packageID: "",
			scheme:    loadbalancerv2.InternetLoadBalancerScheme,
			tags:      map[string]string{},
		},

		isIgnored: false,

		idleTimeoutClient:          50,
		idleTimeoutMember:          50,
		idleTimeoutConnection:      5,
		inboundCIDRs:               []string{"0.0.0.0/0"},
		healthcheckProtocol:        loadbalancerv2.HealthCheckProtocolTCP,
		healthcheckHttpMethod:      loadbalancerv2.HealthCheckMethodGET,
		healthcheckPath:            "/",
		successCodes:               "200",
		healthcheckHttpVersion:     loadbalancerv2.HealthCheckHttpVersionHttp1,
		healthcheckHttpDomainName:  "",
		poolAlgorithm:              loadbalancerv2.PoolAlgorithmRoundRobin,
		healthyThresholdCount:      3,
		unhealthyThresholdCount:    3,
		healthcheckTimeoutSeconds:  5,
		healthcheckIntervalSeconds: 30,
		healthcheckPort:            0,
		targetNodeLabels:           map[string]string{},
		securityGroups:             []string{},
		enableProxyProtocol:        []string{},
		enableAutoscale:            false,
		targetType:                 TargetTypeInstance,
		enableStickySession:        false,
		enableTLSEncryption:        false,
		certificateIDs:             []string{},
		autoReorderPolicies:        false,

		isAutoCreateSecurityGroup:     true,
		isPOC:                         false,
		implementationSpecificConfigs: make([]implementationSpecificConfig, 0),
		insertHeaders: insertHeadersConfig{
			Http: map[string]string{
				"X-Forwarded-For":   "true",
				"X-Forwarded-Proto": "true",
				"X-Forwarded-Port":  "true",
			},
			Https: map[string]string{
				"X-Forwarded-For":   "true",
				"X-Forwarded-Proto": "true",
				"X-Forwarded-Port":  "true",
			},
		},
		clientCertificateID: "",
		certBuilders:        make([]*certificateBuilderType, 0),

		preferZoneID:   "",
		preferSubnetID: "",

		ZoneID:     "",
		SubnetID:   "",
		SubnetCIDR: "",
	}

	config.parseAnnotation(annos)
	return config, nil
}

func (l *AnnotationConfig) parseAnnotation(annos map[string]string) {
	if l.annotationParser == nil {
		l.logger.Warn("annotationParser is nil")
		return
	}

	l.loadBalancerName = l.parseAnnotationLoadBalancerName(annos)
	l.packageID = l.parseAnnotationPackageID(annos)
	l.scheme = l.parseAnnotationScheme(annos)
	l.targetType = l.parseAnnotationTargetType(annos)
	l.loadBalancerID = l.parseAnnotationLoadBalancerID(annos)
	l.isIgnored = l.parseAnnotationIgnore(annos)
	l.idleTimeoutClient = l.parseAnnotationIdleTimeoutClient(annos)
	l.idleTimeoutMember = l.parseAnnotationIdleTimeoutMember(annos)
	l.idleTimeoutConnection = l.parseAnnotationIdleTimeoutConnection(annos)
	l.inboundCIDRs = l.parseAnnotationInboundCIDRs(annos)
	l.healthcheckProtocol = l.parseAnnotationHealthcheckProtocol(annos)
	l.healthcheckPath = l.parseAnnotationHealthcheckPath(annos)
	l.successCodes = l.parseAnnotationSuccessCodes(annos)
	l.healthcheckHttpVersion = l.parseAnnotationHealthcheckHttpVersion(annos)
	l.healthcheckHttpDomainName = l.parseAnnotationHealthcheckHttpDomainName(annos)
	l.healthyThresholdCount = l.parseAnnotationHealthyThresholdCount(annos)
	l.unhealthyThresholdCount = l.parseAnnotationUnhealthyThresholdCount(annos)
	l.poolAlgorithm = l.parseAnnotationPoolAlgorithm(annos)
	l.enableAutoscale = l.parseAnnotationEnableAutoscale(annos)
	l.tags = l.parseAnnotationTags(annos)
	l.targetNodeLabels = l.parseAnnotationTargetNodeLabels(annos)
	l.securityGroups = l.parseAnnotationSecurityGroups(annos)
	l.healthcheckPort = l.parseAnnotationHealthcheckPort(annos)
	l.enableProxyProtocol = l.parseAnnotationEnableProxyProtocol(annos)
	l.enableStickySession = l.parseAnnotationEnableStickySession(annos)
	l.enableTLSEncryption = l.parseAnnotationEnableTLSEncryption(annos)
	l.certificateIDs = l.parseAnnotationCertificateIDs(annos)
	l.healthcheckHttpMethod = l.parseAnnotationHealthcheckHttpMethod(annos)
	l.healthcheckTimeoutSeconds = l.parseAnnotationHealthcheckTimeoutSeconds(annos)
	l.healthcheckIntervalSeconds = l.parseAnnotationHealthcheckIntervalSeconds(annos)
	l.isPOC = l.parseAnnotationIsPOC_old(annos) // isPOC2 is deprecated
	l.isPOC = l.parseAnnotationIsPOC(annos)
	l.implementationSpecificConfigs = l.parseAnnotationImplementationSpecificConfigs(annos)
	l.insertHeaders = l.parseAnnotationInsertHeaders(annos)
	l.clientCertificateID = l.parseAnnotationClientCertificateID(annos)
	l.autoReorderPolicies = l.parseAnnotationAutoReorderPolicies(annos)
	l.preferZoneID = l.parseAnnotationPreferZoneID(annos)
	l.preferSubnetID = l.parseAnnotationPreferSubnetID(annos)
}

func (l *AnnotationConfig) parseAnnotationTargetType(annos map[string]string) TargetType {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixTargetType, &option, annos)
	if !exist {
		return l.targetType
	}
	switch option {
	case string(TargetTypeIP), string(TargetTypeInstance):
		return TargetType(option)
	default:
		if exist {
			l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"", annotations.SuffixTargetType, TargetTypeInstance, TargetTypeIP)
		}
	}
	return TargetTypeInstance
}

func (l *AnnotationConfig) parseAnnotationLoadBalancerName(annos map[string]string) string {
	option := l.GetLoadBalancerName()
	l.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerName, &option, annos)
	return option
}

func (l *AnnotationConfig) parseAnnotationPackageID(annos map[string]string) string {
	option := l.packageID
	l.annotationParser.ParseStringAnnotation(annotations.SuffixPackageID, &option, annos)
	return option
}

func (l *AnnotationConfig) parseAnnotationScheme(annos map[string]string) loadbalancerv2.LoadBalancerScheme {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixScheme, &option, annos)
	switch option {
	case "internet-facing":
		return loadbalancerv2.InternetLoadBalancerScheme
	case "internal":
		return loadbalancerv2.InternalLoadBalancerScheme
	default:
		if exist {
			l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"",
				annotations.SuffixScheme, "internet-facing", "internal")
		}
	}
	return loadbalancerv2.InternetLoadBalancerScheme
}

func (l *AnnotationConfig) parseAnnotationLoadBalancerID(annos map[string]string) string {
	option := ""
	l.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &option, annos)
	return option
}

func (l *AnnotationConfig) parseAnnotationIgnore(annos map[string]string) bool {
	option := false
	l.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &option, annos)
	return option
}

func (l *AnnotationConfig) parseAnnotationIdleTimeoutClient(annos map[string]string) int {
	optionsInt64 := int64(l.idleTimeoutClient)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutClient, &optionsInt64, annos)
	if !exists {
		return l.idleTimeoutClient
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutClient, l.idleTimeoutClient)
		return l.idleTimeoutClient
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationIdleTimeoutMember(annos map[string]string) int {
	optionsInt64 := int64(l.idleTimeoutMember)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutMember, &optionsInt64, annos)
	if !exists {
		return l.idleTimeoutMember
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutMember, l.idleTimeoutMember)
		return l.idleTimeoutMember
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationIdleTimeoutConnection(annos map[string]string) int {
	optionsInt64 := int64(l.idleTimeoutConnection)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutConnection, &optionsInt64, annos)
	if !exists {
		return l.idleTimeoutConnection
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutConnection, l.idleTimeoutConnection)
		return l.idleTimeoutConnection
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationInboundCIDRs(annos map[string]string) []string {
	option := l.inboundCIDRs
	exist := l.annotationParser.ParseStringSliceAnnotation(annotations.SuffixInboundCIDRs, &option, annos)
	if !exist {
		return l.inboundCIDRs
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationHealthcheckProtocol(annos map[string]string) loadbalancerv2.HealthCheckProtocol {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckProtocol, &option, annos)
	switch option {
	case
		string(loadbalancerv2.HealthCheckProtocolHTTP),
		string(loadbalancerv2.HealthCheckProtocolHTTPs),
		string(loadbalancerv2.HealthCheckProtocolTCP),
		string(loadbalancerv2.HealthCheckProtocolPINGUDP):
		return loadbalancerv2.HealthCheckProtocol(option)
	default:
		if exist {
			l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\", \"%s\" or \"%s\"",
				annotations.SuffixHealthcheckProtocol,
				loadbalancerv2.HealthCheckProtocolHTTP,
				loadbalancerv2.HealthCheckProtocolHTTPs,
				loadbalancerv2.HealthCheckProtocolTCP,
				loadbalancerv2.HealthCheckProtocolPINGUDP)
		}
	}
	return l.healthcheckProtocol
}

func (l *AnnotationConfig) parseAnnotationHealthcheckPath(annos map[string]string) string {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckPath, &option, annos)
	if !exist {
		return l.healthcheckPath
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationSuccessCodes(annos map[string]string) string {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixSuccessCodes, &option, annos)
	if !exist {
		return l.successCodes
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationHealthcheckHttpVersion(annos map[string]string) loadbalancerv2.HealthCheckHttpVersion {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpVersion, &option, annos)
	if !exist {
		return l.healthcheckHttpVersion
	}
	switch option {
	case string(loadbalancerv2.HealthCheckHttpVersionHttp1),
		string(loadbalancerv2.HealthCheckHttpVersionHttp1Minor1):
		return loadbalancerv2.HealthCheckHttpVersion(option)
	default:
		l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"",
			annotations.SuffixHealthcheckHttpVersion,
			loadbalancerv2.HealthCheckHttpVersionHttp1,
			loadbalancerv2.HealthCheckHttpVersionHttp1Minor1)
	}
	return l.healthcheckHttpVersion
}

func (l *AnnotationConfig) parseAnnotationHealthcheckHttpDomainName(annos map[string]string) string {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpDomainName, &option, annos)
	if !exist {
		return l.healthcheckHttpDomainName
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationHealthyThresholdCount(annos map[string]string) int {
	optionsInt64 := int64(l.healthyThresholdCount)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixHealthyThresholdCount, &optionsInt64, annos)
	if !exists {
		return l.healthyThresholdCount
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixHealthyThresholdCount, l.healthyThresholdCount)
		return l.healthyThresholdCount
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationUnhealthyThresholdCount(annos map[string]string) int {
	optionsInt64 := int64(l.unhealthyThresholdCount)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixUnhealthyThresholdCount, &optionsInt64, annos)
	if !exists {
		return l.unhealthyThresholdCount
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixUnhealthyThresholdCount, l.unhealthyThresholdCount)
		return l.unhealthyThresholdCount
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationPoolAlgorithm(annos map[string]string) loadbalancerv2.PoolAlgorithm {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixPoolAlgorithm, &option, annos)
	if !exist {
		return l.poolAlgorithm
	}
	switch option {
	case string(loadbalancerv2.PoolAlgorithmLeastConn),
		string(loadbalancerv2.PoolAlgorithmRoundRobin),
		string(loadbalancerv2.PoolAlgorithmSourceIP):
		return loadbalancerv2.PoolAlgorithm(option)
	default:
		l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixPoolAlgorithm,
			loadbalancerv2.PoolAlgorithmLeastConn,
			loadbalancerv2.PoolAlgorithmRoundRobin,
			loadbalancerv2.PoolAlgorithmSourceIP)
	}
	return l.poolAlgorithm
}

func (l *AnnotationConfig) parseAnnotationEnableAutoscale(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableAutoscale, &option, annos)
	if !exists {
		return l.enableAutoscale
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixEnableAutoscale, l.enableAutoscale)
		return l.enableAutoscale
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationTags(annos map[string]string) map[string]string {
	option := make(map[string]string)
	exist, err := l.annotationParser.ParseStringMapAnnotation(annotations.SuffixTags, &option, annos)
	if !exist {
		return l.tags
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTags, l.tags)
		return l.tags
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationTargetNodeLabels(annos map[string]string) map[string]string {
	option := l.targetNodeLabels
	exist, err := l.annotationParser.ParseStringMapAnnotation(annotations.SuffixTargetNodeLabels, &option, annos)
	if !exist {
		return l.targetNodeLabels
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTargetNodeLabels, l.targetNodeLabels)
		return l.targetNodeLabels
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationSecurityGroups(annos map[string]string) []string {
	option := l.securityGroups
	exist := l.annotationParser.ParseStringSliceAnnotation(annotations.SuffixSecurityGroups, &option, annos)
	if !exist {
		return l.securityGroups
	}
	l.isAutoCreateSecurityGroup = false
	return option
}

func (l *AnnotationConfig) parseAnnotationHealthcheckPort(annos map[string]string) int {
	optionsInt64 := int64(l.healthcheckPort)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckPort, &optionsInt64, annos)
	if !exists {
		return l.healthcheckPort
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixHealthcheckPort, l.healthcheckPort)
		return l.healthcheckPort
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationEnableProxyProtocol(annos map[string]string) []string {
	option := l.enableProxyProtocol
	exist := l.annotationParser.ParseStringSliceAnnotation(annotations.SuffixEnableProxyProtocol, &option, annos)
	if !exist {
		return l.enableProxyProtocol
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationEnableStickySession(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableStickySession, &option, annos)
	if !exists {
		return l.enableStickySession
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixEnableStickySession, l.enableStickySession)
		return l.enableStickySession
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationEnableTLSEncryption(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableTLSEncryption, &option, annos)
	if !exists {
		return l.enableTLSEncryption
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixEnableTLSEncryption, l.enableTLSEncryption)
		return l.enableTLSEncryption
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationCertificateIDs(annos map[string]string) []string {
	option := l.certificateIDs
	exist := l.annotationParser.ParseStringSliceAnnotation(annotations.SuffixCertificateIDs, &option, annos)
	if !exist {
		return l.certificateIDs
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationHealthcheckHttpMethod(annos map[string]string) loadbalancerv2.HealthCheckMethod {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpMethod, &option, annos)
	if !exist {
		return l.healthcheckHttpMethod
	}
	switch option {
	case
		string(loadbalancerv2.HealthCheckMethodGET),
		string(loadbalancerv2.HealthCheckMethodPUT),
		string(loadbalancerv2.HealthCheckMethodPOST):
		return loadbalancerv2.HealthCheckMethod(option)
	default:
		l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixHealthcheckHttpMethod,
			loadbalancerv2.HealthCheckMethodGET,
			loadbalancerv2.HealthCheckMethodPUT,
			loadbalancerv2.HealthCheckMethodPOST)
		return l.healthcheckHttpMethod
	}
}

func (l *AnnotationConfig) parseAnnotationHealthcheckTimeoutSeconds(annos map[string]string) int {
	optionsInt64 := int64(l.healthcheckTimeoutSeconds)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckTimeoutSeconds, &optionsInt64, annos)
	if !exists {
		return l.healthcheckTimeoutSeconds
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixHealthcheckTimeoutSeconds, l.healthcheckTimeoutSeconds)
		return l.healthcheckTimeoutSeconds
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationHealthcheckIntervalSeconds(annos map[string]string) int {
	optionsInt64 := int64(l.healthcheckIntervalSeconds)
	exists, err := l.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckIntervalSeconds, &optionsInt64, annos)
	if !exists {
		return l.healthcheckIntervalSeconds
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixHealthcheckIntervalSeconds, l.healthcheckIntervalSeconds)
		return l.healthcheckIntervalSeconds
	}
	return int(optionsInt64)
}

func (l *AnnotationConfig) parseAnnotationIsPOC(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC, &option, annos)
	if !exists {
		return l.isPOC
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixIsPOC, l.isPOC)
		return l.isPOC
	}
	return option
}
func (l *AnnotationConfig) parseAnnotationIsPOC_old(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC2, &option, annos)
	if !exists {
		return l.isPOC
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixIsPOC, l.isPOC)
		return l.isPOC
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationImplementationSpecificConfigs(annos map[string]string) []implementationSpecificConfig {
	option := make([]implementationSpecificConfig, 0)
	exist, err := l.annotationParser.ParseJSONAnnotation(annotations.SuffixImplementationSpecificParams, &option, annos)
	if !exist {
		return l.implementationSpecificConfigs
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a JSON object, using default value %v",
			annotations.SuffixImplementationSpecificParams, l.implementationSpecificConfigs)
		return l.implementationSpecificConfigs
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationInsertHeaders(annos map[string]string) insertHeadersConfig {
	option := insertHeadersConfig{}
	exist, err := l.annotationParser.ParseJSONAnnotation(annotations.SuffixInsertHeaders, &option, annos)
	if !exist {
		return l.insertHeaders
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a JSON object, using default value %v",
			annotations.SuffixInsertHeaders, l.insertHeaders)
		return l.insertHeaders
	}
	if option.Http == nil {
		option.Http = make(map[string]string)
	}
	if option.Https == nil {
		option.Https = make(map[string]string)
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationClientCertificateID(annos map[string]string) string {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixClientCertificateID, &option, annos)
	if !exist {
		return l.clientCertificateID
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationAutoReorderPolicies(annos map[string]string) bool {
	option := false
	exists, err := l.annotationParser.ParseBoolAnnotation(annotations.SuffixAutoReorderPolicies, &option, annos)
	if !exists {
		return l.autoReorderPolicies
	}
	if err != nil {
		l.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean, using default value %t",
			annotations.SuffixAutoReorderPolicies, l.autoReorderPolicies)
		return l.autoReorderPolicies
	}
	return option
}

func (l *AnnotationConfig) parseAnnotationPreferZoneID(annos map[string]string) common.Zone {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixPreferZoneID, &option, annos)
	if !exist {
		return l.preferZoneID
	}
	switch option {
	case string(common.HCM_03_1A_ZONE),
		string(common.HCM_03_1B_ZONE),
		string(common.HCM_03_1C_ZONE),
		string(common.HCM_03_BKK_01_ZONE),
		string(common.HAN_01_1A_ZONE):
		return common.Zone(option)
	default:
		l.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\", \"%s\", \"%s\" or \"%s\".",
			annotations.SuffixPreferZoneID,
			common.HCM_03_1A_ZONE,
			common.HCM_03_1B_ZONE,
			common.HCM_03_1C_ZONE,
			common.HCM_03_BKK_01_ZONE,
			common.HAN_01_1A_ZONE,
		)
	}
	return l.preferZoneID
}

func (l *AnnotationConfig) parseAnnotationPreferSubnetID(annos map[string]string) string {
	option := ""
	exist := l.annotationParser.ParseStringAnnotation(annotations.SuffixPreferSubnetID, &option, annos)
	if !exist {
		return l.preferSubnetID
	}
	return option
}

// func (l *AnnotationConfig) parseAnnotation

func (l *AnnotationConfig) GetPreferZoneID() common.Zone {
	return l.preferZoneID
}

func (l *AnnotationConfig) GetPreferSubnetID() string {
	return l.preferSubnetID
}

func (l *AnnotationConfig) IsMetadataValid() bool {
	return l.ZoneID != "" && l.SubnetID != "" && l.SubnetCIDR != ""
}

func (l *AnnotationConfig) IsIgnored() bool {
	return l.isIgnored
}
