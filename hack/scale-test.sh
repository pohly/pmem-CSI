#!/bin/bash
#
# Copyright 2020 Intel Corporation
#
# This script needs to be invoked inside the PMEM-CSI repository.
#
# It depends on:
# - git
# - kubectl configured with access to a cluster
# - Vertical Pod Autoscaler installed in the cluster
# - kube-controller-manager running with
#   --kube-api-burst=100000
#   --kube-api-qps=100000
# - worker nodes being labeled with storage=pmem
#
# For each test run, the PMEM-CSI driver is installed such
# that it fakes device operations (i.e. no PMEM needed, little
# actual work per volume).
#
# Each test run creates and deletes volumes with an increasing
# provisioning rate until the apiserver load becomes too high.
#
# This is done with and without distributed provisioning.

set -ex

result_dir=$(pwd)/$(date +_work/clusterloader-%Y%m%d-%H%M)
mkdir -p $result_dir
exec &> >(tee $result_dir/output.log)

expected_rate=5 # volumes per second provisioned - this depends mostly on QPS/burst settings kube-controller-manager
expected_duration=$((60 * 5)) # 5 Minutes per test run

# Number of volumes determines test duration. With an expected rate of
# ~50 volumes/second, we get a reasonable overall test duration of a
# few minutes per test when using this number of volumes.
num_volumes=$(($expected_rate * $expected_duration))

# Testing at different rates of PVCs per second is necessary to
# determine where the maximum supported rate is.
volume_rates="150"

modes="distributed central"

nodes=$(kubectl get nodes --no-headers -l storage=pmem)
num_nodes=$(echo "$nodes" | wc -l)

# mode is either "central" or "distributed"
install_pmem_csi () (
    test_dir=$1
    shift
    mode=$1
    shift
    base=$1
    shift
    max=$1

    # Delete any previous objects. This fails when there isn't anything to delete,
    # so we ignore errors here (hack!).
    kubectl delete --wait -f deploy/kubernetes-1.19/pmem-csi-fake.yaml || true
    kubectl delete --wait -f deploy/kubernetes-1.19-distributed/pmem-csi-fake.yaml || true
    kubectl delete -k deploy/kustomize/vpa-for-pmem-csi || true

    # Reinstall.
    yaml=
    case "$mode" in
        central)
            yaml=deploy/kubernetes-1.19/pmem-csi-fake.yaml
            ;;
        distributed)
            yaml=deploy/kubernetes-1.19-distributed/pmem-csi-fake.yaml
            ;;
    esac
    test/setup-ca-kubernetes.sh
    # Unmerged features are needed, therefore custom images have to be used.
    # QPS gets set so high that it shouldn't be the limiting factor.
    sed \
        -e 's;kube-api-qps=.*;kube-api-qps=100000;' \
        -e 's;-v=3;-v=3;' \
        -e 's;intel/pmem-csi-driver:canary;pohly/pmem-csi-driver:canary-2020-11-30;' \
        -e 's;pohly/csi-provisioner:.*;pohly/csi-provisioner:2020-12-07-2;' \
        -e "s;node-deployment-base-delay=.*;node-deployment-base-delay=$base;" \
        -e "s;node-deployment-max-delay=.*;node-deployment-max-delay=$max;" \
        $yaml | tee $test_dir/pmem-csi.yaml | kubectl create -f -

    # Tell Vertical Pod Autoscaler to provide recommendations for the PMEM-CSI StatefulSet
    # and DaemonSet. This also covers external-provisioner and driver-registrar.
    kubectl create -k deploy/kustomize/vpa-for-pmem-csi

    # Wait for all pods to run. A better solution would be to query the controller metrics
    # and check how many nodes have registered, but accessing that service is more complicated
    # (kubectl port-forward ... ).
    kubectl wait --timeout=5m --for=condition=Ready pod/pmem-csi-controller-0
    start=$SECONDS
    while true; do
        num_ready=$(kubectl get -o jsonpath={.status.numberAvailable} daemonset/pmem-csi-node)
        if [ "$num_ready" ] && [ "$num_ready" -eq $num_nodes ]; then
            break
        fi
        if [ $(($SECONDS - $start)) -gt 600 ]; then
            echo "PMEM-CSI node pods not ready after 10 minutes"
            exit 1
        fi
        sleep 5
    done

    # Dump output in the background.
    mkdir -p $test_dir/pmem-csi-logs
    kubectl get pods -o=jsonpath='{range .items[*]}{"\n"}{.metadata.name}{" "}{.spec.nodeName}{" "}{range .spec.containers[*]}{.name}{" "}{end}{end}' | while read -r pod node containers; do
        for container in $containers; do
            kubectl logs -f $pod $container >$test_dir/pmem-csi-logs/$node.$pod.$container.log &
        done
    done

    kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: pmem-csi-sc
