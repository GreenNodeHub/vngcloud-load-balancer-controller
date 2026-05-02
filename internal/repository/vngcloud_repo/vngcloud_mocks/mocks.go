package vngcloud_mocks

import (
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ServerId1 = "ins-00000000-0000-0000-0000-000000000001"
	ServerId2 = "ins-00000000-0000-0000-0000-000000000002"
	ServerId3 = "ins-00000000-0000-0000-0000-000000000003"
	ServerId4 = "ins-00000000-0000-0000-0000-000000000004"
)

const (
	MockL7PackageName = "ALB_Small"
	MockL7PackageId   = "lbp-77777777"

	MockL4PackageName = "NLB_Small"
	MockL4PackageId   = "lbp-44444444"
)

const (
	MockProjectID = "projectID"
	MockNetID     = "netID"
	MockNetCIDR   = "199.0.0.0/16"

	MockSubnetID        = "subnetID-hcm-1a"
	MockSubnetID_1b_1   = "subnetID-hcm-1b-1"
	MockSubnetID_1b_2   = "subnetID-hcm-1b-2"
	MockSubnetCIDR      = "199.0.0.0/24"
	MockSubnetCIDR_1b_1 = "299.0.0.0/24"
	MockSubnetCIDR_1b_2 = "399.0.0.0/24"
)

const (
	MockLBNameError = "error-lb" // create lb with this name will be error
)

var (
	MockNode1 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-1",
			Labels: map[string]string{
				"nodeName":                  "mock-node-1",
				"nodeGroup":                 "mock-node-group-a",
				"vks.vngcloud.vn/mgmt-zone": "mock-mgmt-zone",
				"node.kubernetes.io/flavor": "s-general-1",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://" + ServerId1,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeHostName, Address: "mock-node-1"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	MockNode2 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-2",
			Labels: map[string]string{
				"nodeName":                  "mock-node-2",
				"nodeGroup":                 "mock-node-group-a",
				"node.kubernetes.io/flavor": "s-general-2",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://" + ServerId2,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.2"},
				{Type: corev1.NodeHostName, Address: "mock-node-2"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	MockNode3 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-3",
			Labels: map[string]string{
				"nodeName":                  "mock-node-3",
				"nodeGroup":                 "mock-node-group-b",
				"node.kubernetes.io/flavor": "s-general-2",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://" + ServerId3,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.3"},
				{Type: corev1.NodeHostName, Address: "mock-node-3"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	MockNode4 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-4",
			Labels: map[string]string{
				"nodeName":                  "mock-node-4",
				"nodeGroup":                 "mock-node-group-b",
				"node.kubernetes.io/flavor": "s-general-2",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://" + ServerId4,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.4"},
				{Type: corev1.NodeHostName, Address: "mock-node-4"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	NodeZones     = []string{string(common.HCM_03_1A_ZONE), string(common.HCM_03_1B_ZONE)}
	NodeSubnetIDs = []string{MockSubnetID, MockSubnetID_1b_1, MockSubnetID_1b_2}

	MapServerIdToSubnet = map[string]string{
		ServerId1: MockSubnetID,
		ServerId2: MockSubnetID,
		ServerId3: MockSubnetID_1b_1,
		ServerId4: MockSubnetID_1b_2,
	}

	MapSubnetToZone = map[string]string{
		MockSubnetID:      string(common.HCM_03_1A_ZONE),
		MockSubnetID_1b_1: string(common.HCM_03_1B_ZONE),
		MockSubnetID_1b_2: string(common.HCM_03_1B_ZONE),
	}

	MapSubnetToCIDR = map[string]string{
		MockSubnetID:      MockSubnetCIDR,
		MockSubnetID_1b_1: MockSubnetCIDR_1b_1,
		MockSubnetID_1b_2: MockSubnetCIDR_1b_2,
	}
)

var (
	MockCerts = []string{"cert1", "cert2", "cert3"}
)
