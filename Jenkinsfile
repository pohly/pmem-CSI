pipeline {

    agent {

        label "pmem-csi"
    }

    options {

        timeout(time: 2, unit: "HOURS")

    }

    environment {

        /* 29890 broke networking
        (https://github.com/clearlinux/distribution/issues/904). We
        need to stay on the latest known-good version. */
        TEST_CLEAR_LINUX_VERSION = "29880"

        PMEM_PATH = "/go/src/github.com/intel/pmem-csi"
        BUILD_IMAGE = "clearlinux-builder"
        CLUSTER = "ci-clear-govm"
    }

    stages {

        stage('Create build environment') {

            options {

                timeout(time: 60, unit: "MINUTES")

            }

            steps {

                sh 'docker version'
                sh "docker build --target build --build-arg CACHEBUST=${env.BUILD_ID} -t ${env.BUILD_IMAGE} ."

             }

        }

        stage('make test') {

            options {

                timeout(time: 20, unit: "MINUTES")
                retry(3)

            }

            steps {

                sh "docker run --rm \
                -v `pwd`:${env.PMEM_PATH} \
                -w $PMEM_PATH \
                ${env.BUILD_IMAGE} \
                make test"
            }

        }

        stage('Build test image') {

            options {

                timeout(time: 60, unit: "MINUTES")
                retry(2)

            }

            steps {

                sh "docker run --rm \
                    -e BUILD_IMAGE_ID=${env.BUILD_ID} \
                    -v /var/run/docker.sock:/var/run/docker.sock \
                    -v /usr/bin/docker:/usr/bin/docker \
                    -v `pwd`:${env.PMEM_PATH} \
                    -w ${env.PMEM_PATH} \
                    ${env.BUILD_IMAGE} \
                    make build-images"

            }

        }

        stage('E2E') {

            options {

                timeout(time: 90, unit: "MINUTES")
                retry(2)

            }

            steps {
                    /*
                     We have to run "make start" in the current directory
                     because the QEMU instances that it starts under Docker
                     run outside of the container. For "make test_e2e" we
                     then have to switch into the GOPATH. Once we can
                     build outside of the GOPATH, we can simplify that to
                     build inside one directory.
                    */
                    sh "docker run --rm \
                        -e GOVM_YAML=`pwd`/_work/$CLUSTER/deployment.yaml \
                        -e CLUSTER=${env.CLUSTER} \
                        -e TEST_CREATE_REGISTRY=true \
                        -e TEST_CHECK_SIGNED_FILES=false \
                        -e TEST_CLEAR_LINUX_VERSION=${env.TEST_CLEAR_LINUX_VERSION} \
                        -v /var/run/docker.sock:/var/run/docker.sock \
                        -v `pwd`:$PMEM_PATH \
                        -v /usr/bin/docker:/usr/bin/docker \
                        -v `pwd`:`pwd` \
                        -w `pwd` \
                        ${env.BUILD_IMAGE} \
                        bash -c 'swupd bundle-add openssh-server &&  \
                            make start && cd ${env.PMEM_PATH} && \
                            make test_e2e'"

            }

        }

    }
}
