# Internal Load Balancer

An internal load balancer is only reachable within your VPC, making it suitable for private services, inter-service communication, and database proxies.

## Service with Internal Scheme

```yaml
apiVersion: v1
kind: Service
metadata:
  name: internal-service
  namespace: default
  annotations:
    vks.vngcloud.vn/scheme: "Internal"
    vks.vngcloud.vn/package-id: "lbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
spec:
  type: LoadBalancer
  selector:
    app: my-private-app
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

## Ingress with Internal Scheme

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: internal-ingress
  namespace: default
  annotations:
    vks.vngcloud.vn/scheme: "Internal"
spec:
  ingressClassName: vngcloud
  rules:
    - host: internal.example.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-private-app
                port:
                  number: 80
```

## InterVPC Load Balancer

An InterVPC load balancer enables cross-VPC traffic routing. The load balancer has a private interface in the client's VPC.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: intervpc-service
  namespace: default
  annotations:
    vks.vngcloud.vn/scheme: "InterVPC"
    vks.vngcloud.vn/private-subnet-id: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    vks.vngcloud.vn/private-zone-id: "HCM-1"
spec:
  type: LoadBalancer
  selector:
    app: my-app
  ports:
    - port: 443
      targetPort: 8443
```

The InterVPC setup requires:
- `vks.vngcloud.vn/scheme: "InterVPC"` — declares the scheme
- `vks.vngcloud.vn/private-subnet-id` — the subnet in the **client** VPC where the LB's private interface is placed
- `vks.vngcloud.vn/private-zone-id` — the zone of that private subnet

Super-client credentials must be configured in the controller for cross-VPC management. See [Configuration](../deploy/configuration.md).
