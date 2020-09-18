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
        CLEAR_LINUX_VERSION_1_17 = "32690" // current release at the moment
        CLEAR_LINUX_VERSION_1_16 = "31760"
        CLEAR_LINUX_VERSION_1_15 = "31070"

        REGISTRY_NAME = "cloud-native-image-registry.westus.cloudapp.azure.com"

        // Per-branch build environment, marked as "do not promote to public registry".
        // Set below via a script, must *not* be set here as it can't be overwritten.
        // BUILD_IMAGE = ""

        // A running container based on BUILD_IMAGE, with volumes for everything that we
        // need from the build host.
        BUILD_CONTAINER = "builder"

        // Tag or branch name that is getting built, depending on the job.
        // Set below via a script, must *not* be set here as it can't be overwritten.
        // BUILD_TARGET = ""

        // This image is pulled at the beginning and used as cache.
        // TODO: Here we use "canary" which is correct for the "devel" branch, but other
        // branches may need something else to get better caching.
        PMEM_CSI_IMAGE = "${env.REGISTRY_NAME}/pmem-csi-driver:canary"

        // A file stored on a sufficiently large tmpfs for use as etcd volume
        // and its size. It has to be inside the data directory of the master node.
        CLUSTER = "govm"
        TEST_ETCD_TMPFS = "${WORKSPACE}/_work/${env.CLUSTER}/data/pmem-csi-${env.CLUSTER}-master/etcd-tmpfs"
        TEST_ETCD_VOLUME = "${env.TEST_ETCD_TMPFS}/etcd-volume"
        TEST_ETCD_VOLUME_SIZE = "1073741824" // 1GB
    }

    stages {
        stage('Create build environment') {
            options {
                timeout(time: 60, unit: "MINUTES")
            }

            steps {
                SetupHost()

                withDockerRegistry([ credentialsId: "${env.DOCKER_REGISTRY}", url: "https://${REGISTRY_NAME}" ]) {
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

                    PrepareEnv()
                }
            }
        }

        stage('update base image') {
            // Update the base image before doing a full build + test cycle. If that works,
            // we push the new commits to GitHub.
            when { environment name: 'JOB_BASE_NAME', value: 'pmem-csi-release' }

            steps {
                script {
                    status = sh ( script: "${RunInBuilder()} ${env.BUILD_CONTAINER} hack/create-new-release.sh", returnStatus: true )
                    if ( status == 2 ) {
                        // https://stackoverflow.com/questions/42667600/abort-current-build-from-pipeline-in-jenkins
                        currentBuild.result = 'ABORTED'
                        error('No new release, aborting...')
                    }
                    if ( status != 0 ) {
                        error("Creating a new release failed.")
                    }
                }
                // We must ensure that the workers use the same modified source code.
                // This relies on create-new-release.sh producing just a single commit.
                sh "git format-patch -n1 --stdout >_work/release.patch"
            }
        }

        stage('docsite') {
            steps {
                sh "${RunInBuilder()} ${env.BUILD_CONTAINER} env GITHUB_SHA=${GIT_COMMIT} GITHUB_REPOSITORY=${SourceRepo()} make vhtml"
                publishHTML([allowMissing: false, alwaysLinkToLastBuild: false, keepAll: false, reportDir: '_output/html', reportFiles: 'index.html', reportName: 'Doc Site', reportTitles: ''])
            }
        }

        stage('make test') {
            options {
                timeout(time: 40, unit: "MINUTES")
            }

            steps {
                sh "${RunInBuilder()} ${env.BUILD_CONTAINER} make test"
            }
        }

        stage('Build test image') {
            options {
                timeout(time: 60, unit: "MINUTES")
            }

            steps {
                // This builds images for REGISTRY_NAME with the version automatically determined by
                // the make rules.
                sh "${RunInBuilder()} ${env.BUILD_CONTAINER} make build-images"

                // For testing we have to have those same images also in a registry. Tag and push for
                // localhost, which is the default test registry.
                sh "imageversion=\$(${RunInBuilder()} ${env.BUILD_CONTAINER} make print-image-version) && \
                    for suffix in '' '-test'; do \
                        docker tag ${env.REGISTRY_NAME}/pmem-csi-driver\$suffix:\$imageversion localhost:5000/pmem-csi-driver\$suffix:\$imageversion && \
                        docker push localhost:5000/pmem-csi-driver\$suffix:\$imageversion; \
                    done"
            }
        }

        // In order to enable running on additional Jenkins workers in parallel, we
        // need to save and stash the images, then (if needed for a new worker) restore
        // the build environment.
        //
        // lz4 is used because compression with gzip slowed down creating the archive too much.
        //
        // Alternatively, we could transmit images through the shared registry, but then would
        // need to solve assigning a per-job tag and garbage collection of those images.
        stage('Stash images') {
            steps {
                sh "imageversion=\$(${RunInBuilder()} ${env.BUILD_CONTAINER} make print-image-version) && \
                    docker save localhost:5000/pmem-csi-driver:\$imageversion \
                                localhost:5000/pmem-csi-driver-test:\$imageversion \
                                ${env.BUILD_IMAGE} | \
                           lz4 > _work/images.tar.lz4 && \
                    ls -l -h _work/images.tar.lz4"
                stash includes: '_work/images.tar.lz4', name: 'images'
                stash includes: '_work/release.patch', name: 'release', allowEmpty: true
            }
        }

        // Some stages are skipped entirely when testing PRs, the
        // others skip certain tests in that case:
        // - production deployment is only tested with Kubernetes 1.16
        //   and testing deployment only with Kubernetes 1.18
        stage('Testing') {
            parallel {
                // This runs most tests and thus gets to use the initial worker immediately.
                stage('1.19') {
                    options {
                        timeout(time: 720, unit: "MINUTES")
                    }
                    steps {
                        TestInVM("", "fedora", "", "1.19", "Top.Level..[[:alpha:]]*-production[[:space:]]")
                    }
                }

                // All others set up their own worker.
                stage('1.18') {
                    when { not { changeRequest() } }
                    options {
                        timeout(time: 720, unit: "MINUTES")
                    }
                    agent {
                        label "pmem-csi"
                    }
                    steps {
                        TestInVM("fedora-1.18", "fedora", "", "1.18", "")
                    }
                }
                stage('1.17') {
                    options {
                        timeout(time: 720, unit: "MINUTES")
                    }
                    agent {
                        label "pmem-csi"
                    }
                    steps {
                        TestInVM("fedora-1.17", "fedora", "", "1.17", "Top.Level..[[:alpha:]]*-testing[[:space:]]")
                    }
                }

                // Disabled because of stability issues:
                // - https://github.com/clearlinux/distribution/issues/2007
                // - https://github.com/clearlinux/distribution/issues/1980
                // stage('Clear Linux, 1.17') {
                //     options {
                //         timeout(time: 240, unit: "MINUTES")
                //     }
                //     agent {
                //         label "pmem-csi"
                //     }
                //     steps {
                //         TestInVM("clear-1.17", "clear", "${env.CLEAR_LINUX_VERSION_1_17}", "",  "Top.Level..[[:alpha:]]*-testing[[:space:]]")
                //     }
                // }
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

        // Pushing images uses the DOCKER_CONFIG set up inside the build container earlier.
        stage('Push images') {
            when {
                not { changeRequest() }
                not { environment name: 'JOB_BASE_NAME', value: 'pmem-csi-release' } // New release will be built and pushed normally.
            }
            steps {
                // Push PMEM-CSI images without rebuilding them.
                //
                // When building a tag, we expect the code to contain that version as image version.
                // When building a branch, we expect "canary" for the "devel" branch and (currently) don't publish
                // canary images for other branches.
                sh "imageversion=\$(${RunInBuilder()} ${env.BUILD_CONTAINER} make print-image-version) && \
                    expectedversion=\$(echo '${env.BUILD_TARGET}' | sed -e 's/devel/canary/') && \
                    if [ \"\$imageversion\" = \"\$expectedversion\" ] ; then \
                        ${RunInBuilder()} ${env.BUILD_CONTAINER} make push-images PUSH_IMAGE_DEP=; \
                    else \
                        echo \"Skipping the pushing of PMEM-CSI driver images with version \$imageversion because this build is for ${env.BUILD_TARGET}.\"; \
                    fi"
                // Also push the build image, for later reuse in PR jobs.
                sh "${RunInBuilder()} ${env.BUILD_CONTAINER} docker image push ${env.BUILD_IMAGE}"
            }
        }
    }
}

/*
 A command line for running some command inside the build container with:
 - common Makefile values (cachebust, cache populated from images if available) in environment
 - source in current directory
 - GOPATH alongside it
 - HOME above it
 - same uid as on the host, gid same as for Docker socket

 Using the same uid/gid and auxiliary groups would be nicer, but "docker exec" does not
 support --group-add.

 A function is used because a variable, even one which uses a closure with lazy evaluation,
 didn't actually result in a string with all variables replaced by the current values.
 Do not use lazy evaluation inside the function, that caused steps which use
 this function to get skipped silently?!
*/
String RunInBuilder() {
    "\
    docker exec \
    -e BUILD_IMAGE_ID=${env.CACHEBUST} \
    -e 'BUILD_ARGS=--cache-from ${env.BUILD_IMAGE} --cache-from ${env.PMEM_CSI_IMAGE}' \
    -e DOCKER_CONFIG=${WORKSPACE}/_work/docker-config \
    -e REGISTRY_NAME=${env.REGISTRY_NAME} \
    -e HOME=${WORKSPACE}/.. \
    -e GOPATH=${WORKSPACE}/../gopath \
    -e USER=`id -nu` \
    --user `id -u`:`stat --format %g /var/run/docker.sock` \
    --workdir ${WORKSPACE} \
    "
}

/*
 Returns <owner>/<repo> from which the code was built.
*/
String SourceRepo() {
    // Content of CHANGE_FORK varies, see https://issues.jenkins-ci.org/browse/JENKINS-58450.
    (! env.CHANGE_FORK) ?
        "github.com/intel/pmem-csi" :
    env.CHANGE_FORK.matches('.*/.*') ?
        env.CHANGE_FORK :
        env.CHANGE_FORK + '/pmem-csi'
}

/*
 Dump and/or change the configuration of the host on which the agent runs.
*/
void SetupHost() {
     sh '''
         hostname
         docker version
         git version
         free
         command -v top >/dev/null 2>&1 ||
            if command -v apt-get >/dev/null 2>&1; then
                sudo apt-get install procps
            else
                sudo dnf -y install procps
            fi
         head -n 30 /proc/cpuinfo; echo ...; tail -n 30 /proc/cpuinfo
         git remote set-url origin git@github.com:intel/pmem-csi.git
         git config user.name 'Intel Kubernetes CI/CD Bot'
         git config user.email 'k8s-bot@intel.com'
    '''

    // known_hosts entry created and verified as described in https://serverfault.com/questions/856194/securely-add-a-host-e-g-github-to-the-ssh-known-hosts-file
    sh "mkdir -p ~/.ssh && echo 'github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7hRGmdnm9tUDbO9IDSwBK6TbQa+PXYPCPy6rbTrTtw7PHkccKrpp0yVhp5HdEIcKr6pLlVDBfOLX9QUsyCOV0wzfjIJNlGEYsdlLJizHhbn2mUjvSAHQqZETYP81eFzLQNnPHt4EVVUh7VfDESU84KezmD5QlWpXLmvU31/yMf+Se8xhHTvKSCZIFImWwoG6mbUoWf9nzpIoaSjB+weqqUUmpaaasXVal72J+UX2B+2RPW3RcT0eOzQgqlJL3RKrTJvdsjE3JEAvGq3lGHSZXy28G3skua2SmVi/w4yCE6gbODqnTWlg7+wC604ydGXA8VJiS5ap43JXiUFFAaQ==' >>~/.ssh/known_hosts && chmod -R go-rxw ~/.ssh"
}

/*
 Set up build container and Docker registry.
 Must be called after the build image is ready.
*/
void PrepareEnv() {
    // Create a tmpfs for use as backing store for a large file that will be passed
    // into QEMU for storing the etcd database.
    sh """
        mkdir -p '${env.TEST_ETCD_TMPFS}'
        sudo mount -osize=${env.TEST_ETCD_VOLUME_SIZE} -t tmpfs none '${env.TEST_ETCD_TMPFS}'
        sudo truncate --size=${env.TEST_ETCD_VOLUME_SIZE} '${env.TEST_ETCD_VOLUME}'
    """

    // Create a running container (https://stackoverflow.com/a/38308399). We keep it running
    // and just "docker exec" commands in it. withDockerRegistry creates the DOCKER_CONFIG directory
    // and deletes it when done, so we have to make a copy for later use inside the container.
    withDockerRegistry([ credentialsId: "${env.DOCKER_REGISTRY}", url: "https://${REGISTRY_NAME}" ]) {
        sh "mkdir -p _work"
        sh "cp -a $DOCKER_CONFIG _work/docker-config"
        sh "docker create --name=${env.BUILD_CONTAINER} \
                   --volume /var/run/docker.sock:/var/run/docker.sock \
                   --volume /usr/bin/docker:/usr/bin/docker \
                   --volume ${WORKSPACE}/..:${WORKSPACE}/.. \
                   ${env.BUILD_IMAGE} \
                   sleep infinity"
    }
    sh "docker start ${env.BUILD_CONTAINER} && \
        timeout=0; \
        while [ \$(docker inspect --format '{{.State.Status}}' ${env.BUILD_CONTAINER}) != running ]; do \
            docker ps; \
            if [ \$timeout -ge 60 ]; then \
               docker inspect ${env.BUILD_CONTAINER}; \
               echo 'ERROR: ${env.BUILD_CONTAINER} container still not running'; \
               exit 1; \
            fi; \
            sleep 10; \
            timeout=\$((timeout + 10)); \
       done"

    // Install additional tools:
    // - ssh client for govm
    // - python3 for Sphinx (i.e. make html)
    // - parted, xfsprogs, os-cloudguest-aws (contains mkfs.ext4) for ImageFile test
    sh "docker exec ${env.BUILD_CONTAINER} swupd bundle-add openssh-client python3-basic parted xfsprogs os-cloudguest-aws"

    // Now commit those changes to ensure that the result of "swupd bundle add" gets cached.
    sh "docker commit ${env.BUILD_CONTAINER} ${env.BUILD_IMAGE}"

    // Make /usr/local/bin writable for all users. Used to install kubectl.
    sh "docker exec ${env.BUILD_CONTAINER} sh -c 'mkdir -p /usr/local/bin && chmod a+wx /usr/local/bin'"

    // Some tools expect a user entry for the jenkins user (like govm?)
    sh "echo jenkins:x:`id -u`:0:Jenkins:${WORKSPACE}/..:/bin/bash | docker exec -i ${env.BUILD_CONTAINER} tee --append /etc/passwd >/dev/null"
    sh "echo 'jenkins:*:0:0:99999:0:::' | docker exec -i ${env.BUILD_CONTAINER} tee --append /etc/shadow >/dev/null"

    // Verify that docker works in the updated image.
    sh "${RunInBuilder()} ${env.BUILD_CONTAINER} docker ps"

    // Run a per-test registry on the build host.  This is where we
    // will push images for use by the cluster during testing.
    sh "docker run -d -p 5000:5000 --restart=always --name registry registry:2"
}

/*
 Must be called on additional agents to replicate the environment on the main agent.
*/
void RestoreEnv() {
     SetupHost()

    // Get images, ready for use and/or pushing to localhost:5000.
    unstash 'images'
    sh 'lz4cat _work/images.tar.lz4 | docker load'

    // In case of a release update, also apply the same source code patch.
    // Does not exist during normal PR testing.
    unstash 'release'
    sh 'if [ -f _work/release.patch ]; then git am _work/release.patch; fi'

    // Set up build container and registry.
    PrepareEnv()

    // Now populate the registry like we did on the master node.
    sh "imageversion=\$(${RunInBuilder()} ${env.BUILD_CONTAINER} make print-image-version) && \
        for suffix in '' '-test'; do \
            docker push localhost:5000/pmem-csi-driver\$suffix:\$imageversion; \
        done"
}

void TestInVM(worker, distro, distroVersion, kubernetesVersion, skipIfPR) {
    if (worker) {
        RestoreEnv()
    }
    try {
        /*
        We have to run "make start" in the current directory
        because the QEMU instances that it starts under Docker
        run outside of the container and thus paths used inside
        the container have to be the same as outside.

        For "make test_e2e" we then have to switch into the
        GOPATH. Once we can build outside of the GOPATH, we can
        simplify that to build inside one directory.

        This spawns some long running processes. Those do not killed when the
        main process returns when using "docker exec", so we should better clean
        up ourselves. "make stop" was hanging and waiting for these processes to
        exit even though there were from a different "docker exec" invocation.

        The default QEMU cpu enables nested virtualization with "-cpu
        host".  However, that fails on some Azure machines
        (`qemu-system-x86_64: error: failed to set MSR 0x48b to
        0x1582e00000000`,
        https://www.mail-archive.com/qemu-devel@nongnu.org/msg665051.html),
        so for now we disable VMX with -vmx.

        TODO: test in parallel (on different nodes? single node didn't work,
        https://github.com/intel/pmem-CSI/pull/309#issuecomment-504659383)
        */
        sh " \
           loggers=; \
           atexit () { set -x; kill \$loggers; killall sleep; }; \
           trap atexit EXIT; \
           mkdir -p build/reports && \
           if ${env.LOGGING_JOURNALCTL}; then sudo journalctl -f; fi & \
           ( set +x; while sleep ${env.LOGGING_SAMPLING_DELAY}; do top -i -b -n 1 -w 120; df -h; done ) & \
           loggers=\"\$loggers \$!\" && \
           ${RunInBuilder()} \
                  -e CLUSTER=${env.CLUSTER} \
                  -e TEST_LOCAL_REGISTRY=\$(ip addr show dev docker0 | grep ' inet ' | sed -e 's/.* inet //' -e 's;/.*;;'):5000 \
                  -e TEST_CHECK_SIGNED_FILES=false \
                  -e TEST_CHECK_KVM=false \
                  -e TEST_QEMU_CPU=host,-vmx \
                  -e TEST_DISTRO=${distro} \
                  -e TEST_DISTRO_VERSION=${distroVersion} \
                  -e TEST_KUBERNETES_VERSION=${kubernetesVersion} \
                  -e TEST_ETCD_VOLUME=${env.TEST_ETCD_VOLUME} \
                  ${env.BUILD_CONTAINER} \
                  bash -c 'set -x; \
                           loggers=; \
                           atexit () { set -x; kill \$loggers; kill \$( ps --no-header -o %p ); }; \
                           trap atexit EXIT; \
                           make stop && \
                           make start && \
                           _work/${env.CLUSTER}/ssh.0 kubectl get pods --all-namespaces -o wide && \
                           for pod in ${env.LOGGING_PODS}; do \
                               _work/${env.CLUSTER}/ssh.0 kubectl logs -f -n kube-system \$pod-pmem-csi-${env.CLUSTER}-master | sed -e \"s/^/\$pod: /\" & \
                               loggers=\"\$loggers \$!\"; \
                           done && \
                           _work/${env.CLUSTER}/ssh.0 tar -C / -cf - usr/bin/kubectl | tar -C /usr/local/bin --strip-components=2 -xf - && \
                           for ssh in \$(ls _work/${env.CLUSTER}/ssh.[0-9]); do \
                               hostname=\$(\$ssh hostname) && \
                               if ${env.LOGGING_JOURNALCTL}; then \
                                   ( set +x; while true; do \$ssh journalctl -f; done ) & \
                                   loggers=\"\$loggers \$!\"; \
                               fi; \
                               ( set +x; \
                                 while sleep ${env.LOGGING_SAMPLING_DELAY}; do \
                                     \$ssh top -i -b -n 1 -w 120 2>&1; \
                                 done | sed -e \"s/^/\$hostname: /\" ) & \
                               loggers=\"\$loggers \$!\"; \
                           done && \
                           testrun=\$(echo '${distro}-${distroVersion}-${kubernetesVersion}' | sed -e s/--*/-/g | tr . _ ) && \
                           make test_e2e TEST_E2E_REPORT_DIR=${WORKSPACE}/build/reports.tmp/\$testrun \
                                         TEST_E2E_SKIP=\$(if [ \"${env.CHANGE_ID}\" ] && [ \"${env.CHANGE_ID}\" != null ]; then echo \\\\[Slow\\\\]@${skipIfPR}; fi) \
                           ' \
           "
    } catch (exc) {
        echo "Handling exception, get pod state and kubelet logs:"
        sh "_work/${env.CLUSTER}/ssh.0 kubectl get pods --all-namespaces -o wide"
        sh "for cmd in `ls _work/${env.CLUSTER}/ssh.*`; do \$cmd sudo journalctl -u kubelet; done"
        // regular error handling
        throw exc
    } finally {
        // Each test run produces junit_*.xml files with testsuite name="PMEM E2E suite".
        // To make test names unique in the Jenkins UI, we rename that test suite per run,
        // mangle the <testcase name="..." classname="..."> such that
        // Jenkins shows them group as <testrun>/[sanity|E2E]/<test case>,
        // and place files where the 'junit' step above expects them.
        //
        // Example input and output (note that "gotests" only has two words in the name, not three,
        // to prevent it from being listed under "direct-testing"):
        //
        // <       <testcase name="direct-production E2E [Driver: direct-production-pmem-csi] [Testpattern: Dynamic PV (ntfs)][sig-windows] provisioning should provision storage with mount options" classname="PMEM E2E suite" time="0.021836673">
        // >       <testcase name="[Driver: direct-production-pmem-csi] [Testpattern: Dynamic PV (ntfs)][sig-windows] provisioning should provision storage with mount options" classname="fedora-1_16.direct-production.E2E" time="0.021836673">
        //
        // <       <testcase name="direct-testing-gotests ./pkg/pmem-csi-driver" classname="PMEM E2E suite" time="69.389477842"></testcase>
        // >       <testcase name="./pkg/pmem-csi-driver" classname="fedora-1_16.direct-production-gotests" time="69.389477842"></testcase>
        sh '''set -x
            for i in build/reports.tmp/*/*.xml; do
                if [ -f $i ]; then
                    testrun=$(basename $(dirname $i))
                    sed -e "s/PMEM E2E suite/$testrun/" \
                        -e 's/testcase name="\\([^ ]*\\) \\([^ ]*\\) \\(..*\\)" classname="\\([^"]*\\)"/testcase classname="\\4.\\1.\\2" name="\\3"/' \
                        -e 's/testcase name="\\([^ ]*\\) \\(..*\\)" classname="\\([^"]*\\)"/testcase classname="\\3.\\1" name="\\2"/' \
                        $i >build/reports/$testrun.xml
                    diff $i build/reports/$testrun.xml || true
               fi
           done'''
        junit 'build/reports/**/*.xml'
    }
}
