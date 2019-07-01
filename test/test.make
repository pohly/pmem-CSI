TEST_CMD=go test
TEST_ARGS=$(IMPORT_PATH)/pkg/...

.PHONY: vet
test: vet
	go vet $(IMPORT_PATH)/pkg/...

# Check resp. fix formatting.
.PHONY: test_fmt fmt
test: test_fmt
test_fmt:
	@ files=$$(find pkg cmd -name '*.go'); \
	if [ $$(gofmt -d $$files | wc -l) -ne 0 ]; then \
		echo "formatting errors:"; \
		gofmt -d $$files; \
		false; \
	fi

fmt:
	gofmt -l -w $$(find pkg cmd -name '*.go')


# This ensures that the vendor directory and vendor-bom.csv are in sync
# at least as far as the listed components go.
.PHONY: test_vendor_bom
test: test_vendor_bom
test_vendor_bom:
	@ if ! diff -c \
		<(tail -n +2 vendor-bom.csv | sed -e 's/;.*//') \
		<((grep '^  name =' Gopkg.lock  | sed -e 's/.*"\(.*\)"/\1/') | LC_ALL=C LANG=C sort); then \
		echo; \
		echo "vendor-bom.csv not in sync with vendor directory (aka Gopk.lock):"; \
		echo "+ new entry, missing in vendor-bom.csv"; \
		echo "- obsolete entry in vendor-bom.csv"; \
		false; \
	fi

# This ensures that we know about all components that are needed at
# runtime on a production system. Those must be scrutinized more
# closely than components that are merely needed for testing.
#
# Intel has a process for this. The mapping from import path to "name"
# + "download URL" must match how the components are identified at
# Intel while reviewing the components.
.PHONY: test_runtime_deps
test: test_runtime_deps

test_runtime_deps:
	@ if ! diff -c \
		runtime-deps.csv \
		<( $(RUNTIME_DEPS) ); then \
		echo; \
		echo "runtime-deps.csv not up-to-date. Update RUNTIME_DEPS in test/test.make, rerun, review and finally apply the patch above."; \
		false; \
	fi

RUNTIME_DEPS =

# We use "go list" because it is readily available. A good replacement
# would be godeps. We list dependencies recursively, not just the
# direct dependencies.
RUNTIME_DEPS += go list -f '{{ join .Deps "\n" }}' ./cmd/pmem-csi-driver |

# This focuses on packages that are not in Golang core.
RUNTIME_DEPS += grep '^github.com/intel/pmem-csi/vendor/' |

# Filter out some packages that aren't really code.
RUNTIME_DEPS += grep -v -e 'github.com/container-storage-interface/spec' |
RUNTIME_DEPS += grep -v -e 'google.golang.org/genproto/googleapis/rpc/status' |

# Reduce the package import paths to project names + download URL.
# - strip prefix
RUNTIME_DEPS += sed -e 's;github.com/intel/pmem-csi/vendor/;;' |
# - use path inside github.com as project name
RUNTIME_DEPS += sed -e 's;^github.com/\([^/]*\)/\([^/]*\).*;github.com/\1/\2;' |
# - everything from gRPC is one project
RUNTIME_DEPS += sed -e 's;google.golang.org/grpc/*.*;grpc-go,https://github.com/grpc/grpc-go;' |
# - various other projects
RUNTIME_DEPS += sed \
	-e 's;github.com/google/uuid;google uuid,https://github.com/google/uuid;' \
	-e 's;github.com/golang/protobuf;golang-protobuf,https://github.com/golang/protobuf;' \
	-e 's;github.com/gogo/protobuf;gogo protobuf,https://github.com/gogo/protobuf;' \
	-e 's;github.com/golang/glog;glog,https://github.com/golang/glog;' \
	-e 's;github.com/pkg/errors;pkg/errors,https://github.com/pkg/errors;' \
	-e 's;github.com/vgough/grpc-proxy;grpc-proxy,https://github.com/vgough/grpc-proxy;' \
	-e 's;golang.org/x/.*;Go,https://github.com/golang/go,9051;' \
	-e 's;k8s.io/.*\|github.com/kubernetes-csi/.*;kubernetes,https://github.com/kubernetes/kubernetes,9641;' \
	-e 's;gopkg.in/fsnotify.*;golang-github-fsnotify-fsnotify,https://github.com/fsnotify/fsnotify;' \
	| cat |

# Ignore duplicates.
RUNTIME_DEPS += LC_ALL=C LANG=C sort -u

# E2E tests which are known to be unsuitable (space separated list of regular expressions).
TEST_E22_SKIP = no-such-test

# The test's check whether a driver supports multiple nodes is incomplete and does
# not work for the topology-based single-node access in PMEM-CSI:
# https://github.com/kubernetes/kubernetes/blob/25ffbe633810609743944edd42d164cd7990071c/test/e2e/storage/testsuites/provisioning.go#L175-L181
TEST_E22_SKIP += should.access.volume.from.different.nodes

