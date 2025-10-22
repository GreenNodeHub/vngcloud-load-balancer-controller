package utils

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

func NewNameHelper(clusterID, resourceType, resourceNamespace, resourceName string) NameHelper {
	return &nameHelper{
		clusterID:         clusterID,
		resourceType:      resourceType,
		resourceNamespace: resourceNamespace,
		resourceName:      resourceName,
	}
}

type NameHelper interface {
	// return the generated name of the load balancer
	GetLoadBalancerDefaultName() string
	GenerateHash() string
	ValidateName(newName string) string

	GenL4PoolName(pPort corev1.ServicePort, realProtocol string) string
	GenL4ListenerName(pPort corev1.ServicePort) string
}

var _ NameHelper = &nameHelper{}

type nameHelper struct {
	// resource info
	clusterID         string
	resourceType      string // service, ingress
	resourceName      string
	resourceNamespace string
}

func (l *nameHelper) GetLoadBalancerDefaultName() string {
	hash := l.GenerateHash()
	name := fmt.Sprintf("%s_%s_%s_%s_%s",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(l.clusterID, 10),
		TrimString(l.resourceNamespace, 10),
		TrimString(l.resourceName, 10),
		hash)
	return l.ValidateName(name)
}

func (l *nameHelper) GenerateHash() string {
	fullName := fmt.Sprintf("%s_%s_%s_%s", l.clusterID, l.resourceNamespace, l.resourceName, l.resourceType)
	hash := HashString(fullName)
	return TrimString(hash, consts.DEFAULT_HASH_NAME_LENGTH)
}

func (l *nameHelper) ValidateName(newName string) string {
	for _, char := range newName {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '-' && char != '.' {
			newName = strings.ReplaceAll(newName, string(char), "-")
		}
	}
	return TrimString(newName, consts.DEFAULT_PORTAL_NAME_LENGTH)
}

// genL4PoolName generates the name of the pool.
func (t *nameHelper) GenL4PoolName(pPort corev1.ServicePort, realProtocol string) string {
	hash := t.GenerateHash()
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(t.clusterID, 10),
		TrimString(t.resourceNamespace, 9),
		TrimString(t.resourceName, 9),
		hash,
		TrimString(realProtocol, 3),
		pPort.Port)
	return t.ValidateName(name)
}

// genL4ListenerName generates the name of the listener.
func (t *nameHelper) GenL4ListenerName(pPort corev1.ServicePort) string {
	hash := t.GenerateHash()
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(t.clusterID, 10),
		TrimString(t.resourceNamespace, 9),
		TrimString(t.resourceName, 9),
		hash,
		TrimString(string(pPort.Protocol), 3),
		pPort.Port)
	return t.ValidateName(name)
}

func GenerateLBConfigName(prefix, baseName string) string {
	full := fmt.Sprintf("%s-%s", prefix, baseName)
	if len(full) <= 63 {
		return full
	}

	hash := fmt.Sprintf("%x", sha1.Sum([]byte(full)))[:6] // lấy 6 ký tự hash
	cutLen := 63 - len(prefix) - len(hash) - 2            // trừ thêm 2 dấu '-'
	if cutLen < 0 {
		cutLen = 0
	}
	short := baseName[:cutLen]
	return fmt.Sprintf("%s-%s-%s", prefix, short, hash)
}
