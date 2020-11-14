/*
Copyright 2020 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package v1beta1

import (
	"github.com/intel/pmem-csi/pkg/apis/pmemcsi/base"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeploymentSpec defines the desired state of Deployment
// +k8s:deepcopy-gen=true
type DeploymentSpec struct {
	// Important: Run "make operator-generate-k8s" to regenerate code after modifying this file

	base.DeploymentSpec `json:",inline"`

	// ProvisionerResources Compute resources required by provisioner sidecar container
	ProvisionerResources *corev1.ResourceRequirements `json:"provisionerResources,omitempty"`
	// NodeRegistrarResources Compute resources required by node registrar sidecar container
	NodeRegistrarResources *corev1.ResourceRequirements `json:"nodeRegistrarResources,omitempty"`
	// NodeDriverResources Compute resources required by driver container running on worker nodes
	NodeDriverResources *corev1.ResourceRequirements `json:"nodeDriverResources,omitempty"`
	// ControllerDriverResources Compute resources required by driver container running on master node
	ControllerDriverResources *corev1.ResourceRequirements `json:"controllerDriverResources,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Deployment is the Schema for the deployments API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=deployments,scope=Cluster
// +kubebuilder:printcolumn:name="DeviceMode",type=string,JSONPath=`.spec.deviceMode`
// +kubebuilder:printcolumn:name="NodeSelector",type=string,JSONPath=`.spec.nodeSelector`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:storageversion
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentSpec        `json:"spec,omitempty"`
	Status base.DeploymentStatus `json:"status,omitempty"`
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

func (d *Deployment) SetCondition(t base.DeploymentConditionType, state corev1.ConditionStatus, reason string) {
	d.Status.SetCondition(t, state, reason)
}

func (d *Deployment) SetDriverStatus(t base.DriverType, status, reason string) {
	d.Status.SetDriverStatus(t, status, reason)
}

// EnsureDefaults make sure that the deployment object has all defaults set properly
func (d *Deployment) EnsureDefaults(operatorImage string) error {
	if err := d.Spec.EnsureDefaults(operatorImage); err != nil {
		return err
	}

	if d.Spec.ControllerDriverResources == nil {
		d.Spec.ControllerDriverResources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultControllerResourceRequestCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultControllerResourceRequestMemory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultControllerResourceLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultControllerResourceLimitMemory),
			},
		}
	}

	if d.Spec.ProvisionerResources == nil {
		d.Spec.ProvisionerResources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultProvisionerRequestCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultProvisionerRequestMemory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultProvisionerLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultProvisionerLimitMemory),
			},
		}
	}

	if d.Spec.NodeDriverResources == nil {
		d.Spec.NodeDriverResources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultNodeResourceRequestCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultNodeResourceRequestMemory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultNodeResourceLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultNodeResourceLimitMemory),
			},
		}
	}

	if d.Spec.NodeRegistrarResources == nil {
		d.Spec.NodeRegistrarResources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultNodeRegistrarRequestCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultNodeRegistrarRequestMemory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultNodeRegistrarLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultNodeRegistrarLimitMemory),
			},
		}
	}

	return nil
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

// HaveCertificatesConfigured checks if the configured deployment
func (d *Deployment) HaveCertificatesConfigured() (bool, error) {
	return d.Spec.HaveCertificatesConfigured()
}
