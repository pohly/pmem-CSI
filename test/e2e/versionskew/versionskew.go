/*
Copyright 2020 Intel Corporation.

SPDX-License-Identifier: Apache-2.0
*/

/* Version skew testing ensures that APIs and state is compatible
across up- and downgrades. The driver for older releases is installed
by checking out the deployment YAML files from an older release.

The operator is not covered yet.
*/
package versionskew

import (
	"context"
	"fmt"

	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"

	"github.com/intel/pmem-csi/pkg/pmem-csi-driver/parameters"
	"github.com/intel/pmem-csi/test/e2e/deploy"
	"github.com/intel/pmem-csi/test/e2e/driver"
	"github.com/intel/pmem-csi/test/e2e/storage/dax"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2estatefulset "k8s.io/kubernetes/test/e2e/framework/statefulset"
	e2evolume "k8s.io/kubernetes/test/e2e/framework/volume"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	base = "0.7"
)

type skewTestSuite struct {
	tsInfo testsuites.TestSuiteInfo
}

var _ testsuites.TestSuite = &skewTestSuite{}

var (
	// The version skew tests run with combinations of the
	// following volume parameters.
	fsTypes       = []string{"", "ext4"}
	volTypes      = []testpatterns.TestVolType{testpatterns.CSIInlineVolume, testpatterns.DynamicPV}
	volParameters = []map[string]string{
		nil,
		{
			string(parameters.CacheSize):        "2",
			string(parameters.PersistencyModel): string(parameters.PersistencyCache),
		},
	}
	volModes = []v1.PersistentVolumeMode{
		v1.PersistentVolumeFilesystem,
		v1.PersistentVolumeBlock,
	}
)

// InitSkewTestSuite dynamically generates testcases for version skew testing.
// Each test case represents a certain kind of volume supported by PMEM-CSI.
func InitSkewTestSuite() testsuites.TestSuite {
	suite := &skewTestSuite{
		tsInfo: testsuites.TestSuiteInfo{
			Name: "skew",
		},
	}

	haveCSIInline := false
	haveBlock := false
	for _, volType := range volTypes {
		for _, fs := range fsTypes {
			for _, parameters := range volParameters {
				scp := driver.StorageClassParameters{
					FSType:     fs,
					Parameters: parameters,
				}
				for _, volMode := range volModes {
					pattern := testpatterns.TestPattern{
						Name:    driver.EncodeTestPatternName(volType, volMode, scp),
						VolType: volType,
						VolMode: volMode,
						FsType:  fs,
					}
					if volType == testpatterns.CSIInlineVolume {
						if haveCSIInline {
							// Only generate a single test pattern for inline volumes
							// because we don't want the number of testcases to explode.
							continue
						}
						haveCSIInline = true
					}
					if volMode == v1.PersistentVolumeBlock {
						if haveBlock {
							// Same for raw block.
							continue
						}
						haveBlock = true
					}
					suite.tsInfo.TestPatterns = append(suite.tsInfo.TestPatterns, pattern)
				}
			}
		}
	}

	return suite
}

func (p *skewTestSuite) GetTestSuiteInfo() testsuites.TestSuiteInfo {
	return p.tsInfo
}

func (p *skewTestSuite) SkipRedundantSuite(driver testsuites.TestDriver, pattern testpatterns.TestPattern) {
}

type local struct {
	config      *testsuites.PerTestConfig
	testCleanup func()

	unused, usedBefore, usedAfter *testsuites.VolumeResource
}

