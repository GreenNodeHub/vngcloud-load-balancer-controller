# Installation

## Prerequisites

- A running VKS (VNGCloud Kubernetes Service) cluster, Kubernetes v1.11.3+
- `kubectl` v1.11.3+
- `helm` v3+
- VNGCloud IAM credentials (Client ID and Client Secret)

## Install via Helm (recommended)

### HCM region

```bash
helm install vngcloud-load-balancer-controller \
  oci://vcr.vngcloud.vn/81-vks-public/vks-helm-charts/vngcloud-load-balancer-controller \
  --namespace kube-system \
  --set mysecret.global.clientID="<YOUR_CLIENT_ID>" \
  --set mysecret.global.clientSecret="<YOUR_CLIENT_SECRET>" \
  --set mysecret.global.vserverURL="https://hcm-3.api.vngcloud.vn/vserver"
```

### HAN region

```bash
helm install vngcloud-load-balancer-controller \
  oci://vcr-han.vngcloud.vn/81-vks-public/vks-helm-charts/vngcloud-load-balancer-controller \
  --namespace kube-system \
  --set mysecret.global.clientID="<YOUR_CLIENT_ID>" \
  --set mysecret.global.clientSecret="<YOUR_CLIENT_SECRET>" \
  --set mysecret.global.vserverURL="https://han-1.api.vngcloud.vn/vserver"
```

### Verify the installation

```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=vngcloud-load-balancer-controller
```

## Install via kubectl

Download the latest release manifest and apply:

```bash
kubectl apply -f https://raw.githubusercontent.com/vngcloud/vngcloud-load-balancer-controller/v3/dist/install.yaml
```

You must also create the configuration secret:

```bash
kubectl create secret generic vngcloud-load-balancer-controller-mysecret \
  --namespace kube-system \
  --from-literal=clientID=<YOUR_CLIENT_ID> \
  --from-literal=clientSecret=<YOUR_CLIENT_SECRET> \
  --from-literal=vserverURL=https://hcm-3.api.vngcloud.vn/vserver
```

## Uninstall

```bash
helm uninstall vngcloud-load-balancer-controller --namespace kube-system
```

Or if you installed via kubectl:

```bash
kubectl delete -f https://raw.githubusercontent.com/vngcloud/vngcloud-load-balancer-controller/v3/dist/install.yaml
```
