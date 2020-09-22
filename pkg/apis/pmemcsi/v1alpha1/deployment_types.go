/*
Copyright 2020 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceMode type decleration for allowed driver device managers
type DeviceMode string

// Set sets the value
func (mode *DeviceMode) Set(value string) error {
	switch value {
	case string(DeviceModeLVM), string(DeviceModeDirect):
		*mode = DeviceMode(value)
	case "ndctl":
		// For backwards-compatibility.
		*mode = DeviceModeDirect
	default:
		return errors.New("invalid device manager mode")
	}
	return nil
}

func (mode *DeviceMode) String() string {
	return string(*mode)
}

// +kubebuilder:validation:Enum=lvm,direct
const (
	// DeviceModeLVM represents 'lvm' device manager
	DeviceModeLVM DeviceMode = "lvm"
	// DeviceModeDirect represents 'direct' device manager
	DeviceModeDirect DeviceMode = "direct"
)

// NOTE(avalluri): Due to below errors we stop setting
// few CRD schema fields by prefixing those lines a '-'.
// Once the below issues go fixed replace those '-' with '+'
// Setting default(+kubebuilder:default=value) for v1beta1 CRD fails, only supports since v1 CRD.
//   Related issue : https://github.com/kubernetes-sigs/controller-tools/issues/478
// Fails setting min/max for integers: https://github.com/helm/helm/issues/5806

// DeploymentSpec defines the desired state of Deployment
type DeploymentSpec struct {
	// Important: Run "make operator-generate-k8s" to regenerate code after modifying this file

	// PMEM-CSI driver container image
	Image string `json:"image,omitempty"`
	// PullPolicy image pull policy one of Always, Never, IfNotPresent
	PullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ProvisionerImage CSI provisioner sidecar image
	ProvisionerImage string `json:"provisionerImage,omitempty"`
	// NodeRegistrarImage CSI node driver registrar sidecar image
	NodeRegistrarImage string `json:"nodeRegistrarImage,omitempty"`
	// ControllerResources Compute resources required by Controller driver
	ControllerResources *corev1.ResourceRequirements `json:"controllerResources,omitempty"`
	// NodeResources Compute resources required by Node driver
	NodeResources *corev1.ResourceRequirements `json:"nodeResources,omitempty"`
	// DeviceMode to use to manage PMEM devices. One of lvm, direct
	// +kubebuilder:default:lvm
	DeviceMode DeviceMode `json:"deviceMode,omitempty"`
	// LogLevel number for the log verbosity
	// +kubebuilder:validation:Required
	// kubebuilder:default=3
	LogLevel uint16 `json:"logLevel,omitempty"`
	// RegistryCert encoded certificate signed by a CA for registry server authentication
	// If not provided, provisioned one by the operator using self-signed CA
	RegistryCert []byte `json:"registryCert,omitempty"`
	// RegistryPrivateKey encoded private key used for registry server certificate
	// If not provided, provisioned one by the operator
	RegistryPrivateKey []byte `json:"registryKey,omitempty"`
	// NodeControllerCert encoded certificate signed by a CA for node controller server authentication
	// If not provided, provisioned one by the operator using self-signed CA
	NodeControllerCert []byte `json:"nodeControllerCert,omitempty"`
	// NodeControllerPrivateKey encoded private key used for node controller server certificate
	// If not provided, provisioned one by the operator
	NodeControllerPrivateKey []byte `json:"nodeControllerKey,omitempty"`
	// CACert encoded root certificate of the CA by which the registry and node controller certificates are signed
	// If not provided operator uses a self-signed CA certificate
	CACert []byte `json:"caCert,omitempty"`
	// NodeSelector node labels to use for selection of driver node
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// PMEMPercentage represents the percentage of space to be used by the driver in each PMEM region
	// on every node.
	// This is only valid for driver in LVM mode.
	// +kubebuilder:validation:Required
	// -kubebuilder:validation:Minimum=1
	// -kubebuilder:validation:Maximum=100
	// -kubebuilder:default=100
	PMEMPercentage uint16 `json:"pmemPercentage,omitempty"`
	// Labels contains additional labels for all objects created by the operator.
	Labels map[string]string `json:"labels,omitempty"`
	// KubeletDir kubelet's root directory path
	KubeletDir string `json:"kubeletDir,omitempty"`
}

// DeploymentStatus defines the observed state of Deployment
type DeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make operator-generate-k8s" to regenerate code after modifying this file

	// Phase indicates the state of the deployment
	Phase DeploymentPhase `json:"phase,omitempty"`
	// LastUpdated time of the deployment status
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Deployment is the Schema for the deployments API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=deployments,scope=Cluster
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentSpec   `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeploymentList contains a list of Deployment
type DeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Deployment{}, &DeploymentList{})
}

const (
	// EventReasonNew new driver deployment found
	EventReasonNew = "NewDeployment"
	// EventReasonRunning driver has been successfully deployed
	EventReasonRunning = "Running"
	// EventReasonFailed driver deployment failed, Event.Message holds detailed information
	EventReasonFailed = "Failed"
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

	// DefaultControllerResourceCPU default CPU resource limit used for controller pod
	DefaultControllerResourceCPU = "100m" // MilliSeconds
	// DefaultControllerResourceMemory default memory resource limit used for controller pod
	DefaultControllerResourceMemory = "250Mi" // MB
	// DefaultNodeResourceCPU default CPU resource limit used for node driver pod
	DefaultNodeResourceCPU = "100m" // MilliSeconds
	// DefaultNodeResourceMemory default memory resource limit used for node driver pod
	DefaultNodeResourceMemory = "250Mi" // MB
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

// DeploymentPhase represents the status phase of a driver deployment
type DeploymentPhase string

const (
	// DeploymentPhaseNew indicates a new deployment
	DeploymentPhaseNew DeploymentPhase = ""
	// DeploymentPhaseInitializing indicates deployment initialization is in progress
	DeploymentPhaseInitializing DeploymentPhase = "Initializing"
	// DeploymentPhaseRunning indicates that the deployment was successful
	DeploymentPhaseRunning DeploymentPhase = "Running"
	// DeploymentPhaseFailed indicates that the deployment was failed
	DeploymentPhaseFailed DeploymentPhase = "Failed"
)

// DeploymentChange type declaration for changes between two deployments
type DeploymentChange int

const (
	DriverMode = iota + 1
	DriverImage
	PullPolicy
	LogLevel
	ProvisionerImage
	NodeRegistrarImage
	ControllerResources
	NodeResources
	NodeSelector
	PMEMPercentage
	Labels
	CACertificate
	RegistryCertificate
	RegistryKey
	NodeControllerCertificate
	NodeControllerKey
	KubeletDir
)

func (c DeploymentChange) String() string {
	return map[DeploymentChange]string{
		DriverMode:                "deviceMode",
		DriverImage:               "image",
		PullPolicy:                "imagePullPolicy",
		LogLevel:                  "logLevel",
		ProvisionerImage:          "provisionerImage",
		NodeRegistrarImage:        "nodeRegistrarImage",
		ControllerResources:       "controllerResources",
		NodeResources:             "nodeResources",
		NodeSelector:              "nodeSelector",
		PMEMPercentage:            "pmemPercentage",
		Labels:                    "labels",
		CACertificate:             "caCert",
		RegistryCertificate:       "registryCert",
		RegistryKey:               "registryKey",
		NodeControllerCertificate: "nodeControllerCert",
		NodeControllerKey:         "nodeControllerKey",
		KubeletDir:                "kubeletDir",
	}[c]
}

// EnsureDefaults make sure that the deployment object has all defaults set properly
func (d *Deployment) EnsureDefaults(operatorImage string) error {
	if d.Spec.Image == "" {
		// If provided use operatorImage
		if operatorImage != "" {
			d.Spec.Image = operatorImage
		} else {
			d.Spec.Image = DefaultDriverImage
		}
	}
	if d.Spec.PullPolicy == "" {
		d.Spec.PullPolicy = DefaultImagePullPolicy
	}
	if d.Spec.LogLevel == 0 {
		d.Spec.LogLevel = DefaultLogLevel
	}

	/* Controller Defaults */

	if d.Spec.ProvisionerImage == "" {
		d.Spec.ProvisionerImage = DefaultProvisionerImage
	}

	if d.Spec.ControllerResources == nil {
		d.Spec.ControllerResources = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(DefaultControllerResourceCPU),
				corev1.ResourceMemory: resource.MustParse(DefaultControllerResourceMemory),
			},
		}
	}

	/* Node Defaults */

	// Validate the given driver mode.
	// In a realistic case this check might not needed as it should be
	// handled by JSON schema as we defined deviceMode as enumeration.
	switch d.Spec.DeviceMode {
	case "":
		d.Spec.DeviceMode = DefaultDeviceMode
	case DeviceModeDirect, DeviceModeLVM:
	default:
		return fmt.Errorf("invalid device mode %q", d.Spec.DeviceMode)
	}

	if d.Spec.NodeRegistrarImage == "" {
		d.Spec.NodeRegistrarImage = DefaultRegistrarImage
	}

	if d.Spec.NodeResources == nil {
		d.Spec.NodeResources = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(DefaultNodeResourceCPU),
				corev1.ResourceMemory: resource.MustParse(DefaultNodeResourceMemory),
			},
		}
	}

	if d.Spec.NodeSelector == nil {
		d.Spec.NodeSelector = DefaultNodeSelector
	}

	if d.Spec.PMEMPercentage == 0 {
		d.Spec.PMEMPercentage = DefaultPMEMPercentage
	}

	if d.Spec.KubeletDir == "" {
		d.Spec.KubeletDir = DefaultKubeletDir
	}

	return nil
}

