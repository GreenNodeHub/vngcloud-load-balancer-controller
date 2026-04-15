# NodeSecurityGroup CRD

The `NodeSecurityGroup` custom resource allows you to manage VNGCloud security group rules for the nodes in your Kubernetes cluster.

!!! info
    This feature is currently in alpha (`v1alpha1`). The API may change in future releases.

## Overview

When a load balancer is created, the controller automatically manages the security group rules on cluster nodes to allow health check and traffic flows. The `NodeSecurityGroup` CRD gives you visibility and fine-grained control over these rules.

```bash
kubectl get nodesecuritygroup -A
```

Short name: `nsg`

## Example

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: NodeSecurityGroup
metadata:
  name: my-nsg
  namespace: kube-system
spec:
  # Security group rules are managed by the controller
  # based on active load balancers in the cluster
```

## How It Works

1. When a `Service` of type `LoadBalancer` (or `LoadBalancerConfig`) is reconciled, the controller identifies the VNGCloud security groups attached to cluster nodes.
2. It creates or updates inbound rules to allow traffic from the load balancer's subnet and health check probes.
3. When the service is deleted, the controller removes the corresponding rules.

## Status

The `NodeSecurityGroup` status reflects the current security group rules managed by the controller:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSuccess
```
