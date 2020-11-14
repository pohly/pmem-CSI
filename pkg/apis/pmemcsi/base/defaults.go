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

	// DefaultControllerResourceLimitCPU default CPU resource limit used for controller driver container
	DefaultControllerResourceLimitCPU = "500m" // MilliSeconds
	// DefaultControllerResourceLimitMemory default memory resource limit used for controller driver container
	DefaultControllerResourceLimitMemory = "250Mi" // MB
	// DefaultNodeResourceLimitCPU default CPU resource limit used for node driver container
	DefaultNodeResourceLimitCPU = "600m" // MilliSeconds
	// DefaultNodeResourceLimitMemory default memory resource limit used for node driver container
	DefaultNodeResourceLimitMemory = "500Mi" // MB

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
