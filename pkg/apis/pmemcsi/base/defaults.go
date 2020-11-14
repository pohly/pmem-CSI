/*
Copyright 2020 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package base

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultLogLevel default logging level used for the driver
	DefaultLogLevel = uint16(5)
	// DefaultImagePullPolicy default image pull policy for all the images used by the deployment
	DefaultImagePullPolicy = corev1.PullIfNotPresent

	defaultDriverImageName = "intel/pmem-csi-driver"
	defaultDriverImageTag  = "canary"
	// DefaultDriverImage default PMEM-CSI driver docker image
	DefaultDriverImage = defaultDriverImageName + ":" + defaultDriverImageTag

	// The sidecar versions must be kept in sync with the
	// deploy/kustomize YAML files!

	defaultProvisionerImageName = "k8s.gcr.io/sig-storage/csi-provisioner"
	defaultProvisionerImageTag  = "v2.0.2"
	// DefaultProvisionerImage default external provisioner image to use
	DefaultProvisionerImage = defaultProvisionerImageName + ":" + defaultProvisionerImageTag

	defaultRegistrarImageName = "k8s.gcr.io/sig-storage/csi-node-driver-registrar"
	defaultRegistrarImageTag  = "v1.2.0"
	// DefaultRegistrarImage default node driver registrar image to use
	DefaultRegistrarImage = defaultRegistrarImageName + ":" + defaultRegistrarImageTag

	// Below resource requests and limits are derived(with minor adjustments) from
	// recommendations reported by VirtualPodAutoscaler(LowerBound -> Requests and UpperBound -> Limits)

	// DefaultControllerResourceRequestCPU default CPU resource request used for controller driver container
	DefaultControllerResourceRequestCPU = "12m" // MilliSeconds
	// DefaultControllerResourceRequestMemory default memory resource request used for controller driver container
	DefaultControllerResourceRequestMemory = "128Mi" // MB
	// DefaultNodeResourceRequestCPU default CPU resource request used for node driver container
	DefaultNodeResourceRequestCPU = "100m" // MilliSeconds
	// DefaultNodeResourceRequestMemory default memory resource request used for node driver container
	DefaultNodeResourceRequestMemory = "250Mi" // MB
	// DefaultNodeRegistrarRequestCPU default CPU resource request used for node registrar container
	DefaultNodeRegistrarRequestCPU = "12m" // MilliSeconds
	// DefaultNodeRegistrarRequestMemory default memory resource request used for node registrar container
	DefaultNodeRegistrarRequestMemory = "128Mi" // MB
	// DefaultProvisionerRequestCPU default CPU resource request used for provisioner container
	DefaultProvisionerRequestCPU = "12m" // MilliSeconds
	// DefaultProvisionerRequestMemory default memory resource request used for node registrar container
	DefaultProvisionerRequestMemory = "128Mi" // MB

	// DefaultControllerResourceLimitCPU default CPU resource limit used for controller driver container
	DefaultControllerResourceLimitCPU = "500m" // MilliSeconds
	// DefaultControllerResourceLimitMemory default memory resource limit used for controller driver container
	DefaultControllerResourceLimitMemory = "250Mi" // MB
	// DefaultNodeResourceLimitCPU default CPU resource limit used for node driver container
	DefaultNodeResourceLimitCPU = "600m" // MilliSeconds
	// DefaultNodeResourceLimitMemory default memory resource limit used for node driver container
	DefaultNodeResourceLimitMemory = "500Mi" // MB
	// DefaultNodeRegistrarLimitCPU default CPU resource limit used for node registrar container
	DefaultNodeRegistrarLimitCPU = "100m" // MilliSeconds
	// DefaultNodeRegistrarLimitMemory default memory resource limit used for node registrar container
	DefaultNodeRegistrarLimitMemory = "128Mi" // MB
	// DefaultProvisionerLimitCPU default CPU resource limit used for provisioner container
	DefaultProvisionerLimitCPU = "250m" // MilliSeconds
	// DefaultProvisionerLimitMemory default memory resource limit used for node registrar container
	DefaultProvisionerLimitMemory = "250Mi" // MB

	// DefaultDeviceMode default device manger used for deployment
	DefaultDeviceMode = DeviceModeLVM
	// DefaultPMEMPercentage PMEM space to reserve for the driver
	DefaultPMEMPercentage = 100
	// DefaultKubeletDir default kubelet's path
	DefaultKubeletDir = "/var/lib/kubelet"
)

var (
	// DefaultNodeSelector default node label used for node selection
	DefaultNodeSelector = map[string]string{"storage": "pmem"}
)
