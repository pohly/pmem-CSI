/*
Copyright The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/intel/pmem-csi/pkg/apis/pmemcsi/v1beta1"
)

var _ conversion.Convertible = &Deployment{}

// ConvertTo converts to Hub(v1beta1) type
func (d *Deployment) ConvertTo(dst conversion.Hub) error {
	dstDep := dst.(*v1beta1.Deployment)

	// Use v1alpha1 `spec.{node,controller}Resources` for setting pmem-driver
	// container resources, other container resources are set to default.
	if d.Spec.NodeResources != nil {
		dstDep.Spec.NodeDriverResources = d.Spec.NodeResources
	}
	if d.Spec.ControllerResources != nil {
		dstDep.Spec.ControllerDriverResources = d.Spec.ControllerResources
	}

	// no change in other fields
	dstDep.ObjectMeta = d.ObjectMeta
	d.Spec.DeploymentSpec.DeepCopyInto(&dstDep.Spec.DeploymentSpec)
	d.Status.DeepCopyInto(&dstDep.Status)

	// +kubebuilder:docs-gen:collapse=rote conversion

	klog.Infof("Coverted Object: %+v", *dstDep)

	return nil
}

// ConvertFrom converts from Hub type to current type
func (d *Deployment) ConvertFrom(src conversion.Hub) error {
	srcDep := src.(*v1beta1.Deployment)

	// Use v1beta1 `spec.{node,controller}DriverResources` as setting the
	// pod resources
	if srcDep.Spec.NodeDriverResources != nil {
		d.Spec.NodeResources = srcDep.Spec.NodeDriverResources
	}
	if srcDep.Spec.ControllerDriverResources != nil {
		d.Spec.ControllerResources = srcDep.Spec.ControllerDriverResources
	}

	// no change in other fields
	d.ObjectMeta = srcDep.ObjectMeta
	srcDep.Spec.DeploymentSpec.DeepCopyInto(&d.Spec.DeploymentSpec)
	srcDep.Status.DeepCopyInto(&d.Status)

	// +kubebuilder:docs-gen:collapse=rote conversion

	return nil
}
