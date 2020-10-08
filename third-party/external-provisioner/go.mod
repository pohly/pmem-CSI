module github.com/kubernetes-csi/external-provisioner

go 1.15

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/mock v1.4.3
	github.com/kubernetes-csi/csi-lib-utils v0.8.1
	github.com/kubernetes-csi/csi-test/v3 v3.1.1
	github.com/kubernetes-csi/external-snapshotter/client/v2 v2.2.0-rc3
	github.com/spf13/pflag v1.0.5
	google.golang.org/grpc v1.29.1
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/apiserver v0.19.0
	k8s.io/client-go v0.19.1
	k8s.io/component-base v0.19.0
	k8s.io/csi-translation-lib v0.19.3
	k8s.io/klog/v2 v2.3.0
	k8s.io/kubernetes v1.19.0
	sigs.k8s.io/controller-runtime v0.6.2
	sigs.k8s.io/sig-storage-lib-external-provisioner/v6 v6.1.0
)

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.0

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.0

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.0

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.0

replace k8s.io/code-generator => k8s.io/code-generator v0.19.0

replace k8s.io/cri-api => k8s.io/cri-api v0.19.0

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.0

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.0

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.0

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.0

replace k8s.io/kubectl => k8s.io/kubectl v0.19.0

replace k8s.io/kubelet => k8s.io/kubelet v0.19.0

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.0

replace k8s.io/metrics => k8s.io/metrics v0.19.0

replace k8s.io/node-api => k8s.io/node-api v0.17.0

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.0

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.19.0

replace k8s.io/sample-controller => k8s.io/sample-controller v0.19.0

replace k8s.io/api => k8s.io/api v0.19.0

replace k8s.io/apimachinery => k8s.io/apimachinery v0.19.0

replace k8s.io/apiserver => k8s.io/apiserver v0.19.0

replace k8s.io/client-go => k8s.io/client-go v0.19.0

replace k8s.io/component-base => k8s.io/component-base v0.19.0

replace k8s.io/controller-manager => k8s.io/controller-manager v0.19.0

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.3