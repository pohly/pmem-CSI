/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Run the csi-test sanity tests against a pmem-csi driver
var _ = Describe("sanity", func() {
	f := framework.NewDefaultFramework("pmem")
	f.SkipNamespaceCreation = true // We don't need a per-test namespace and skipping it makes the tests run faster.

	var (
		cleanup func()
		config  = sanity.Config{
			TestVolumeSize: 1 * 1024 * 1024,
			// The actual directories will be created as unique
			// temp directories inside these directories.
			// We intentionally do not use the real /var/lib/kubelet/pods as
			// root for the target path, because kubelet is monitoring it
			// and deletes all extra entries that it does not know about.
			TargetPath:  "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pmem-sanity-target.XXXXXX",
			StagingPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pmem-sanity-staging.XXXXXX",
		}
	)

	BeforeEach(func() {
		cs := f.ClientSet

		// This test expects that PMEM-CSI was deployed with
		// socat port forwarding enabled (see deploy/kustomize/testing/README.md).
		hosts, err := framework.NodeSSHHosts(cs)
		Expect(err).NotTo(HaveOccurred(), "failed to find external/internal IPs for every node")
		if len(hosts) <= 1 {
			framework.Failf("not enough nodes with external IP")
		}
		// Node #1 is expected to have a PMEM-CSI node driver
		// instance. If it doesn't, connecting to the PMEM-CSI
		// node service will fail.
		host := strings.Split(hosts[1], ":")[0] // Instead of duplicating the NodeSSHHosts logic we simply strip the ssh port.
		config.Address = fmt.Sprintf("dns:///%s:%d", host, 9735)
		config.ControllerAddress = fmt.Sprintf("dns:///%s:%d", host, getServicePort(cs, "pmem-csi-controller-testing"))

		// Wait for socat pod on that node. We need it for
		// creating directories.  We could use the PMEM-CSI
		// node container, but that then forces us to have
		// mkdir and rmdir in that container, which we might
		// not want long-term.
		socat := getAppInstance(cs, "pmem-csi-node-testing", host)

		// Determine how many nodes have the CSI-PMEM running.
		set := getDaemonSet(cs, "pmem-csi-node")

		// We have to ensure that volumes get provisioned on
		// the host were we can do the node operations. We do
		// that by creating cache volumes on each node.
		config.TestVolumeParameters = map[string]string{
			"persistencyModel": "cache",
			"cacheSize":        fmt.Sprintf("%d", set.Status.DesiredNumberScheduled),
		}

		exec := func(args ...string) string {
			// f.ExecCommandInContainerWithFullOutput assumes that we want a pod in the test's namespace,
			// so we have to set one.
			f.Namespace = &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			stdout, stderr, err := f.ExecCommandInContainerWithFullOutput(socat.Name, "socat", args...)
			framework.ExpectNoError(err, "%s in socat container, stderr:\n%s", args, stderr)
			Expect(stderr).To(BeEmpty(), "unexpected stderr from %s in socat container", args)
			return stdout
		}
		mkdir := func(path string) (string, error) {
			return exec("mktemp", "-d", path), nil
		}
		rmdir := func(path string) error {
			exec("rmdir", path)
			return nil
		}

		config.CreateTargetDir = mkdir
		config.CreateStagingDir = mkdir
		config.RemoveTargetPath = rmdir
		config.RemoveStagingPath = rmdir
	})

	AfterEach(func() {
		if cleanup != nil {
			cleanup()
		}
	})
	// This adds several tests that just get skipped.
	// TODO: static definition of driver capabilities (https://github.com/kubernetes-csi/csi-test/issues/143)
	sanity.GinkgoTest(&config)

	Context("node", func() {
		sc := &sanity.SanityContext{
			Config: &config,
		}
		var (
			cs     clientset.Interface
			cl     *sanity.Cleanup
			c      csi.NodeClient
			s, sn  csi.ControllerClient
			nodeID string
		)

		BeforeEach(func() {
			sc.Setup()
			cs = f.ClientSet
			c = csi.NewNodeClient(sc.Conn)
			s = csi.NewControllerClient(sc.ControllerConn)
			sn = csi.NewControllerClient(sc.Conn) // This works because PMEM-CSI exposes the node, controller, and ID server via its csi.sock.
			cl = &sanity.Cleanup{
				Context:                    sc,
				NodeClient:                 c,
				ControllerClient:           s,
				ControllerPublishSupported: true,
				NodeStageSupported:         true,
			}
			nid, err := c.NodeGetInfo(
				context.Background(),
				&csi.NodeGetInfoRequest{})
			framework.ExpectNoError(err, "get node ID")
			nodeID = nid.GetNodeId()
		})

		AfterEach(func() {
			cl.DeleteVolumes()
			sc.Teardown()
		})

		It("stores state across reboots for single volume", func() {
			namePrefix := "state-volume"

			// We intentionally check the state of the controller on the node here.
			// The master caches volumes and does not get rebooted.
			initialVolumes, err := sn.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
			framework.ExpectNoError(err, "list volumes")

			_, vol := createVolume(s, sc, cl, namePrefix)
			createdVolumes, err := sn.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
			Expect(createdVolumes.Entries).To(HaveLen(len(initialVolumes.Entries)+1), "one more volume")

			// Restart.
			restartNode(cs, nodeID)

			// Once we get an answer, it is expected to be the same as before.
			By("checking volumes")
			Eventually(func() bool {
				restartedVolumes, err := sn.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
				if err != nil {
					return false
				}
				Expect(restartedVolumes.Entries).To(ConsistOf(createdVolumes.Entries), "same volumes as before node reboot")
				return true
			}, "5m", "1s").Should(BeTrue(), "list volumes")

			deleteVolume(s, vol)
		})

		It("can mount again after reboot", func() {
			namePrefix := "mount-volume"

			name, vol := createVolume(s, sc, cl, namePrefix)
			// Publish for the second time.
			nodeID := publishVolume(s, c, sc, cl, name, vol)

			// Restart.
			restartNode(cs, nodeID)
			Eventually(func() bool {
				_, err := sn.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
				if err != nil {
					return false
				}
				return true
			}, "5m", "1s").Should(BeTrue(), "node controller running again")

			// No failure, is already unpublished.
			// TODO: In practice this fails with "no mount point specified".
			unpublishVolume(s, c, sc, vol, nodeID)

			// Publish for the second time.
			publishVolume(s, c, sc, cl, name, vol)

			unpublishVolume(s, c, sc, vol, nodeID)
			deleteVolume(s, vol)
		})
	})
})