func (p *skewTestSuite) DefineTests(driver testsuites.TestDriver, pattern testpatterns.TestPattern) {
	var l local

	f := framework.NewDefaultFramework("skew")

	init := func(all bool) {
		l = local{}
		l.config, l.testCleanup = driver.PrepareTest(f)

		// Now do the more expensive test initialization. We potentially create more than one
		// storage class, so each resource needs a different prefix.
		l.unused = createVolumeResource(driver, l.config, "-unused", pattern)
		if all {
			l.usedBefore = createVolumeResource(driver, l.config, "-before", pattern)
			l.usedAfter = createVolumeResource(driver, l.config, "-after", pattern)
		}
	}

	cleanup := func() {
		if l.unused != nil {
			l.unused.CleanupResource()
			l.unused = nil
		}

		if l.usedBefore != nil {
			l.usedBefore.CleanupResource()
			l.usedBefore = nil
		}

		if l.usedAfter != nil {
			l.usedAfter.CleanupResource()
			l.usedAfter = nil
		}

		if l.testCleanup != nil {
			l.testCleanup()
			l.testCleanup = nil
		}
	}

	testVersionChange := func(otherDeploymentName string) {
		withKataContainers := false

		// Create volumes.
		init(true)
		defer cleanup()

		// Use some volume before the up- or downgrade
		By(fmt.Sprintf("creating pod before switching driver to %s", otherDeploymentName))
		podBefore := dax.CreatePod(f, "pod-before-test", l.usedBefore.Pattern.VolMode, l.usedBefore.VolSource, l.config, withKataContainers)

		// Change driver releases.
		By(fmt.Sprintf("switch driver to %s", otherDeploymentName))
		deployment, err := deploy.Parse(otherDeploymentName)
		if err != nil {
			framework.Failf("internal error while parsing %s: %v", otherDeploymentName, err)
		}
		deploy.EnsureDeploymentNow(f, deployment)

		// Use some other volume.
		By(fmt.Sprintf("creating pod after switching driver to %s", otherDeploymentName))
		podAfter := dax.CreatePod(f, "pod-after-test", l.usedAfter.Pattern.VolMode, l.usedAfter.VolSource, l.config, withKataContainers)

		// Remove everything.
		By("cleaning up")
		dax.DeletePod(f, podBefore)
		dax.DeletePod(f, podAfter)
		cleanup()
	}

	// This changes controller and node versions at the same time.
	It("everything [Slow]", func() {
		// First try the downgrade direction. We rely here on
		// the driver being named after a deployment (see
		// csi_volumes.go).
		currentDeploymentName := driver.GetDriverInfo().Name
		oldDeploymentName := currentDeploymentName + "-" + base
		testVersionChange(oldDeploymentName)

		// Now that older driver is running, do the same for
		// an upgrade. When the test is done, the cluster is
		// back in the same state as before.
		testVersionChange(currentDeploymentName)
	})

	// This test combines controller and node from different releases
	// and checks that they can work together. This can happen when
	// the operator mutates the deployment objects and the change isn't
	// applied everywhere at once.
	//
	// We change the controller because that side is easier to modify
	// (scale down, change spec, scale up) and test only one direction
	// (old nodes, new controller) because that direction is more likely
	// and if there compatibility issues, then hopefully the direction
	// of the skew won't matter.
	It("controller [Slow]", func() {
		withKataContainers := false
		c, err := deploy.NewCluster(f.ClientSet, f.DynamicClient)

		// Get the current controller image.
		//
		// The test has to make some assumptions about our deployments,
		// like "controller is in a statefulset" and what its name is.
		// The test also relies on command line parameters staying
		// compatible. If we ever change that, we need to add some extra
		// logic here.
		controllerSet, err := f.ClientSet.AppsV1().StatefulSets("default").Get(context.Background(), "pmem-csi-controller", metav1.GetOptions{})
		framework.ExpectNoError(err, "get controller")
		currentImage := controllerSet.Spec.Template.Spec.Containers[0].Image
		Expect(currentImage).To(ContainSubstring("pmem-csi"))

		// Now downgrade.
		currentDeploymentName := driver.GetDriverInfo().Name
		otherDeploymentName := currentDeploymentName + "-" + base
		deployment, err := deploy.Parse(otherDeploymentName)
		if err != nil {
			framework.Failf("internal error while parsing %s: %v", otherDeploymentName, err)
		}
		deploy.EnsureDeploymentNow(f, deployment)
		deployment, err = deploy.FindDeployment(c)
		framework.ExpectNoError(err, "find downgraded deployment")
		Expect(deployment.Version).NotTo(BeEmpty(), "should be running an old release")

		// Update the controller image.
		setImage := func(newImage string) string {
			By(fmt.Sprintf("changing controller image to %s", newImage))
			controllerSet, err := f.ClientSet.AppsV1().StatefulSets("default").Get(context.Background(), "pmem-csi-controller", metav1.GetOptions{})
			framework.ExpectNoError(err, "get controller")
			oldImage := controllerSet.Spec.Template.Spec.Containers[0].Image
			controllerSet.Spec.Template.Spec.Containers[0].Image = newImage
			controllerSet, err = f.ClientSet.AppsV1().StatefulSets("default").Update(context.Background(), controllerSet, metav1.UpdateOptions{})
			framework.ExpectNoError(err, "update controller")

			// Ensure that the stateful set runs the modified image.
			e2estatefulset.Restart(f.ClientSet, controllerSet)

			return oldImage
		}
		oldImage := setImage(currentImage)
		// Strictly speaking, we could also leave a broken deployment behind because the next
		// test will want to start with a deployment of the current release and thus will
		// reinstall anyway, but it is cleaner this way.
		defer setImage(oldImage)

		// check that PMEM-CSI is up again.
		framework.ExpectNoError(err, "get cluster information")
		deploy.WaitForPMEMDriver(c, "pmem-csi", deployment.Namespace)

		// This relies on FindDeployment getting the version number from the image.
		deployment, err = deploy.FindDeployment(c)
		framework.ExpectNoError(err, "find modified deployment")
		Expect(deployment.Version).To(BeEmpty(), "should be running a current release") // TODO: what about testing 0.8?

		// Now that we are in a version skewed state, try some simple interaction between
		// controller and node by creating a volume and using it. This makes sense
		// even for CSI inline volumes because those may invoke the scheduler extensions.
		init(false)
		defer cleanup()
		pod := dax.CreatePod(f, "pod-skew-test", l.unused.Pattern.VolMode, l.unused.VolSource, l.config, withKataContainers)
		dax.DeletePod(f, pod)
	})
}

// createVolumeResource takes one of the test patterns prepared by InitSkewTestSuite and
// creates a volume for it.
func createVolumeResource(pmemDriver testsuites.TestDriver, config *testsuites.PerTestConfig, suffix string, pattern testpatterns.TestPattern) *testsuites.VolumeResource {
	_, _, scp, err := driver.DecodeTestPatternName(pattern.Name)
	Expect(err).NotTo(HaveOccurred(), "decode test pattern name")
	pmemDriver = pmemDriver.(driver.DynamicDriver).WithStorageClassNameSuffix(suffix).WithParameters(scp.Parameters)
	return testsuites.CreateVolumeResource(pmemDriver, config, pattern, e2evolume.SizeRange{})
}
