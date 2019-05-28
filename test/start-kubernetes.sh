#!/bin/bash
set -o errexit
set -o pipefail

TEST_DIRECTORY=${TEST_DIRECTORY:-$(dirname $(readlink -f $0))}
source ${TEST_CONFIG:-${TEST_DIRECTORY}/test-config.sh}
DEPLOYMENT_SUFFIX=${DEPLOYMENT_SUFFIX:-govm}
CLOUD_USER=${CLOUD_USER:-clear}
CLUSTER=${CLUSTER:-${CLOUD_USER}-${DEPLOYMENT_SUFFIX}}
DEPLOYMENT_ID=${DEPLOYMENT_ID:-pmem-csi-${CLUSTER}}
GOVM_YAML=${GOVM_YAML:-$(mktemp --suffix $DEPLOYMENT_ID.yml)}
REPO_DIRECTORY=${REPO_DIRECTORY:-$(dirname $TEST_DIRECTORY)}
TEST_DIRECTORY=${TEST_DIRECTORY:-$(dirname $(readlink -f $0))}
RESOURCES_DIRECTORY=${RESOURCES_DIRECTORY:-${REPO_DIRECTORY}/_work/resources}
WORKING_DIRECTORY="${WORKING_DIRECTORY:-${REPO_DIRECTORY}/_work/${CLUSTER}}"
NODES=( $DEPLOYMENT_ID-master
	$DEPLOYMENT_ID-worker1
	$DEPLOYMENT_ID-worker2
	$DEPLOYMENT_ID-worker3)
CLOUD="${CLOUD:-true}"
FLAVOR="${FLAVOR:-medium}"
SSH_KEY="${SSH_KEY:-${RESOURCES_DIRECTORY}/id_rsa}"
SSH_PUBLIC_KEY="${SSH_KEY}.pub"
IMAGE_TAG="${IMAGE_TAG:-canary}"
EFI="${EFI:-true}"
KVM_CPU_OPTS="${KVM_CPU_OPTS:-\
		-m 2G,slots=${TEST_MEM_SLOTS:-2},maxmem=34G -smp 4 \
		-machine pc,accel=kvm,nvdimm=on}"
EXTRA_QEMU_OPTS="${EXTRA_QWEMU_OPTS:-\
		-object memory-backend-file,id=mem1,share=${TEST_PMEM_SHARE:-on},\
		mem-path=/data/nvdimm0,size=${TEST_PMEM_MEM_SIZE:-32768}M \
		-device nvdimm,id=nvdimm1,memdev=mem1,label-size=${TEST_PMEM_LABEL_SIZE:-2097152} \
		-machine pc,nvdimm}"
CLOUD_USER=${CLOUD_USER:-clear}
if [ "$TEST_CLEAR_LINUX_VERSION" ]; then
    CLOUD_IMAGE="clear-$TEST_CLEAR_LINUX_VERSION-cloud.img.xz"
else
    : ${CLOUD_IMAGE:=$(\
	       curl -s https://download.clearlinux.org/image/latest-images |
		   awk '/cloud.img/ {print $0}')}
