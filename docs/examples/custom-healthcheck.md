# Custom Health Check

By default, the controller uses a TCP health check on the node port. You can customise the health check using annotations or the `LoadBalancerConfig` CRD.

## HTTP Health Check via Annotations

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: default
  annotations:
    vks.vngcloud.vn/healthcheck-protocol: "HTTP"
    vks.vngcloud.vn/healthcheck-path: "/healthz"
    vks.vngcloud.vn/healthcheck-http-method: "GET"
    vks.vngcloud.vn/healthcheck-http-version: "1.1"
    vks.vngcloud.vn/success-codes: "200,201"
    vks.vngcloud.vn/healthcheck-interval-seconds: "15"
    vks.vngcloud.vn/healthcheck-timeout-seconds: "5"
    vks.vngcloud.vn/healthy-threshold-count: "3"
    vks.vngcloud.vn/unhealthy-threshold-count: "3"
spec:
  type: LoadBalancer
  selector:
    app: my-app
  ports:
    - port: 80
      targetPort: 8080
```

## HTTP Health Check via LoadBalancerConfig

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: LoadBalancerConfig
metadata:
  name: my-lb-config
  namespace: default
spec:
  type: Network
  subnetId: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  vpcId: "net-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  zoneId: "HCM-1"

  pools:
    - name: my-pool
      protocol: HTTP
      healthMonitor:
        protocol: HTTP
        healthCheckPath: "/healthz"
        healthCheckMethod: GET
        httpVersion: "1.1"
        domainName: "my-service.example.com"
        successCode: "200-299"
        interval: 15
        timeout: 5
        healthyThreshold: 3
        unhealthyThreshold: 3
```

## Health Check Protocol Options

| Protocol | Notes |
|---|---|
| `TCP` | Simple TCP connection check. Default. |
| `HTTP` | Send an HTTP request; check the response code. |
| `HTTPS` | Send an HTTPS request; check the response code. |
| `PING` | ICMP ping check. |

## Custom Port

To health check on a different port than the service port:

```yaml
annotations:
  vks.vngcloud.vn/healthcheck-port: "8086"  # metrics/healthz port
```