// Compare compares 'other' deployment spec with current deployment and returns
// the all the changes. If len(changes) == 0 represents both deployment spec
// are equivalent.
func (d *Deployment) Compare(other *Deployment) map[DeploymentChange]struct{} {
	changes := map[DeploymentChange]struct{}{}
	if d == nil || other == nil {
		return changes
	}

	if d.Spec.DeviceMode != other.Spec.DeviceMode {
		changes[DriverMode] = struct{}{}
	}
	if d.Spec.Image != other.Spec.Image {
		changes[DriverImage] = struct{}{}
	}
	if d.Spec.PullPolicy != other.Spec.PullPolicy {
		changes[PullPolicy] = struct{}{}
	}
	if d.Spec.LogLevel != other.Spec.LogLevel {
		changes[LogLevel] = struct{}{}
	}
	if d.Spec.ProvisionerImage != other.Spec.ProvisionerImage {
		changes[ProvisionerImage] = struct{}{}
	}
	if d.Spec.NodeRegistrarImage != other.Spec.NodeRegistrarImage {
		changes[NodeRegistrarImage] = struct{}{}
	}
	if !compareResources(d.Spec.ControllerResources, other.Spec.ControllerResources) {
		changes[ControllerResources] = struct{}{}
	}
	if !compareResources(d.Spec.NodeResources, other.Spec.NodeResources) {
		changes[NodeResources] = struct{}{}
	}

	if !reflect.DeepEqual(d.Spec.NodeSelector, other.Spec.NodeSelector) {
		changes[NodeSelector] = struct{}{}
	}

	if d.Spec.PMEMPercentage != other.Spec.PMEMPercentage {
		changes[PMEMPercentage] = struct{}{}
	}

	if !reflect.DeepEqual(d.Spec.Labels, other.Spec.Labels) {
		changes[Labels] = struct{}{}
	}

	if bytes.Compare(d.Spec.CACert, other.Spec.CACert) != 0 {
		changes[CACertificate] = struct{}{}
	}
	if bytes.Compare(d.Spec.RegistryCert, other.Spec.RegistryCert) != 0 {
		changes[RegistryCertificate] = struct{}{}
	}
	if bytes.Compare(d.Spec.NodeControllerCert, other.Spec.NodeControllerCert) != 0 {
		changes[NodeControllerCertificate] = struct{}{}
	}
	if bytes.Compare(d.Spec.RegistryPrivateKey, other.Spec.RegistryPrivateKey) != 0 {
		changes[RegistryKey] = struct{}{}
	}
	if bytes.Compare(d.Spec.NodeControllerPrivateKey, other.Spec.NodeControllerPrivateKey) != 0 {
		changes[NodeControllerKey] = struct{}{}
	}
	if d.Spec.KubeletDir != other.Spec.KubeletDir {
		changes[KubeletDir] = struct{}{}
	}

	return changes
}

