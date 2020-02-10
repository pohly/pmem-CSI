#!/bin/bash

set -o errexit
set -o pipefail

TEST_DIRECTORY=${TEST_DIRECTORY:-$(dirname $(readlink -f $0))}
source ${TEST_CONFIG:-${TEST_DIRECTORY}/test-config.sh}

CLUSTER=${CLUSTER:-pmem-govm}
REPO_DIRECTORY="${REPO_DIRECTORY:-$(dirname $(dirname $(readlink -f $0)))}"
WORK_DIRECTORY="${WORK_DIRECTORY:-${REPO_DIRECTORY}/_work/${CLUSTER}}"
SSH="${WORK_DIRECTORY}/ssh-${CLUSTER}"
KUBECTL="${SSH} kubectl" # Always use the kubectl installed in the cluster.
KUBERNETES_VERSION="$(${KUBECTL} version --short | grep 'Server Version' | \
        sed -e 's/.*: v\([0-9]*\)\.\([0-9]*\)\..*/\1.\2/')"
DEPLOYMENT_DIRECTORY="${REPO_DIRECTORY}/deploy/kubernetes-$KUBERNETES_VERSION"
case ${TEST_DEPLOYMENTMODE} in
    testing)
        deployment_suffix="/testing";;
    production)
        deployment_suffix="";;
    *)
        echo >&2 "invalid TEST_DEPLOYMENTMODE: ${TEST_DEPLOYMENTMODE}"
        exit 1
esac
DEPLOY=(
    ${TEST_DEVICEMODE}${deployment_suffix}
    pmem-storageclass-ext4.yaml
    pmem-storageclass-xfs.yaml
    pmem-storageclass-cache.yaml
    pmem-storageclass-late-binding.yaml
    scheduler
    webhook
)

