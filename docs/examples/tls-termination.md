# TLS Termination

This example shows how to terminate TLS at the VNGCloud Application Load Balancer and forward plain HTTP to your backend pods.

## Prerequisites

- A TLS certificate. You can either:
  - Store it as a Kubernetes TLS secret and let the controller upload it automatically, or
  - Use an existing VNGCloud certificate by its ID.

## Option 1: Kubernetes TLS Secret (auto-upload)

Create a TLS secret:

```bash
kubectl create secret tls my-tls-secret \
  --cert=tls.crt \
  --key=tls.key \
  --namespace default
```

Create an Ingress that references the secret:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: tls-ingress
  namespace: default
  annotations:
    vks.vngcloud.vn/package-id: "lbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
spec:
  ingressClassName: vngcloud
  tls:
    - hosts:
        - secure.example.com
      secretName: my-tls-secret
  rules:
    - host: secure.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-service
                port:
                  number: 80
```

The controller automatically:
1. Reads the TLS secret
2. Creates a certificate in VNGCloud
3. Attaches the certificate to the HTTPS listener

## Option 2: Existing VNGCloud Certificate

If you already have a certificate in VNGCloud:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: tls-ingress
  namespace: default
  annotations:
    vks.vngcloud.vn/certificate-ids: "cert-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
spec:
  ingressClassName: vngcloud
  rules:
    - host: secure.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-service
                port:
                  number: 80
```

## HTTP to HTTPS Redirect

Combine an HTTP and HTTPS listener using a `LoadBalancerConfig` CRD:

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: LoadBalancerConfig
metadata:
  name: my-alb-with-redirect
  namespace: default
spec:
  type: Application
  subnetId: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  vpcId: "net-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  zoneId: "HCM-1"

  pools:
    - name: web-pool
      protocol: HTTP

  listeners:
    - name: http-listener
      protocol: HTTP
      protocolPort: 80
      policies:
        - name: redirect-to-https
          action: REDIRECT_TO_URL
          redirectUrl: "https://secure.example.com"
          redirectHttpCode: 301
          l7Rules: []

    - name: https-listener
      protocol: TERMINATED_HTTPS
      protocolPort: 443
      defaultPoolName: web-pool
      certificateDefault:
        secretName: my-tls-secret
```

## Verify

```bash
curl -I https://secure.example.com
# HTTP/2 200
# server: nginx

# Test redirect
curl -I http://secure.example.com
# HTTP/1.1 301 Moved Permanently
# location: https://secure.example.com
```
