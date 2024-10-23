test:
	helm package .
	helm install -n kube-system vngcloud-load-balancer-controller ./vngcloud-load-balancer-controller-0.0.1.tgz \
		--set mysecret.cluster.clusterID="aaaa" \
		--set mysecret.global.clientID="bbbb" \
		--set mysecret.global.clientSecret="cccc"