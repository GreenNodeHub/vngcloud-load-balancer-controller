# Ingress (L7 Load Balancer)

The VNGCloud Load Balancer Controller provisions an **Application Load Balancer (ALB)** for each Kubernetes `Ingress` resource with the `vks.vngcloud.vn` ingress class.

## IngressClass

The controller manages Ingress resources that reference the `vngcloud` IngressClass. Ensure your Ingress sets the `ingressClassName` field:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
  namespace: default
  annotations:
    vks.vngcloud.vn/package-id: "lbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
spec:
  ingressClassName: vngcloud
  rules:
    - host: example.com
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

## HTTP Ingress

A basic HTTP ingress routes traffic based on host and path:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: http-ingress
  namespace: default
spec:
  ingressClassName: vngcloud
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: api-service
                port:
                  number: 8080
          - path: /
            pathType: Prefix
            backend:
              service:
                name: frontend-service
                port:
                  number: 80
```

## HTTPS / TLS Termination

To enable TLS termination, reference a Kubernetes TLS secret. The controller will automatically create the corresponding certificate in VNGCloud and attach it to the listener.

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
                  number: 443
```

!!! tip
    You can reference existing VNGCloud certificate IDs with the `vks.vngcloud.vn/certificate-ids` annotation, or let the controller create certificates from Kubernetes TLS secrets automatically.

## Advanced: Sticky Sessions

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/enable-sticky-session: "true"
```

## Advanced: Insert Custom Headers

```yaml
metadata:
  annotations:
    vks.vngcloud.vn/insert-headers: "X-Forwarded-Port:80,X-Custom-Header:value"
```

## Ingress Status

Once the Ingress is created, the controller populates the status with the load balancer's IP or hostname:

```bash
kubectl get ingress my-ingress
NAME         CLASS      HOSTS             ADDRESS           PORTS   AGE
my-ingress   vngcloud   example.com       203.0.113.42      80      2m
```

## Full Annotation Reference

See the [Ingress Annotations](../annotations/ingress.md) page for a complete list of all supported annotations.