// GetHyphenedName returns the name of the deployment with dots replaced by hyphens.
// Most objects created for the deployment will use hyphens in the name, sometimes
// with an additional suffix like -controller, but others must use the original
// name (like the CSIDriver object).
func (d *Deployment) GetHyphenedName() string {
	return strings.ReplaceAll(d.GetName(), ".", "-")
}

// GetOwnerReference returns self owner reference could be used by other object
// to add this deployment to it's owner reference list.
func (d *Deployment) GetOwnerReference() metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         d.APIVersion,
		Kind:               d.Kind,
		Name:               d.GetName(),
		UID:                d.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func GetDeploymentCRDSchema() *apiextensions.JSONSchemaProps {
	One := float64(1)
	Hundred := float64(100)
	return &apiextensions.JSONSchemaProps{
		Type:        "object",
		Description: "https://github.com/intel/pmem-csi.git",
		Properties: map[string]apiextensions.JSONSchemaProps{
			"spec": apiextensions.JSONSchemaProps{
				Type:        "object",
				Description: "DeploymentSpec defines the desired state of Deployment",
				Properties: map[string]apiextensions.JSONSchemaProps{
					"logLevel": apiextensions.JSONSchemaProps{
						Type:        "integer",
						Description: "logging level",
					},
					"deviceMode": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "CSI Driver mode for device management: 'lvm' or 'direct'",
						Enum: []apiextensions.JSON{
							apiextensions.JSON{Raw: []byte("\"" + DeviceModeLVM + "\"")},
							apiextensions.JSON{Raw: []byte("\"" + DeviceModeDirect + "\"")},
						},
					},
					"image": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "PMEM-CSI driver docker image",
					},
					"provisionerImage": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "CSI provisioner docker image",
					},
					"nodeRegistrarImage": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "CSI node driver registrar docker image",
					},
					"imagePullPolicy": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Docker image pull policy: Always, Never, IfNotPresent",
						Enum: []apiextensions.JSON{
							apiextensions.JSON{Raw: []byte("\"" + corev1.PullAlways + "\"")},
							apiextensions.JSON{Raw: []byte("\"" + corev1.PullIfNotPresent + "\"")},
							apiextensions.JSON{Raw: []byte("\"" + corev1.PullNever + "\"")},
						},
					},
					"controllerResources": getResourceRequestsSchema(),
					"nodeResources":       getResourceRequestsSchema(),
					"caCert": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Encoded CA certificate",
					},
					"registryCert": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Encoded pmem-registry certificate",
					},
					"registryKey": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Encoded private key used for generating pmem-registry certificate",
					},
					"nodeControllerCert": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Encoded pmem-node-controller certificate",
					},
					"nodeControllerKey": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Encoded private key used for generating pmem-node-controller certificate",
					},
					"nodeSelector": apiextensions.JSONSchemaProps{
						Type:        "object",
						Description: "Set of node labels to use to select a node to run PMEM-CSI driver",
						AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
							Allows: true,
							Schema: &apiextensions.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					"pmemPercentage": apiextensions.JSONSchemaProps{
						Type:        "integer",
						Description: "Percentage of space to use from total available PMEM space, within range of 1 to 100",
						Minimum:     &One,
						Maximum:     &Hundred,
					},
					"labels": apiextensions.JSONSchemaProps{
						Type:        "object",
						Description: "Set of additional labels for all objects created by the operator",
						AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
							Allows: true,
							Schema: &apiextensions.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					"kubeletDir": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "Kubelet root directory path",
					},
				},
			},
			"status": apiextensions.JSONSchemaProps{
				Type:        "object",
				Description: "State of the deployment",
				Properties: map[string]apiextensions.JSONSchemaProps{
					"phase": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "deployment phase",
					},
					"lastUpdated": apiextensions.JSONSchemaProps{
						Type:        "string",
						Description: "time when the status last updated",
					},
				},
			},
		},
	}
}

