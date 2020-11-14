/*
Copyright 2020 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"github.com/intel/pmem-csi/pkg/apis/pmemcsi/base"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen=true
// DeploymentSpec defines the desired state of Deployment
type DeploymentSpec struct {
	// Important: Run "make operator-generate-k8s" to regenerate code after modifying this file

	base.DeploymentSpec `json:",inline"`

	// ControllerResources Compute resources required by Controller driver
	ControllerResources *corev1.ResourceRequirements `json:"controllerResources,omitempty"`
	// NodeResources Compute resources required by Node driver
	NodeResources *corev1.ResourceRequirements `json:"nodeResources,omitempty"`
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

	if d.Spec.ControllerResources == nil {
		d.Spec.ControllerResources = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultControllerResourceLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultControllerResourceLimitMemory),
			},
		}
	}

	if d.Spec.NodeResources == nil {
		d.Spec.NodeResources = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(base.DefaultNodeResourceLimitCPU),
				corev1.ResourceMemory: resource.MustParse(base.DefaultNodeResourceLimitMemory),
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
