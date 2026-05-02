test:
	rm -rf ./*.tgz
	helm package .
	helm install -n kube-system vngcloud-load-balancer-controller ./vngcloud-load-balancer-controller-0.0.1.tgz \
		--set mysecret.global.clientID="bbbb" \
		--set mysecret.global.clientSecret="cccc"
template:
	rm -rf ./*.tgz
	helm package .

	rm -rf ./helm/vngcloud-load-balancer-controller/templates

	helm template vngcloud-load-balancer-controller ./*.tgz \
		--namespace kube-system \
		--set mysecret.global.clientID="bbbb" \
		--set mysecret.global.clientSecret="cccc" \
		--debug --output-dir helm -f custom-value.yaml