func getResourceRequestsSchema() apiextensions.JSONSchemaProps {
	return apiextensions.JSONSchemaProps{
		Type:        "object",
		Description: "Compute resource requirements for controller driver Pod",
		Properties: map[string]apiextensions.JSONSchemaProps{
			"limits": apiextensions.JSONSchemaProps{
				Type:        "object",
				Description: "The maximum amount of compute resources allowed",
				AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
					Allows: true,
					Schema: &apiextensions.JSONSchemaProps{
						Type: "string",
					},
				},
			},
			"requests": apiextensions.JSONSchemaProps{
				Type:        "object",
				Description: "The minimum amount of compute resources required",
				AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
					Allows: true,
					Schema: &apiextensions.JSONSchemaProps{
						Type: "string",
					},
				},
			},
		},
	}
}

func compareResources(rsA *corev1.ResourceRequirements, rsB *corev1.ResourceRequirements) bool {
	if rsA == nil {
		return rsB == nil
	}
	if rsB == nil {
		return false
	}
	if rsA == nil && rsB != nil {
		return false
	}
	if !rsA.Limits.Cpu().Equal(*rsB.Limits.Cpu()) ||
		!rsA.Limits.Memory().Equal(*rsB.Limits.Memory()) ||
		!rsA.Requests.Cpu().Equal(*rsB.Requests.Cpu()) ||
		!rsA.Requests.Memory().Equal(*rsB.Requests.Memory()) {
		return false
	}

	return true
}
