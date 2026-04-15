# NodeSecurityGroup CRD

The `NodeSecurityGroup` CRD lets you manage VNGCloud security groups for Kubernetes cluster nodes declaratively. You can attach existing security groups to nodes or create and manage a new security group with custom rules.

Short name: `nsg`

```bash
kubectl get nodesecuritygroup -A
```

## Example: Attach Existing Security Groups

Attach one or more existing VNGCloud security groups to nodes that match a label selector:

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: NodeSecurityGroup
metadata:
  name: attach-existing-sg
  namespace: kube-system
spec:
  selectNodeLabels:
    role: worker
  attachSecurityGroups:
    - "secg-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    - "secg-yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
```

## Example: Create and Manage a Security Group

Let the controller create a new security group with specific rules:

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: NodeSecurityGroup
metadata:
  name: managed-sg
  namespace: kube-system
spec:
  selectNodeLabels:
    role: worker
  managedSecurityGroup:
    name: "my-node-security-group"
    description: "Managed by vngcloud-load-balancer-controller"
    rules:
      - protocol: TCP
        fromPort: 30000
        toPort: 32767
        cidr: "10.0.0.0/8"
        direction: ingress
        etherType: IPv4
        description: "Allow NodePort traffic from internal network"
      - protocol: TCP
        fromPort: 443
        toPort: 443
        cidr: "0.0.0.0/0"
        direction: ingress
        etherType: IPv4
        description: "Allow HTTPS"
```

## Spec Reference

### NodeSecurityGroupSpec

| Field | Required | Description |
|---|---|---|
| `selectNodeLabels` | No | Label selector — only nodes with all matching labels are managed |
| `attachSecurityGroups` | No | List of existing VNGCloud security group IDs to attach to selected nodes |
| `managedSecurityGroup` | No | A security group to create and fully manage (rules, members) |

### ManagedSecurityGroup

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Name of the security group to create in VNGCloud |
| `description` | No | Description for the security group |
| `rules` | No | List of security group rules |

### NodeSecurityGroupRule

| Field | Required | Description |
|---|---|---|
| `protocol` | Yes | Protocol: `TCP`, `UDP`, `ICMP`, `ANY` |
| `fromPort` | Yes | Start of port range |
| `toPort` | Yes | End of port range |
| `cidr` | Yes | Source/destination CIDR block |
| `direction` | No | `ingress` or `egress` (default: `ingress`) |
| `etherType` | No | `IPv4` or `IPv6` (default: `IPv4`) |
| `description` | No | Human-readable description for the rule |

## Status

The controller updates the status after each reconcile:

```yaml
status:
  observedGeneration: 2
  lastReconcileTime: "2026-04-15T10:00:00Z"
  lastReconcileMessage: "NodeSecurityGroup reconciled successfully"
  selectedNodes:
    - name: worker-node-1
      serverId: "ins-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    - name: worker-node-2
      serverId: "ins-yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
  managedSecurityGroup:
    id: "secg-zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"
  serverSecurityGroups:
    - serverId: "ins-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      attachedSecurityGroupIds:
        - "secg-zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSuccess
```

## How It Works

1. The controller watches `NodeSecurityGroup` resources and the nodes in the cluster.
2. It filters nodes by `selectNodeLabels` to find the target set.
3. If `managedSecurityGroup` is set, the controller creates/updates that security group in VNGCloud with the declared rules.
4. It attaches all security groups (from `attachSecurityGroups` + the managed one) to the VNGCloud server instances backing the selected nodes.
5. When the `NodeSecurityGroup` is deleted, the controller detaches and (if managed) deletes the security group.
