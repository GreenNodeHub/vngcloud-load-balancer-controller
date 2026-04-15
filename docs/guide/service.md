# Service (L4 Load Balancer)

The VNGCloud Load Balancer Controller provisions a **Network Load Balancer (NLB)** for each Kubernetes `Service` of type `LoadBalancer`.

## Basic Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: default
spec:
  type: LoadBalancer
  selector:
    app: my-app
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 8080
```

Once created, check the status for the assigned IP address:

```bash
kubectl get service my-service
NAME         TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)        AGE
my-service   LoadBalancer   10.96.42.100    203.0.113.42    80:32000/TCP   2m
```

## Internal Load Balancer

To create an internal (private) load balancer that is only reachable within your VPC:

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/scheme: "Internal"
```

## Customising the Package (Size)

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/package-id: "lbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
```

## Proxy Protocol (L4)

Enable PROXY protocol so your backend applications can see the real client IP:

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/enable-proxy-protocol: "true"
```

## Target Type

Controls whether traffic is routed to node ports or directly to pod IPs:

| Value | Description |
|---|---|
| `instance` (default) | Routes to `NodePort` on each node |
| `ip` | Routes directly to pod IPs (requires Cilium native routing) |

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/target-type: "ip"
```

## InterVPC Load Balancer

For cross-VPC scenarios, set the scheme to `InterVPC` and provide the private subnet:

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/scheme: "InterVPC"
    vks.vngcloud.vn/private-subnet-id: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    vks.vngcloud.vn/private-zone-id: "HAN-1"
```

## Non-LoadBalancer Service Types

The controller can attach a load balancer to `NodePort` or `ClusterIP` services using the `enable-load-balancer` annotation:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-nodeport-service
  annotations:
    vks.vngcloud.vn/enable-load-balancer: "true"
spec:
  type: NodePort
  # ...
```

!!! note
    For `ClusterIP` type, only works with Cilium native routing; target type is always `ip`.

## Targeting Specific Nodes

Use `target-node-labels` to restrict which nodes are added as pool members:

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/target-node-labels: "role=worker,topology.kubernetes.io/zone=hcm-3a"
```

## Full Annotation Reference

See the [Service Annotations](../annotations/service.md) page for a complete list of all supported annotations.
