/*
Copyright 2020 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package base

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceMode type decleration for allowed driver device managers
type DeviceMode string

// +kubebuilder:validation:Enum=lvm,direct
const (
	// DeviceModeLVM represents 'lvm' device manager
	DeviceModeLVM DeviceMode = "lvm"
	// DeviceModeDirect represents 'direct' device manager
	DeviceModeDirect DeviceMode = "direct"
	// DeviceModeFake represents a device manager for testing:
	// volume creation and deletion is just recorded in memory,
	// without any actual backing store. Such fake volumes cannot
	// be used for pods.
	DeviceModeFake DeviceMode = "fake"
)

// Set sets the value
func (mode *DeviceMode) Set(value string) error {
	switch value {
	case string(DeviceModeLVM), string(DeviceModeDirect), string(DeviceModeFake):
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

// DriverType type decleration for representing the type of driver instance
type DriverType int

const (
	// ControllerDriver represents controller driver instance
	ControllerDriver DriverType = iota
	// NodeDriver represents driver instance running on worker nodes
	NodeDriver
)

func (t DriverType) String() string {
	switch t {
	case ControllerDriver:
		return "Controller"
	case NodeDriver:
		return "Node"
	}
	return ""
}

// NOTE(avalluri): Due to below errors we stop setting
// few CRD schema fields by prefixing those lines a '-'.
// Once the below issues go fixed replace those '-' with '+'
// Setting default(+kubebuilder:default=value) for v1beta1 CRD fails, only supports since v1 CRD.
//   Related issue : https://github.com/kubernetes-sigs/controller-tools/issues/478
// Fails setting min/max for integers: https://github.com/helm/helm/issues/5806

// DeploymentSpec defines the desired state of Deployment
// +k8s:deepcopy-gen=true
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

const (
	// EventReasonNew new driver deployment found
	EventReasonNew = "NewDeployment"
	// EventReasonRunning driver has been successfully deployed
	EventReasonRunning = "Running"
	// EventReasonFailed driver deployment failed, Event.Message holds detailed information
	EventReasonFailed = "Failed"
)

// DeploymentPhase represents the status phase of a driver deployment
type DeploymentPhase string

const (
	// DeploymentPhaseNew indicates a new deployment
	DeploymentPhaseNew DeploymentPhase = ""
	// DeploymentPhaseRunning indicates that the deployment was successful
	DeploymentPhaseRunning DeploymentPhase = "Running"
	// DeploymentPhaseFailed indicates that the deployment was failed
	DeploymentPhaseFailed DeploymentPhase = "Failed"
)

// DeploymentConditionType type for representing a deployment status condition
type DeploymentConditionType string

const (
	// CertsVerified means the provided deployment secrets are verified and valid for usage
	CertsVerified DeploymentConditionType = "CertsVerified"
	// CertsReady means secrests/certificates required for running the PMEM-CSI driver
	// are ready and the deployment could progress further
	CertsReady DeploymentConditionType = "CertsReady"
	// DriverDeployed means that the all the sub-resources required for the deployment CR
	// got created
	DriverDeployed DeploymentConditionType = "DriverDeployed"
)

// DeploymentCondition type definition for driver deployment status conditions
// +k8s:deepcopy-gen=true
type DeploymentCondition struct {
	// Type of condition.
	Type DeploymentConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Message human readable text that explain why this condition is in this state
	// +optional
	Reason string `json:"reason,omitempty"`
	// Last time the condition was probed.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// DriverStatus type definition for representing deployed driver status
// +k8s:deepcopy-gen=true
type DriverStatus struct {
	// DriverComponent represents type of the driver: controller or node
	DriverComponent string `json:"component"`
	// Status represents the state of the component; one of `Ready` or `NotReady`.
	// Component becomes `Ready` if all the instances(Pods) of the driver component
	// are in running state. Otherwise, `NotReady`.
	Status string `json:"status"`
	// Reason represents the human readable text that explains why the
	// driver is in this state.
	Reason string `json:"reason"`
	// LastUpdated time of the driver status
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// DeploymentStatus defines the observed state of Deployment
// +k8s:deepcopy-gen=true
type DeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make operator-generate-k8s" to regenerate code after modifying this file

	// Phase indicates the state of the deployment
	Phase  DeploymentPhase `json:"phase,omitempty"`
	Reason string          `json:"reason,omitempty"`
	// Conditions
	Conditions []DeploymentCondition `json:"conditions,omitempty"`
	Components []DriverStatus        `json:"driverComponents,omitempty"`
	// LastUpdated time of the deployment status
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// SetDriverStatus set/updates the deployment status for given driver component
func (s *DeploymentStatus) SetDriverStatus(t DriverType, status, reason string) {
	if s.Components == nil {
		s.Components = make([]DriverStatus, 2)
	}
	s.Components[t] = DriverStatus{
		DriverComponent: t.String(),
		Status:          status,
		Reason:          reason,
		LastUpdated:     metav1.Now(),
	}
}

// SetCondition set/updates the deployment status condition
func (s *DeploymentStatus) SetCondition(t DeploymentConditionType, state corev1.ConditionStatus, reason string) {
	for _, c := range s.Conditions {
		if c.Type == t {
			c.Status = state
			c.Reason = reason
			c.LastUpdateTime = metav1.Now()
			return
		}
	}
	s.Conditions = append(s.Conditions, DeploymentCondition{
		Type:           t,
		Status:         state,
		Reason:         reason,
		LastUpdateTime: metav1.Now(),
	})
}

// EnsureDefaults make sure that the deployment object has all defaults set properly
func (spec *DeploymentSpec) EnsureDefaults(operatorImage string) error {
	// Validate the given driver mode.
	// In a realistic case this check might not needed as it should be
	// handled by JSON schema as we defined deviceMode as enumeration.
	switch spec.DeviceMode {
	case "":
		spec.DeviceMode = DefaultDeviceMode
	case DeviceModeDirect, DeviceModeLVM:
	default:
		return fmt.Errorf("invalid device mode %q", spec.DeviceMode)
	}

	if spec.Image == "" {
		// If provided use operatorImage
		if operatorImage != "" {
			spec.Image = operatorImage
		} else {
			spec.Image = DefaultDriverImage
		}
	}
	if spec.PullPolicy == "" {
		spec.PullPolicy = DefaultImagePullPolicy
	}
	if spec.LogLevel == 0 {
		spec.LogLevel = DefaultLogLevel
	}

	if spec.ProvisionerImage == "" {
		spec.ProvisionerImage = DefaultProvisionerImage
	}

	if spec.NodeRegistrarImage == "" {
		spec.NodeRegistrarImage = DefaultRegistrarImage
	}

	if spec.NodeSelector == nil {
		spec.NodeSelector = DefaultNodeSelector
	}

	if spec.PMEMPercentage == 0 {
		spec.PMEMPercentage = DefaultPMEMPercentage
	}

	if spec.KubeletDir == "" {
		spec.KubeletDir = DefaultKubeletDir
	}

	return nil
}

// HaveCertificatesConfigured checks if the configured deployment
// certificate fields are valid. Returns
// - true with nil error if provided certificates are valid.
// - false with nil error if no certificates are provided.
// - false with appropriate error if invalid/incomplete certificates provided.
func (spec *DeploymentSpec) HaveCertificatesConfigured() (bool, error) {
	// Encoded private keys and certificates
	caCert := spec.CACert
	registryPrKey := spec.RegistryPrivateKey
	ncPrKey := spec.NodeControllerPrivateKey
	registryCert := spec.RegistryCert
	ncCert := spec.NodeControllerCert

	// sanity check
	if caCert == nil {
		if registryCert != nil || ncCert != nil {
			return false, fmt.Errorf("incomplete deployment configuration: missing root CA certificate by which the provided certificates are signed")
		}
		return false, nil
	} else if registryCert == nil || registryPrKey == nil || ncCert == nil || ncPrKey == nil {
		return false, fmt.Errorf("incomplete deployment configuration: certificates and corresponding private keys must be provided")
	}

	return true, nil
}

// GetHyphenedName returns the name of the deployment with dots replaced by hyphens.
// Most objects created for the deployment will use hyphens in the name, sometimes
// with an additional suffix like -controller, but others must use the original
// name (like the CSIDriver object).
func GetHyphenedName(d metav1.Object) string {
	return strings.ReplaceAll(d.GetName(), ".", "-")
}

// RegistrySecretName returns the name of the registry
// Secret object used by the deployment
func RegistrySecretName(d metav1.Object) string {
	return GetHyphenedName(d) + "-registry-secrets"
}

// NodeSecretName returns the name of the node-controller
// Secret object used by the deployment
func NodeSecretName(d metav1.Object) string {
	return GetHyphenedName(d) + "-node-secrets"
}

// CSIDriverName returns the name of the CSIDriver
// object name for the deployment
func CSIDriverName(d metav1.Object) string {
	return d.GetName()
}

// ControllerServiceName returns the name of the controller
// Service object used by the deployment
func ControllerServiceName(d metav1.Object) string {
	return GetHyphenedName(d) + "-controller"
}

// MetricsServiceName returns the name of the controller metrics
// Service object used by the deployment
func MetricsServiceName(d metav1.Object) string {
	return GetHyphenedName(d) + "-metrics"
}

// ServiceAccountName returns the name of the ServiceAccount
// object used by the deployment
func ServiceAccountName(d metav1.Object) string {
	return GetHyphenedName(d) + "-controller"
}

// ProvisionerRoleName returns the name of the provisioner's
// RBAC Role object name used by the deployment
func ProvisionerRoleName(d metav1.Object) string {
	return GetHyphenedName(d) + "-external-provisioner-cfg"
}

// ProvisionerRoleBindingName returns the name of the provisioner's
// RoleBinding object name used by the deployment
func ProvisionerRoleBindingName(d metav1.Object) string {
	return GetHyphenedName(d) + "-csi-provisioner-role-cfg"
}

// ProvisionerClusterRoleName returns the name of the
// provisioner's ClusterRole object name used by the deployment
func ProvisionerClusterRoleName(d metav1.Object) string {
	return GetHyphenedName(d) + "-external-provisioner-runner"
}

// ProvisionerClusterRoleBindingName returns the name of the
// provisioner ClusterRoleBinding object name used by the deployment
func ProvisionerClusterRoleBindingName(d metav1.Object) string {
	return GetHyphenedName(d) + "-csi-provisioner-role"
}

// NodeDriverName returns the name of the driver
// DaemonSet object name used by the deployment
func NodeDriverName(d metav1.Object) string {
	return GetHyphenedName(d) + "-node"
}

// ControllerDriverName returns the name of the controller
// StatefulSet object name used by the deployment
func ControllerDriverName(d metav1.Object) string {
	return GetHyphenedName(d) + "-controller"
}