empty:=
space:= $(empty) $(empty)

# E2E testing relies on a running QEMU test cluster. It therefore starts it,
# but because it might have been running already and might have to be kept
# running to debug test failures, it doesn't stop it.
# Use count=1 to avoid test results caching, does not make sense for e2e test.
.PHONY: test_e2e
RUN_E2E = KUBECONFIG=`pwd`/_work/$(CLUSTER)/kube.config \
	REPO_ROOT=`pwd` \
	TEST_DEPLOYMENTMODE=$(shell source test/test-config.sh; echo $$TEST_DEPLOYMENTMODE) \
	go test -count=1 -timeout 0 -v ./test/e2e -ginkgo.skip='$(subst $(space),|,$(TEST_E22_SKIP))'
test_e2e: start
	$(RUN_E2E)

# Execute simple unit tests.
.PHONY: run_tests
test: run_tests
RUN_TESTS = TEST_WORK=$(abspath _work) \
	$(TEST_CMD) $(shell go list $(TEST_ARGS) | sed -e 's;$(IMPORT_PATH);.;')
run_tests: _work/pmem-ca/.ca-stamp _work/evil-ca/.ca-stamp
	$(RUN_TESTS)

_work/%/.ca-stamp: test/setup-ca.sh _work/.setupcfssl-stamp
	rm -rf $(@D)
	WORKDIR='$(@D)' PATH='$(PWD)/_work/bin/:$(PATH)' CA='$*' EXTRA_CNS="wrong-node-controller" $<
	touch $@

_work/.setupcfssl-stamp:
	rm -rf _work/bin
	curl -L https://pkg.cfssl.org/R1.2/cfssl_linux-amd64 -o _work/bin/cfssl --create-dirs
	curl -L https://pkg.cfssl.org/R1.2/cfssljson_linux-amd64 -o _work/bin/cfssljson --create-dirs
	chmod a+x _work/bin/cfssl _work/bin/cfssljson
	touch $@

# Build gocovmerge at a certain revision. Depends on go >= 1.11
# because we use module support.
GOCOVMERGE_VERSION=b5bfa59ec0adc420475f97f89b58045c721d761c
_work/gocovmerge-$(GOCOVMERGE_VERSION):
	tmpdir=`mktemp -d` && \
	trap 'rm -r $$tmpdir' EXIT && \
	cd $$tmpdir && \
	echo "module foo" >go.mod && \
	go get github.com/wadey/gocovmerge@$(GOCOVMERGE_VERSION) && \
	go build -o $(abspath $@) github.com/wadey/gocovmerge
	ln -sf $(@F) _work/gocovmerge

# This is a special target that runs unit and E2E testing and
# combines the various cover profiles into one. To re-run testing,
# remove the file or use "make coverage".
#
# We remove all pmem-csi-driver coverage files
# before testing, restart the driver, and then collect all
# files, including the ones written earlier by init containers.
_work/coverage.out: _work/gocovmerge-$(GOCOVMERGE_VERSION)
	$(MAKE) start
	@ echo "removing old pmem-csi-driver coverage information from all nodes"
	@ for ssh in _work/$(CLUSTER)/ssh.*; do for i in $$($$ssh ls /var/lib/pmem-csi-coverage/pmem-csi-driver* 2>/dev/null); do (set -x; $$ssh rm $$i); done; done
	@ rm -rf _work/coverage
	@ mkdir _work/coverage
	@ go clean -testcache
	$(subst go test,go test -coverprofile=$(abspath _work/coverage/unit.out) -covermode=atomic,$(RUN_TESTS))
	$(RUN_E2E)
	@ echo "killing pmem-csi-driver to flush coverage data"
	@ for ssh in _work/$(CLUSTER)/ssh.*; do (set -x; $$ssh killall pmem-csi-driver); done
	@ echo "waiting for all pods to restart"
	@ while _work/$(CLUSTER)/ssh.0 kubectl get --no-headers pods | grep -q -v Running; do sleep 5; done
	@ echo "collecting coverage data"
	@ for ssh in _work/$(CLUSTER)/ssh.*; do for i in $$($$ssh ls /var/lib/pmem-csi-coverage/ 2>/dev/null); do (set -x; $$ssh cat /var/lib/pmem-csi-coverage/$$i) >_work/coverage/$$(echo $$ssh | sed -e 's;.*/ssh\.;;').$$i; done; done
	$< _work/coverage/* >$@

_work/coverage.html: _work/coverage.out
	go tool cover -html $< -o $@

_work/coverage.txt: _work/coverage.out
	go tool cover -func $< -o $@

.PHONY: coverage
coverage:
	@ rm -rf _work/coverage.out
	$(MAKE) _work/coverage.txt _work/coverage.html