func getServicePort(cs clientset.Interface, serviceName string) int32 {
	var port int32
	Eventually(func() bool {
		service, err := cs.CoreV1().Services("default").Get(serviceName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		port = service.Spec.Ports[0].NodePort
		return port != 0
	}, "3m").Should(BeTrue(), "%s service running", serviceName)
	return port
}

func getAppInstance(cs clientset.Interface, app string, ip string) *v1.Pod {
	var pod *v1.Pod
	Eventually(func() bool {
		pods, err := cs.CoreV1().Pods("default").List(metav1.ListOptions{})
		if err != nil {
			return false
		}
		for _, p := range pods.Items {
			if p.Labels["app"] == app &&
				(p.Status.HostIP == ip || p.Status.PodIP == ip) {
				pod = &p
				return true
			}
		}
		return false
	}, "3m").Should(BeTrue(), "%s app running on host %s", app, ip)
	return pod
}

func getDaemonSet(cs clientset.Interface, setName string) *appsv1.DaemonSet {
	var set *appsv1.DaemonSet
	Eventually(func() bool {
		s, err := cs.AppsV1().DaemonSets("default").Get(setName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		set = s
		return set != nil
	}, "3m").Should(BeTrue(), "%s pod running", setName)
	return set
}

func createVolume(s csi.ControllerClient, sc *sanity.SanityContext, cl *sanity.Cleanup, namePrefix string) (string, *csi.Volume) {
	var err error
	name := sanity.UniqueString(namePrefix)

	// Create Volume First
	By("creating a single node writer volume")
	vol, err := s.CreateVolume(
		context.Background(),
		&csi.CreateVolumeRequest{
			Name: name,
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Parameters: sc.Config.TestVolumeParameters,
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(vol).NotTo(BeNil())
	Expect(vol.GetVolume()).NotTo(BeNil())
	Expect(vol.GetVolume().GetVolumeId()).NotTo(BeEmpty())
	cl.RegisterVolume(name, sanity.VolumeInfo{VolumeID: vol.GetVolume().GetVolumeId()})

	return name, vol.GetVolume()
}

func publishVolume(s csi.ControllerClient, c csi.NodeClient, sc *sanity.SanityContext, cl *sanity.Cleanup, name string, vol *csi.Volume) string {
	var err error

	By("getting a node id")
	nid, err := c.NodeGetInfo(
		context.Background(),
		&csi.NodeGetInfoRequest{})
	Expect(err).NotTo(HaveOccurred())
	Expect(nid).NotTo(BeNil())
	Expect(nid.GetNodeId()).NotTo(BeEmpty())

	var conpubvol *csi.ControllerPublishVolumeResponse
	By("controller publishing volume")

	conpubvol, err = s.ControllerPublishVolume(
		context.Background(),
		&csi.ControllerPublishVolumeRequest{
			VolumeId: vol.GetVolumeId(),
			NodeId:   nid.GetNodeId(),
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			VolumeContext: vol.GetVolumeContext(),
			Readonly:      false,
			Secrets:       sc.Secrets.ControllerPublishVolumeSecret,
		},
	)
	Expect(err).NotTo(HaveOccurred())
	cl.RegisterVolume(name, sanity.VolumeInfo{VolumeID: vol.GetVolumeId(), NodeID: nid.GetNodeId()})
	Expect(conpubvol).NotTo(BeNil())

	By("node staging volume")
	nodestagevol, err := c.NodeStageVolume(
		context.Background(),
		&csi.NodeStageVolumeRequest{
			VolumeId: vol.GetVolumeId(),
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			StagingTargetPath: sc.StagingPath,
			VolumeContext:     vol.GetVolumeContext(),
			PublishContext:    conpubvol.GetPublishContext(),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(nodestagevol).NotTo(BeNil())

	// NodePublishVolume
	By("publishing the volume on a node")
	nodepubvol, err := c.NodePublishVolume(
		context.Background(),
		&csi.NodePublishVolumeRequest{
			VolumeId:          vol.GetVolumeId(),
			TargetPath:        sc.TargetPath + "/target",
			StagingTargetPath: sc.StagingPath,
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			VolumeContext:  vol.GetVolumeContext(),
			PublishContext: conpubvol.GetPublishContext(),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(nodepubvol).NotTo(BeNil())

	return nid.GetNodeId()
}

func unpublishVolume(s csi.ControllerClient, c csi.NodeClient, sc *sanity.SanityContext, vol *csi.Volume, nodeID string) {
	var err error

	// NodeUnpublishVolume
	By("cleaning up calling nodeunpublish")
	nodeunpubvol, err := c.NodeUnpublishVolume(
		context.Background(),
		&csi.NodeUnpublishVolumeRequest{
			VolumeId:   vol.GetVolumeId(),
			TargetPath: sc.TargetPath + "/target",
		})
	Expect(err).NotTo(HaveOccurred())
	Expect(nodeunpubvol).NotTo(BeNil())

	By("cleaning up calling nodeunstage")
	nodeunstagevol, err := c.NodeUnstageVolume(
		context.Background(),
		&csi.NodeUnstageVolumeRequest{
			VolumeId:          vol.GetVolumeId(),
			StagingTargetPath: sc.StagingPath,
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(nodeunstagevol).NotTo(BeNil())

	By("cleaning up calling controllerunpublishing")
	controllerunpubvol, err := s.ControllerUnpublishVolume(
		context.Background(),
		&csi.ControllerUnpublishVolumeRequest{
			VolumeId: vol.GetVolumeId(),
			NodeId:   nodeID,
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(controllerunpubvol).NotTo(BeNil())
}

func deleteVolume(s csi.ControllerClient, vol *csi.Volume) {
	var err error

	By("cleaning up deleting the volume")
	_, err = s.DeleteVolume(
		context.Background(),
		&csi.DeleteVolumeRequest{
			VolumeId: vol.GetVolumeId(),
		},
	)
	Expect(err).NotTo(HaveOccurred())
}

var unreachable = v1.Taint{Key: "node.kubernetes.io/unreachable", Effect: "NoSchedule"}

// restartNode works only for one of the nodes in the QEMU virtual cluster.
// It does a hard poweroff via SysRq and relies on Docker to restart the
// "failed" node.
func restartNode(cs clientset.Interface, nodeID string) {
	if !regexp.MustCompile(`worker\d+$`).MatchString(nodeID) {
		framework.Skipf("node %q not one of the expected QEMU nodes (worker<number>))", nodeID)
	}
	node := strings.Split(nodeID, "worker")[1]
	ssh := fmt.Sprintf("%s/_work/%s/ssh.%s",
		os.Getenv("REPO_ROOT"),
		os.Getenv("CLUSTER"),
		node)
	out, err := exec.Command(ssh, "uptime", "--since").CombinedOutput()
	framework.ExpectNoError(err, "original %s uptime --since:\n%s", ssh, string(out))
	originalStartTime := string(out)

	// Shutdown via SysRq b (https://major.io/2009/01/29/linux-emergency-reboot-or-shutdown-with-magic-commands/).
	shutdown := exec.Command(ssh)
	shutdown.Stdin = bytes.NewBufferString(`echo 1 > /proc/sys/kernel/sysrq
echo b > /proc/sysrq-trigger`)
	out, err = shutdown.CombinedOutput()
	framework.ExpectNoError(err, "shutdown via %s:\n%s", ssh, string(out))

	// Wait for node to reboot. We know that the node has rebooted once we can log in (again)
	// and it has a different start time than before.
	Eventually(func() bool {
		out, err := exec.Command(ssh, "uptime", "--since").CombinedOutput()
		return err == nil && string(out) != originalStartTime
	}, "5m", "1s").Should(Equal(true), "node up again")
}