fi
IMAGE_URL=${IMAGE_URL:-https://download.clearlinux.org/releases/${CLOUD_IMAGE//[!0-9]/}/clear}
SSH_TIMEOUT=60
SSH_ARGS="-oIdentitiesOnly=yes -oStrictHostKeyChecking=no \
	-oUserKnownHostsFile=/dev/null -oLogLevel=error \
	-i ${SSH_KEY}"
: ${TEST_CREATE_REGISTRY:=false}
: ${TEST_CHECK_SIGNED_FILES:=true}

function error_handler(){
	local line="${1}"
	echo  "Error unning the ${BASH_COMMAND} on function ${FUNCNAME[1]} at line ${line}"
	delete_vms
}

function download_image(){
    pushd $RESOURCES_DIRECTORY &>/dev/null
	if [ -e "${CLOUD_IMAGE/.xz}" ]; then
		echo "$CLOUD_IMAGE found, skipping download"
		CLOUD_IMAGE=${CLOUD_IMAGE/.xz}
	else
		case $CLOUD_USER in
			clear)
				echo "Downloading ${CLOUD_IMAGE} image"
				curl -O ${IMAGE_URL}/${CLOUD_IMAGE}
				if $TEST_CHECK_SIGNED_FILES; then
					curl -s -O ${IMAGE_URL}/${CLOUD_IMAGE}-SHA512SUMS
					curl -s -O ${IMAGE_URL}/${CLOUD_IMAGE}-SHA512SUMS.sig
					curl -s -O ${IMAGE_URL}/ClearLinuxRoot.pem
					if ! openssl smime -verify \
					-in "${CLOUD_IMAGE}-SHA512SUMS.sig" \
					-inform DER \
					-content "${CLOUD_IMAGE}-SHA512SUMS" \
					-CAfile "ClearLinuxRoot.pem"; then
						cat <<-EOF
						Image verification failed, see error above.

						"unsupported certificate purpose" is a known issue caused by an incompatible openssl
						version (https://github.com/clearlinux/distribution/issues/85). Other errors might indicate
						a download error or man-in-the-middle attack.

						To skip image verification run:
						TEST_CHECK_SIGNED_FILES=false make start
						EOF
						exit 2
					fi
				fi
				unxz ${CLOUD_IMAGE}
				CLOUD_IMAGE=${CLOUD_IMAGE/.xz}
				;;
			ubuntu)
				base_url="https://cloud-images.ubuntu.com/disco/current/"
				sha_file="SHA256SUMS"
				curl -s -O ${base_url}/${CLOUD_IMAGE}.xz
				curl -s -O ${base_url}/${sha_file}
				if sha256sum $sha_file ${CLOUD_IMAGE}.xz; then
					unxz ${CLOUD_IMAGE}.xz
					CLOUD_IMAGE=${CLOUD_IMAGE/.xz}
				fi
				;;
			centos)
				base_url="https://cloud.centos.org/centos/7/images/"
				sha_file="sha256sum.txt"
				curl -s -O ${base_url}/${CLOUD_IMAGE}.xz
				curl -s -O ${base_url}/${sha_file}
				if sha256sum $sha_file ${CLOUD_IMAGE}.xz; then
					unxz ${CLOUD_IMAGE}.xz
					CLOUD_IMAGE=${CLOUD_IMAGE/.xz}
				fi
				;;
		esac
	fi
    popd &>/dev/null
}