provisioner: pmem-csi.intel.com
EOF
)

# A currently unmerged PR has the test that we want to run.
# https://github.com/kubernetes/perf-tests/pull/1530
install_clusterloader () (
    if ! [ -d _work/perf-tests ]; then
        git clone https://github.com/pohly/perf-tests.git _work/perf-tests
    fi
    cd _work/perf-tests
    git fetch https://github.com/pohly/perf-tests.git pvc-creation-test
    git checkout FETCH_HEAD
)

dump_state () (
    dir=$1

    mkdir -p $dir/after
    kubectl get -o yaml pv >$dir/after/pv.yaml
    kubectl describe pv >$dir/after/pv.log
    kubectl get -o yaml --all-namespaces pvc >$dir/after/pvc.yaml
    kubectl describe --all-namespaces pvc >$dir/after/pvc.log

    echo $(grep -r "conflict during PVC.*update" $dir/pmem-csi-logs | wc -l) >$dir/after/conflicts.log
)

run_tests () (
    mode=$1
    shift
    base=$1
    shift
    max=$1
    shift

    volumes_per_node=$(($num_volumes / $num_nodes))
    actual_num_volumes=$(($num_nodes * $volumes_per_node))
    for rate in $volume_rates; do
        unique_name=$mode-qps-$rate-volumes-$num_volumes-base-$base-max-$max
        short_unique_name=$mode-$rate-$num_volumes-$base-$max
        test_dir=$result_dir/$unique_name
        mkdir -p $test_dir
        install_pmem_csi $test_dir $mode $base $max

        cat >$test_dir/overrides.yaml <<EOF
# Should be turned on if possible. In the PMEM-CSI QEMU cluster
# it fails:
# E1119 09:58:23.711358  100846 clusterloader.go:223] --------------------------------------------------------------------------------
# E1119 09:58:23.711365  100846 clusterloader.go:224] Test Finished
# E1119 09:58:23.711371  100846 clusterloader.go:225]   Test: testing/experimental/storage/pod-startup/config.yaml
# E1119 09:58:23.711377  100846 clusterloader.go:226]   Status: Fail
# E1119 09:58:23.711383  100846 clusterloader.go:228]   Errors: [measurement call TestMetrics - TestMetrics error: [action start failed for SchedulingMetrics measurement: # unexpected error (code: 7) in ssh connection to master: <nil>]
# measurement call TestMetrics - TestMetrics error: [action gather failed for SchedulingMetrics measurement: unexpected error (code: 7) in ssh connection to master: <nil>
# action gather failed for MetricsForE2E measurement: Errors while grabbing metrics: [error waiting for controller manager pod to expose metrics: timed out waiting for the condition; an error on the server ("unknown") has prevented the request from succeeding (get pods kube-controller-manager-pmem-csi-pmem-govm-master:10252)]]]

GATHER_METRICS: false

STEP_TIME_SECONDS: $(($expected_duration * 3))

# Ignore PVs from other provisioners.
EXPECTED_PROVISIONER: pmem-csi.intel.com

VOLUMES_PER_POD: 1
NODES_PER_NAMESPACE: $num_nodes # one namespace is enough
PODS_PER_NODE: $volumes_per_node
POD_THROUGHPUT: $rate

# PMEM-CSI has a fake capacity of 1Ti per node when simulating storage.
# We want to fill that up completely to ensure that volume
# creation really must use all nodes.
VOL_SIZE: $((1024 * 1024 * 1024 * 1024 / $volumes_per_node))
STORAGE_CLASS: pmem-csi-sc

# Not interested in actually running pods.
START_PODS: false
EOF

        (trap "dump_state $test_dir" EXIT; cd _work/perf-tests/clusterloader2 && go run cmd/clusterloader.go -v=3 --report-dir=$test_dir --kubeconfig=$KUBECONFIG --provider=local --nodes=$num_nodes --testconfig=testing/experimental/storage/pod-startup/config.yaml --testoverrides=testing/experimental/storage/pod-startup/volume-types/persistentvolume/override.yaml --testoverrides=$test_dir/overrides.yaml) || true

        # Get VPA recommendations.
        kubectl describe vpa/pmem-csi-controller vpa/pmem-csi-node >$test_dir/vpa.log
        kubectl get -o yaml vpa/pmem-csi-controller vpa/pmem-csi-node >$test_dir/vpa.yaml
    done
)


install_clusterloader

for mode in $modes; do
    run_tests $mode 10s 30s 0

    if [ $mode = "distributed" ]; then
        run_tests $mode 20s 30s 0
        run_tests $mode 30s 60s 0
    fi
done
