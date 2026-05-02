# Upgrade

## Upgrade via Helm

```bash
helm upgrade vngcloud-load-balancer-controller \
  oci://vcr.vngcloud.vn/81-vks-public/vks-helm-charts/vngcloud-load-balancer-controller \
  --namespace kube-system \
  --reuse-values \
  --version <NEW_VERSION>
```

!!! warning
    Always check the [release notes](https://github.com/vngcloud/vngcloud-load-balancer-controller/releases) before upgrading. Some versions may require CRD migrations.

## Upgrade CRDs

CRDs are not automatically upgraded by Helm. After upgrading the chart, apply the updated CRDs manually:

```bash
kubectl apply -f https://raw.githubusercontent.com/vngcloud/vngcloud-load-balancer-controller/v3/dist/install.yaml
```

Or using the Helm chart's CRD directory:

```bash
helm pull oci://vcr.vngcloud.vn/81-vks-public/vks-helm-charts/vngcloud-load-balancer-controller --untar
kubectl apply -f vngcloud-load-balancer-controller/crds/
```

## Verify the upgrade

```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=vngcloud-load-balancer-controller
kubectl logs -n kube-system -l app.kubernetes.io/name=vngcloud-load-balancer-controller --tail=50
```