function create_govm_yaml(){
	trap 'error_handler ${LINENO}' ERR
	cat <<-EOF > $GOVM_YAML
	---
	vms:
	EOF

	for node in ${NODES[@]}; do
	cat <<-EOF >> $GOVM_YAML
	  - name: ${node}
	    image: ${RESOURCES_DIRECTORY}/${CLOUD_IMAGE}
	    cloud: ${CLOUD}
	    flavor: ${FLAVOR}
	    workdir: ${WORKING_DIRECTORY}
	    sshkey: ${SSH_PUBLIC_KEY}
	    efi: ${EFI}
	    ContainerEnvVars:
	      - |
	        KVM_CPU_OPTS=
	        ${KVM_CPU_OPTS//$(echo -e "\t")/}
	      - |
	        EXTRA_QEMU_OPTS=
	        ${EXTRA_QEMU_OPTS//$(echo -e "\t")/}
	EOF
	done
}

function create_vms(){
	trap 'error_handler ${LINENO}' ERR
	STOP_VMS_SCRIPT="${WORKING_DIRECTORY}/stop.sh"
	RESTART_VMS_SCRIPT="${WORKING_DIRECTORY}/restart.sh"
	create_govm_yaml
	govm compose -f ${GOVM_YAML}
	IPS=$(govm list -f '{{select (filterRegexp . "Name" "'${DEPLOYMENT_ID}'") "IP"}}' | tac)

	#Create scripts to delete virtual machines
	echo "#!/bin/bash -e" > $STOP_VMS_SCRIPT
	govm list -f '{{select (filterRegexp . "Name" "'${DEPLOYMENT_ID}'") "Name"}}' \
		| xargs -L1 echo govm remove >> $STOP_VMS_SCRIPT
	echo "rm -rf ${WORKING_DIRECTORY}" >> $STOP_VMS_SCRIPT
	chmod +x $STOP_VMS_SCRIPT

	#Create script to restart virtual machines
	cat <<-EOF>$RESTART_VMS_SCRIPT
	#!/bin/bash
	echo "Rebooting virtual machines"
	for ip in $(echo ${IPS}); do
		ssh $SSH_ARGS ${CLOUD_USER}@\${ip} "sudo systemctl reboot"
	done
	echo "Waiting for ssh connectivity"
	for ip in $(echo ${IPS}); do
		while ! ssh $SSH_ARGS ${CLOUD_USER}@\${ip} exit 2>/dev/null; do
			if [ "\$SECONDS" -gt "$SSH_TIMEOUT" ]; then
				echo "Timeout accessing through ssh"
				exit 1
			fi
		done
	done
	while ! ${WORKING_DIRECTORY}/ssh-${CLUSTER} "kubectl get pods" 2>/dev/null; do
		sleep 1
		if [ \$SECONDS -gt $SSH_TIMEOUT ]; then
			break
		fi
	done
	echo "Waiting for kubernetes nodes to be ready"
	while [ \$SECONDS -lt $SSH_TIMEOUT ];do
		sleep 3
		pending_nodes=\$(${WORKING_DIRECTORY}/ssh-${CLUSTER} "kubectl get nodes  -o go-template \
		--template='{{range .items}}{{range .status.conditions }}{{if eq .type \"Ready\"}} \
		{{if ne .status \"True\"}}{{printf \"%s\n\" .reason}}{{end}}{{end}}{{end}}{{end}}'"2>/dev/null)
		if [ -z "\$pending_nodes" ]; then
			break
		fi
	done
	EOF
	chmod +x $RESTART_VMS_SCRIPT
	#Wait for the ssh connectivity in the vms
	for ip in ${IPS} ; do
		SECONDS=0
		NO_PROXY+=",$ip"
		echo "Waiting for ssh connectivity on vm with ip $ip"
		while ! ssh $SSH_ARGS ${CLOUD_USER}@${ip} exit 2>/dev/null; do
			if [ "$SECONDS" -gt "$SSH_TIMEOUT" ]; then
				echo "Timeout accessing through ssh"
				delete_vms
				exit 1
			fi
		done
		echo "ssh $SSH_ARGS ${CLOUD_USER}@${ip} \"sudo systemctl reboot\"" >> $RESTART_VMS_SCRIPT
	done
}

# Prints a single line of foo=<value of foo> assignments for
# proxy variables and all variables starting with TEST_.
function env_vars() (
    for var in $(set | grep -e '^[a-zA-Z_]*=' | sed -e 's/=.*//' | grep -e '^HTTP_PROXY$' -e '^HTTPS_PROXY$' -e '^NO_PROXY$' -e '^TEST_'); do
        echo -n " $var='${!var}'"
    done
)

function init_kubernetes_cluster(){
	trap 'error_handler ${LINENO}' ERR
	workers_ip=""
	master_ip="$(govm list -f '{{select (filterRegexp . "Name" "'${DEPLOYMENT_ID}-master'") "IP"}}')"
	join_token=""
	setup_script="setup-${CLOUD_USER}-govm.sh"
        install_k8s_script="setup-kubernetes.sh"
	KUBECONFIG=${WORKING_DIRECTORY}/kube.config
	echo "Installing dependencies on cloud images, this process may take some minutes"
	vm_id=0
	for ip in ${IPS}; do
		vm_name=$(govm list -f '{{select (filterRegexp . "IP" "'${ip}'") "Name"}}')
		log_name=${WORKING_DIRECTORY}/${vm_name}.log
		ssh_script=${WORKING_DIRECTORY}/ssh.${vm_id}
		((vm_id=vm_id+1))
		if [[ "$vm_name" = *"worker"* ]]; then
			workers_ip+="$ip "
		else
			echo "exec ssh $SSH_ARGS ${CLOUD_USER}@${ip} \"\$@\"" > ${WORKING_DIRECTORY}/ssh-${CLUSTER}
			chmod +x ${WORKING_DIRECTORY}/ssh-${CLUSTER}
		fi
		ENV_VARS="env$(env_vars) HOSTNAME='$vm_name' IPADDR='$ip'"
		scp $SSH_ARGS ${TEST_DIRECTORY}/{$setup_script,$install_k8s_script} ${CLOUD_USER}@${ip}:. >/dev/null
		ssh $SSH_ARGS ${CLOUD_USER}@${ip} "$ENV_VARS ./$setup_script && $ENV_VARS ./$install_k8s_script" &> >(sed -e "s/^/$vm_name: /" | tee -a $log_name ) &
		echo "exec ssh $SSH_ARGS ${CLOUD_USER}@${ip} \"\$@\"" > $ssh_script
		chmod +x $ssh_script
	done
	wait
	#get kubeconfig
	scp $SSH_ARGS ${CLOUD_USER}@${master_ip}:.kube/config $KUBECONFIG
	export KUBECONFIG=${KUBECONFIG}
        # Copy images to local registry in master vm?
    if $TEST_CREATE_REGISTRY; then
	images=( pmem-csi-driver pmem-vgm pmem-ns-init)
	for image in "${images[@]}" ; do
		echo "Saving $image"
		docker save ${TEST_PMEM_REGISTRY}/$image:$IMAGE_TAG > ${WORKING_DIRECTORY}/$image
		echo Copying image $image to master node
		scp $SSH_ARGS $image ${CLOUD_USER}@${master_ip}:.
		echo Load $image into registry
		ssh $SSH_ARGS ${CLOUD_USER}@${master_ip} "sudo docker load < $image"
		ssh $SSH_ARGS ${CLOUD_USER}@${master_ip} "sudo docker push localhost:5000/$image:$IMAGE_TAG"
		rm ${WORKING_DIRECTORY}/$image
        done
    fi

	#get kubernetes join token
	join_token=$(ssh $SSH_ARGS ${CLOUD_USER}@${master_ip} "$ENV_VARS kubeadm token create --print-join-command")
	for ip in ${workers_ip}; do

		vm_name=$(govm list -f '{{select (filterRegexp . "IP" "'${ip}'") "Name"}}')
		log_name=${WORKING_DIRECTORY}/${vm_name}.log
		(
		ssh $SSH_ARGS ${CLOUD_USER}@${ip} "$ENV_VARS sudo $join_token" &> >(sed -e "s/^/$vm_name:/" | tee -a $log_name )
		ssh $SSH_ARGS ${CLOUD_USER}@${master_ip} "kubectl label --overwrite node $vm_name storage=pmem" &> >(sed -e "s/^/$vm_name:/" | tee -a $log_name )
		) &
	done
	wait
}

function delete_vms(){
	trap 'error_handler ${LINENO}' ERR
	echo "Cleanning up environment"
	govm list -f '{{select (filterRegexp . "Name" "'${DEPLOYMENT_ID}'") "Name"}}' \
	| xargs -L1 govm remove
}

function init_workdir(){
	if [ ! -d "$WORKING_DIRECTORY" ]; then
		mkdir -p $WORKING_DIRECTORY
    fi
    if [ ! -d "$RESOURCES_DIRECTORY" ]; then
        mkdir -p $RESOURCES_DIRECTORY
    fi
	if [ ! -e  "$SSH_KEY" ]; then
		ssh-keygen -N '' -f ${SSH_KEY} &>/dev/null
	fi
	pushd $WORKING_DIRECTORY >/dev/null
}

function check_status(){
    deployments=$(govm list -f '{{select (filterRegexp . "Name" "'${DEPLOYMENT_ID}'") "Name"}}')
    if [ ! -z "$deployments" ]; then
        echo "Kubernetes cluster ${CLUSTER} is already running, using it unchanged."
        exit 0
    fi
}

check_status
init_workdir
download_image
create_vms
init_kubernetes_cluster
