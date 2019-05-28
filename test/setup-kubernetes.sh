#!/bin/bash
#
# The generic part of the Kubernetes cluster setup.
#
# This script runs *inside* the cluster. All setting env variables
# used by it must be passed in explicitly via ssh.

set -x
set -o errexit
set -o pipefail

HOSTNAME=${HOSTNAME:-$1}
TEST_CRI=${TEST_CRI:-docker}
INIT_REGION=${INIT_REGION:-TRUE}
CREATE_REGISTRY=${CREATE_REGISTRY:-false}

function error_handler(){
        local line="${1}"
        echo  "Running the ${BASH_COMMAND} on function ${FUNCNAME[1]} at line ${line}"
}

function create_local_registry(){
trap 'error_handler ${LINENO}' ERR
sudo docker run -d -p 5000:5000 --restart=always --name registry registry:2
}

function setup_kubernetes_master(){
trap 'error_handler ${LINENO}' ERR
kubeadm_args=
kubeadm_args_init=
kubeadm_config_init="apiVersion: kubeadm.k8s.io/v1beta1
kind: InitConfiguration"
kubeadm_config_cluster="apiVersion: kubeadm.k8s.io/v1beta1
kind: ClusterConfiguration"
kubeadm_config_kubelet="apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration"
kubeadm_config_file="/tmp/kubeadm-config.yaml"

case $TEST_CRI in
	docker)
		cri_daemon=docker
		# [ERROR SystemVerification]: unsupported docker version: 18.06.1
		kubeadm_args="$kubeadm_args --ignore-preflight-errors=SystemVerification"
	;;
	crio)
		cri_daemon=cri-o
		# Needed for CRI-O (https://clearlinux.org/documentation/clear-linux/tutorials/kubernetes).
		kubeadm_config_init="$kubeadm_config_init
nodeRegistration:
  criSocket: /run/crio/crio.sock"
	;;
    *)
	echo "ERROR: unsupported TEST_CRI=$TEST_CRI"
	exit 1
	;;
esac
# Needed for flannel (https://clearlinux.org/documentation/clear-linux/tutorials/kubernetes).
kubeadm_config_cluster="$kubeadm_config_cluster
networking:
  podSubnet: \"10.244.0.0/16\""


if [ ! -z ${TEST_FEATURE_GATES} ]; then
    kubeadm_config_kubelet="$kubeadm_config_kubelet
featureGates:
$(IFS=","; for f in ${TEST_FEATURE_GATES};do
echo "  $f" | sed 's/=/: /g'
done)"
    kubeadm_config_cluster="$kubeadm_config_cluster
apiServer:
  extraArgs:
    feature-gates: ${TEST_FEATURE_GATES}"
fi

# TODO: it is possible to set up each node in parallel, see
# https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-init/#automating-kubeadm


cat >${kubeadm_config_file} <<EOF
$kubeadm_config_init
---
$kubeadm_config_kubelet
---
$kubeadm_config_cluster
EOF

kubeadm_args_init="$kubeadm_args_init --config=$kubeadm_config_file"
sudo kubeadm init $kubeadm_args $kubeadm_args_init
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config


# Verify that Kubernetes works by starting it and then listing pods.
# We also wait for the node to become ready, which can take a while because
# images might still need to be pulled. This can take minutes, therefore we sleep
# for one minute between output.
echo "Waiting for Kubernetes cluster to become ready..."
while ! kubectl get nodes | grep -q 'Ready'; do
        kubectl get nodes
        kubectl get pods --all-namespaces
        sleep 1
done
kubectl get nodes
kubectl get pods --all-namespaces

${TEST_CONFIGURE_POST_MASTER}

# From https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/#pod-network
kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/bc79dd1505b0c8681ece4de4c0d86c5cd2643275/Documentation/kube-flannel.yml

# Install addon storage CRDs, needed if certain feature gates are enabled.
# Only applicable to Kubernetes 1.13 and older. 1.14 will have them as builtin APIs.
if kubectl version | grep -q '^Server Version.*Major:"1", Minor:"1[01234]"'; then
    if [[ "$TEST_FEATURE_GATES" == *"CSINodeInfo=true"* ]]; then
        kubectl create -f https://raw.githubusercontent.com/kubernetes/kubernetes/release-1.13/cluster/addons/storage-crds/csidriver.yaml
    fi
    if [[ "$TEST_FEATURE_GATES" == *"CSIDriverRegistry=true"* ]]; then
        kubectl create -f https://raw.githubusercontent.com/kubernetes/kubernetes/release-1.13/cluster/addons/storage-crds/csinodeinfo.yaml
    fi
fi

# Run additional commands specified in config.
${TEST_CONFIGURE_POST_ALL}

}


function init_region(){
trap 'error_handler ${LINENO}' ERR
sudo ndctl disable-region region0
sudo ndctl init-labels nmem0
sudo ndctl enable-region region0

}

if [ "$INIT_REGION" = "TRUE" ]; then
	init_region
fi
if [[ "$HOSTNAME" == *"master"* ]]; then
	setup_kubernetes_master
    if [ "$CREATE_REGISTRY" = "true" ]; then
	    create_local_registry
    fi
fi