echo "$KUBERNETES_VERSION" > $WORK_DIRECTORY/kubernetes.version
for deploy in ${DEPLOY[@]}; do
    path="${DEPLOYMENT_DIRECTORY}/${deploy}"
    if [ -f "$path" ]; then
        ${KUBECTL} apply -f - <"$path"
    elif [ -d "$path" ]; then
        # A kustomize base. We need to copy all files over into the cluster, otherwise
        # `kubectl kustomize` won't work.
        tmpdir=$(${SSH} mktemp -d)
        case "$path" in /*) tar -C / -chf - "$(echo "$path" | sed -e 's;^/;;')";;
                         *) tar -chf - "$path";;
        esac | ${SSH} tar -xf - -C "$tmpdir"
        if [ -f "$path/pmem-csi.yaml" ]; then
            # Replace registry. This is easier with sed than kustomize...
            ${SSH} sed -i -e "s^intel/pmem^${TEST_PMEM_REGISTRY}/pmem^g" "$tmpdir/$path/pmem-csi.yaml"
        fi
        ${SSH} mkdir "$tmpdir/my-deployment"
        ${SSH} "cat >'$tmpdir/my-deployment/kustomization.yaml'" <<EOF
bases:
  - ../$path
EOF
        case $deploy in
            ${TEST_DEVICEMODE}${deployment_suffix})
                ${SSH} "cat >>'$tmpdir/my-deployment/kustomization.yaml'" <<EOF
patchesJson6902:
  - target:
      group: apps
      version: v1
      kind: StatefulSet
      name: pmem-csi-controller
    path: scheduler-patch.yaml
EOF
                ${SSH} "cat >'$tmpdir/my-deployment/scheduler-patch.yaml'" <<EOF
- op: add
  path: /spec/template/spec/containers/0/command/-
  value: "--schedulerListen=:8000" # Exposed to kube-scheduler via the pmem-csi-scheduler service.
EOF
                if [ "${TEST_DEVICEMODE}" = "lvm" ]; then
                    # Test these options and kustomization by injecting some non-default values.
                    # This could be made optional to test both default and non-default values,
                    # but for now we just change this in all deployments.
                    ${SSH} "cat >>'$tmpdir/my-deployment/kustomization.yaml'" <<EOF
  - target:
      group: apps
      version: v1
      kind: DaemonSet
      name: pmem-csi-node
    path: lvm-parameters-patch.yaml
EOF
                    ${SSH} "cat >'$tmpdir/my-deployment/lvm-parameters-patch.yaml'" <<EOF
- op: add
  path: /spec/template/spec/initContainers/0/command/-
  value: "--useforfsdax=50"
EOF
                fi
                ;;
            scheduler)
                # Change port number via JSON patch.
                ${SSH} "cat >>'$tmpdir/my-deployment/kustomization.yaml'" <<EOF
patchesJson6902:
  - target:
      version: v1
      kind: Service
      name: pmem-csi-scheduler
    path: scheduler-patch.yaml
EOF
                ${SSH} "cat >'$tmpdir/my-deployment/scheduler-patch.yaml'" <<EOF
- op: add
  path: /spec/ports/0/nodePort
  value: ${TEST_SCHEDULER_EXTENDER_NODE_PORT}
EOF
                ;;
            webhook)
                ${SSH} "cat >>'$tmpdir/my-deployment/kustomization.yaml'" <<EOF
patchesJson6902:
  - target:
      group: admissionregistration.k8s.io
      version: v1beta1
      kind: MutatingWebhookConfiguration
      name: pmem-csi-hook
    path: webhook-patch.yaml
EOF
                # The grep expression below is a kludge to avoid on
                # depending on a proper json parser like
                # jq -r '.clusters[0].cluster."certificate-authority-data"'
                #
                # It relies on the clusters array coming first.
                ${SSH} "cat >'$tmpdir/my-deployment/webhook-patch.yaml'" <<EOF
- op: replace
  path: /webhooks/0/clientConfig/url
  value: https://127.0.0.1:${TEST_SCHEDULER_EXTENDER_NODE_PORT}/pod/mutate
- op: replace
  path: /webhooks/0/clientConfig/caBundle
  value: $(${KUBECTL} config view --raw | grep certificate-authority-data: | head -n 1 | sed -e 's/.*certificate-authority-data://')
EOF
                ;;
        esac
        ${KUBECTL} apply --kustomize "$tmpdir/my-deployment"
        ${SSH} rm -rf "$tmpdir"
    else
        echo >&2 "$path is missing."
        exit 1
    fi
done

${KUBECTL} label --overwrite ns kube-system pmem-csi.intel.com/webhook=ignore

cat <<EOF

The test cluster is ready. Log in with ${WORK_DIRECTORY}/ssh-${CLUSTER}, run kubectl once logged in.
Alternatively, KUBECONFIG=${WORK_DIRECTORY}/kube.config can also be used directly.

To try out the pmem-csi driver persistent volumes:
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-pvc.yaml | ${KUBECTL} create -f -
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-app.yaml | ${KUBECTL} create -f -

To try out the pmem-csi driver cache volumes:
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-pvc-cache.yaml | ${KUBECTL} create -f -
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-app-cache.yaml | ${KUBECTL} create -f -
EOF

if [ -e ${DEPLOYMENT_DIRECTORY}/pmem-storageclass-late-binding.yaml ]; then
    cat <<EOF

To try out the pmem-csi driver persistent volumes with late binding:
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-pvc-late-binding.yaml | ${KUBECTL} create -f -
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-app-late-binding.yaml | ${KUBECTL} create -f -
EOF
fi

if [ -e ${DEPLOYMENT_DIRECTORY}/pmem-app-ephemeral.yaml ]; then
    cat <<EOF

To try out the pmem-csi driver ephemeral volumes:
   cat deploy/kubernetes-${KUBERNETES_VERSION}/pmem-app-ephemeral.yaml | ${KUBECTL} create -f -
EOF
fi
