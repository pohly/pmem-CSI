pipeline {
    options {
        timestamps()
    }
    agent {
        label "pmem-csi"
    }

    environment {
        /*
          Change this into "true" to enable capturing the journalctl
          output of the build host and each VM, either by editing the
          Jenkinsfile in a PR or by logging into Jenkins and editing
          the pipeline before running it again.
        */
        LOGGING_JOURNALCTL = "false"

        /*
          Delay in seconds between dumping system statistics.
        */
        LOGGING_SAMPLING_DELAY = "60"

        /*
          Pod names in the kube-system namespace for which
          log output is to be captured. Empty by default,
          valid values:
          etcd kube-apiserver kube-controller-manager kube-scheduler
        */
        LOGGING_PODS = " " // the space is intentional, otherwise ${env.LOGGING_PODS} expands to null below

        /*
          For each major Kubernetes release we need one version of Clear Linux
          which had that release. Installing different Kubernetes releases
          on the latest Clear Linux is not supported because we always
          use the Clear Linux kubelet, and a more recent kubelet than
          the control plane is unsupported.
        */

        CLEAR_LINUX_VERSION_1_15 = "31070"
        /* 29890 broke networking
        (https://github.com/clearlinux/distribution/issues/904). In
        29880, Docker forgets containers after a system restart
        (https://github.com/clearlinux/distribution/issues/891). We
        need to stay on the latest known-good version. The version
        between *20 and *80 have not been tested. */
        CLEAR_LINUX_VERSION_1_14 = "29820"

        /* last version before the 1.14 update in 28630 */
        CLEAR_LINUX_VERSION_1_13 = "28620"

        REGISTRY_NAME = "cloud-native-image-registry.westus.cloudapp.azure.com"

        // Per-branch build environment, marked as "do not promote to public registry".
        // Set below via a script, must *not* be set here as it can't be overwritten.
        // BUILD_IMAGE = ""

        // Tag or branch name that is getting built, depending on the job.
        // Set below via a script, must *not* be set here as it can't be overwritten.
        // BUILD_TARGET = ""

        // This image is pulled at the beginning and used as cache.
        // TODO: Here we use "canary" which is correct for the "devel" branch, but other
        // branches may need something else to get better caching.
        PMEM_CSI_IMAGE = "${env.REGISTRY_NAME}/pmem-csi-driver:canary"
    }

    stages {
        stage('Create build environment') {
            options {
                timeout(time: 60, unit: "MINUTES")
            }

            steps {
                sh 'docker version'
                sh 'git version'
                sh 'free || true'
                sh 'command -v top >/dev/null 2>&1 || \
                    if command -v apt-get >/dev/null 2>&1; then \
                        sudo apt-get install procps; \
                    else \
                        sudo yum -y install procps; \
                    fi'
                sh 'head -n 30 /proc/cpuinfo; echo ...; tail -n 30 /proc/cpuinfo'
                sh "git remote set-url origin git@github.com:intel/pmem-csi.git"
                sh "git config user.name 'Intel Kubernetes CI/CD Bot'"
                sh "git config user.email 'k8s-bot@intel.com'"
                // known_hosts entry created and verified as described in https://serverfault.com/questions/856194/securely-add-a-host-e-g-github-to-the-ssh-known-hosts-file
                sh "mkdir -p ~/.ssh && echo 'github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7hRGmdnm9tUDbO9IDSwBK6TbQa+PXYPCPy6rbTrTtw7PHkccKrpp0yVhp5HdEIcKr6pLlVDBfOLX9QUsyCOV0wzfjIJNlGEYsdlLJizHhbn2mUjvSAHQqZETYP81eFzLQNnPHt4EVVUh7VfDESU84KezmD5QlWpXLmvU31/yMf+Se8xhHTvKSCZIFImWwoG6mbUoWf9nzpIoaSjB+weqqUUmpaaasXVal72J+UX2B+2RPW3RcT0eOzQgqlJL3RKrTJvdsjE3JEAvGq3lGHSZXy28G3skua2SmVi/w4yCE6gbODqnTWlg7+wC604ydGXA8VJiS5ap43JXiUFFAaQ==' >>~/.ssh/known_hosts && chmod -R go-rxw ~/.ssh"
                withDockerRegistry([ credentialsId: "e16bd38a-76cb-4900-a5cb-7f6aa3aeb22d", url: "https://${REGISTRY_NAME}" ]) {
                    script {
                        // Despite its name, GIT_LOCAL_BRANCH contains the tag name when building a tag.
                        // At some point it also contained the branch name when building
                        // a branch, but not anymore, therefore we fall back to BRANCH_NAME
                        // if unset. Even that isn't set in non-multibranch jobs
                        // (https://issues.jenkins-ci.org/browse/JENKINS-47226), but at least
                        // then we have GIT_BRANCH.
                        if (env.GIT_LOCAL_BRANCH != null) {
                            env.BUILD_TARGET = env.GIT_LOCAL_BRANCH
                        } else if ( env.BRANCH_NAME != null ) {
                            env.BUILD_TARGET = env.BRANCH_NAME
                        } else {
                            env.BUILD_TARGET = env.GIT_BRANCH - 'origin/' // Strip prefix.
                        }
                        if (env.CHANGE_ID != null) {
                            env.BUILD_IMAGE = "${env.REGISTRY_NAME}/pmem-clearlinux-builder:${env.CHANGE_TARGET}-rejected"

                            // Pull previous image and use it as cache (https://andrewlock.net/caching-docker-layers-on-serverless-build-hosts-with-multi-stage-builds---target,-and---cache-from/).
                            sh ( script: "docker image pull ${env.BUILD_IMAGE} || true")
                            sh ( script: "docker image pull ${env.PMEM_CSI_IMAGE} || true")

                            // PR jobs need to use the same CACHEBUST value as the latest build for their
                            // target branch, otherwise they cannot reuse the cached layers. Another advantage
                            // is that they use a version of Clear Linux that is known to work, because "swupd update"
                            // will be cached.
                            env.CACHEBUST = sh ( script: "docker inspect -f '{{ .Config.Labels.cachebust }}' ${env.BUILD_IMAGE} 2>/dev/null || true", returnStdout: true).trim()
                        } else {
                            env.BUILD_IMAGE = "${env.REGISTRY_NAME}/pmem-clearlinux-builder:${env.BRANCH_NAME}-rejected"
                        }

                        if (env.CACHEBUST == null || env.CACHEBUST == "") {
                            env.CACHEBUST = env.BUILD_ID
                        }
                    }
                    sh "env; echo Building BUILD_IMAGE=${env.BUILD_IMAGE} for BUILD_TARGET=${env.BUILD_TARGET}, CHANGE_ID=${env.CHANGE_ID}, CACHEBUST=${env.CACHEBUST}."
                    sh "docker build --cache-from ${env.BUILD_IMAGE} --label cachebust=${env.CACHEBUST} --target build --build-arg CACHEBUST=${env.CACHEBUST} -t ${env.BUILD_IMAGE} ."
                }
            }
        }

        stage('update base image') {
            // Update the base image before doing a full build + test cycle. If that works,
            // we push the new commits to GitHub.
            when { environment name: 'JOB_BASE_NAME', value: 'pmem-csi-release' }

            steps {
                script {
                    status = sh ( script: "docker run --rm ${DockerBuildArgs()} ${env.BUILD_IMAGE} hack/create-new-release.sh", returnStatus: true )
                    if ( status == 2 ) {
                        // https://stackoverflow.com/questions/42667600/abort-current-build-from-pipeline-in-jenkins
                        currentBuild.result = 'ABORTED'
                        error('No new release, aborting...')
                    }
                    if ( status != 0 ) {
                        error("Creating a new release failed.")
                    }
                }
            }
        }

        stage('make test') {
            options {
                timeout(time: 20, unit: "MINUTES")
            }

            steps {
                sh "docker run --rm ${DockerBuildArgs()} ${env.BUILD_IMAGE} make test"
            }
        }

        stage('Build test image') {
            options {
                timeout(time: 60, unit: "MINUTES")
                retry(2)
            }

            steps {
                sh "docker run --rm ${DockerBuildArgs()} ${env.BUILD_IMAGE} make build-images"
            }
        }

        stage('testing 1.16 LVM') {
            options {
                timeout(time: 90, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("lvm", "testing", "fedora", "", "1.16")
            }
        }

        stage('testing 1.16 direct') {
            options {
                timeout(time: 180, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("direct", "testing", "fedora", "", "1.16")
            }
        }

        stage('testing 1.14 LVM') {
            when { not { changeRequest() } }
            options {
                timeout(time: 90, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("lvm", "testing", "fedora", "", "1.14")
            }
        }

        stage('testing 1.14 direct') {
            when { not { changeRequest() } }
            options {
                timeout(time: 180, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("direct", "testing", "fedora", "", "1.14")
            }
        }

        /*
          In production we can only run E2E testing, no sanity testing.
          Therefore it is faster.
        */

        stage('production 1.15, Clear Linux') {
            options {
                timeout(time: 90, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("lvm", "production", "clear", "${env.CLEAR_LINUX_VERSION_1_15}", "")
            }
        }

        stage('production 1.15 LVM') {
            when { not { changeRequest() } }
            options {
                timeout(time: 45, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("lvm", "production", "fedora", "", "1.15")
            }
        }

        stage('production 1.15 direct') {
            when { not { changeRequest() } }
            options {
                timeout(time: 45, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("direct", "production", "fedora", "", "1.15")
            }
        }

        stage('production 1.14 LVM') {
            when { not { changeRequest() } }
            options {
                timeout(time: 45, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("lvm", "production", "fedora", "", "1.14")
            }
        }

        stage('production 1.14 direct') {
            when { not { changeRequest() } }
            options {
                timeout(time: 45, unit: "MINUTES")
                retry(2)
            }
            steps {
                TestInVM("direct", "production", "fedora", "", "1.14")
            }
        }

        stage('Push new release') {
            when {
                environment name: 'JOB_BASE_NAME', value: 'pmem-csi-release'
            }

            steps{
                sshagent(['9b2359bb-540b-4df3-a4b7-d304a426b2db']) {
                    // We build a branch, but have it checked out by commit (detached head).
                    // Therefore we have to specify the branch name explicitly when pushing.
                    sh "git push origin --follow-tags HEAD:${env.BUILD_TARGET}"
                }
            }
        }

        stage('Update master branch') {
            // This stage runs each time "devel" is rebuilt after a merge.
            when {
                environment name: 'BUILD_TARGET', value: 'devel'
                environment name: 'JOB_NAME', value: 'pmem-csi/devel'
            }

            steps{
                sshagent(['9b2359bb-540b-4df3-a4b7-d304a426b2db']) {
                    // All tests have passed on the "devel" branch, we can now fast-forward "master" to it.
                    sh '''
head=$(git rev-parse HEAD) &&
git fetch origin master &&
git checkout FETCH_HEAD &&
git merge --ff-only $head &&
git push origin HEAD:master
'''
                }
            }
        }

        stage('Push images') {
            when {
                not { changeRequest() }
                not { environment name: 'JOB_BASE_NAME', value: 'pmem-csi-release' } // New release will be built and pushed normally.
            }
            steps {
                withDockerRegistry([ credentialsId: "e16bd38a-76cb-4900-a5cb-7f6aa3aeb22d", url: "https://${REGISTRY_NAME}" ]) {
                    // Push PMEM-CSI images without rebuilding them.
                    // When building a tag, we expect the code to contain that version as image version.
                    // When building a branch, we expect "canary" for the "devel" branch and (currently) don't publish
                    // canary images for other branches.
                    sh "imageversion=\$(docker run --rm ${DockerBuildArgs()} ${env.BUILD_IMAGE} make print-image-version) && \
                        expectedversion=\$(echo '${env.BUILD_TARGET}' | sed -e 's/devel/canary/') && \
                        if [ \"\$imageversion\" = \"\$expectedversion\" ] ; then \
                            docker run --rm ${DockerBuildArgs()} -e DOCKER_CONFIG=$DOCKER_CONFIG -v $DOCKER_CONFIG:$DOCKER_CONFIG ${env.BUILD_IMAGE} make push-images PUSH_IMAGE_DEP=; \
                        else \
                            echo \"Skipping the pushing of PMEM-CSI driver images with version \$imageversion because this build is for ${env.BUILD_TARGET}.\"; \
                        fi"
                    // Also push the build image, for later reuse in PR jobs.
                    sh "docker image push ${env.BUILD_IMAGE}"
                }
            }
        }
    }

    post {
        always {
            junit 'build/reports/**/*.xml'
        }
    }
}

/*
 "docker run" parameters which:
 - make the Docker instance on the host available inside a container (socket and command)
 - set common Makefile values (cachebust, cache populated from images if available)
 - source in current directory
 - GOPATH alongside it
 - HOME above it
 - same user inside and outside the container
 - same uid/gid/groups as on the host, plus root=0 for sudo

 "rshared" is needed for mount propagation when govm runs outside the build container.

 A function is used because a variable, even one which uses a closure with lazy evaluation,
 didn't actually result in a string with all variables replaced by the current values.
 Do not use lazy evaluation inside the function, that caused steps which use
 this function to get skipped silently?!
*/
String DockerBuildArgs() {
    "\
    -e BUILD_IMAGE_ID=${env.CACHEBUST} \
    -e 'BUILD_ARGS=--cache-from ${env.BUILD_IMAGE} --cache-from ${env.PMEM_CSI_IMAGE}' \
    -e REGISTRY_NAME=${env.REGISTRY_NAME} \
    -e HOME=`pwd`/.. \
    -e GOPATH=`pwd`/../gopath \
    -e USER=`id -nu` \
    --user `id -u`:`id -g` \
    --group-add `id -G | sed -e 's/ / --group-add /g'` \
    --group-add 0 \
    --volume /var/run/docker.sock:/var/run/docker.sock \
    --volume /usr/bin/docker:/usr/bin/docker \
    --volume `pwd`/..:`pwd`/..:rshared \
    --workdir `pwd` \
    "
}

void TestInVM(deviceMode, deploymentMode, distro, distroVersion, kubernetesVersion) {
    try {
        /*
        We have to run "make start" in the current directory
        because the QEMU instances that it starts under Docker
        run outside of the container and thus paths used inside
        the container have to be the same as outside.

        For "make test_e2e" we then have to switch into the
        GOPATH. Once we can build outside of the GOPATH, we can
        simplify that to build inside one directory.

        For mounting an etcd tmpfs inside the container such that Docker
        on the host and thus QEMU can access it, privileges (for mount)
        and shared mount propagation are needed.

        TODO: test in parallel (on different nodes? single node didn't work,
        https://github.com/intel/pmem-CSI/pull/309#issuecomment-504659383)
        */
        sh " \
           mkdir -p build/reports && \
           if ${env.LOGGING_JOURNALCTL}; then sudo journalctl -f; fi & \
           ( set +x; while true; do sleep ${env.LOGGING_SAMPLING_DELAY}; top -b -n 1 -w 120 | head -n 20; df -h; done ) & \
           docker run --rm \
                  --privileged=true \
                  -e CLUSTER=clear \
                  -e GOVM_YAML=`pwd`/_work/clear/deployment.yaml \
                  -e TEST_BUILD_PMEM_REGISTRY=${env.REGISTRY_NAME} \
                  -e TEST_DEVICEMODE=${deviceMode} \
                  -e TEST_DEPLOYMENTMODE=${deploymentMode} \
                  -e TEST_CREATE_REGISTRY=true \
                  -e TEST_CHECK_SIGNED_FILES=false \
                  -e TEST_DISTRO=${distro} \
                  -e TEST_DISTRO_VERSION=${distroVersion} \
                  -e TEST_KUBERNETES_VERSION=${kubernetesVersion} \
                  -e TEST_ETCD_VOLUME_SIZE=1073741824 \
                  ${DockerBuildArgs()} \
                  ${env.BUILD_IMAGE} \
                  bash -c 'set -x; \
                           testrun=\$(echo '${distro}-${distroVersion}-${kubernetesVersion}-${deviceMode}-${deploymentMode}' | sed -e s/--*/-/g | tr . _ ) && \
                           echo jenkins:x:\$(id -u):\$(id -g):Jenkins:\$(pwd)/..:/bin/bash >>/etc/passwd && \
                           echo jenkins:*:0:0:99999:0::: >>/etc/shadow && \
                           echo jenkins:x:\$(id -g): >>/etc/group && \
                           make start && \
                           _work/clear/ssh.0 kubectl get pods --all-namespaces -o wide && \
                           for pod in ${env.LOGGING_PODS}; do \
                               _work/clear/ssh.0 kubectl logs -f -n kube-system \$pod-pmem-csi-clear-master | sed -e \"s/^/\$pod: /\" & \
                           done && \
                           _work/clear/ssh.0 tar -C / -cf - usr/bin/kubectl | sudo tar -C /usr/local/bin --strip-components=2 -xf - && \
                           for ssh in \$(ls _work/clear/ssh.[0-9]); do \
                               hostname=\$(\$ssh hostname) && \
                               ( set +x; \
                                 if ${env.LOGGING_JOURNALCTL}; then while true; do \$ssh journalctl -f; done; fi & \
                                 while true; do \
                                     sleep ${env.LOGGING_SAMPLING_DELAY}; \
                                     \$ssh top -b -n 1 -w 120 2>&1 | head -n 20; \
                                 done ) | sed -e \"s/^/\$hostname: /\" & \
                           done && \
                           make test_e2e TEST_E2E_REPORT_DIR=`pwd`/build/reports.tmp/\$testrun' \
           "
    } finally {
        // Each test run produces junit_*.xml files with testsuite name="PMEM E2E suite".
        // To make test names unique in the Jenkins UI, we rename that test suite per run,
        // mangle the <testcase name="..." classname="..."> such that
        // Jenkins shows them group as <testrun>/[sanity|E2E]/<test case>,
        // and place files where the 'junit' step above expects them.
        sh '''set -x
            for i in build/reports.tmp/*/*.xml; do
                if [ -f $i ]; then
                    testrun=$(basename $(dirname $i))
                    sed -e "s/PMEM E2E suite/$testrun/" -e 's/testcase name="\\([^ ]*\\) *\\(.*\\)" classname="\\([^"]*\\)"/testcase classname="\\3.\\1" name="\\2"/' $i >build/reports/$testrun.xml
               fi
           done'''

        // Always shut down the cluster to free up resources. As in "make start", we have to expose
        // the path as used on the host also inside the containner, but we don't need to be in it.
        sh "docker run --rm -e CLUSTER=clear ${DockerBuildArgs()} ${env.BUILD_IMAGE} make stop"
    }
}
