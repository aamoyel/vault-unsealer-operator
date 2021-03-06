/*
Copyright 2022.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Pending: Switch to Running state
	StatusPending = "PENDING"
	// Running: Create deployment
	StatusRunning  = "RUNNING"
	StatusCleaning = "CLEANING"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UnsealSpec defines the desired state of Unseal
type UnsealSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Container image to use
	Image string `json:"image,omitempty"`
	// Number of replicas
	Replicas int32 `json:"replicas,omitempty"`
}

// UnsealStatus defines the observed state of Unseal
type UnsealStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Status of the unseal
	UnsealStatus string `json:"unsealStatus,omitempty"`
	// Last Pod Name status
	LastDeployName string `json:"lastDeployName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Unseal is the Schema for the unseals API
type Unseal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnsealSpec   `json:"spec,omitempty"`
	Status UnsealStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UnsealList contains a list of Unseal
type UnsealList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Unseal `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Unseal{}, &UnsealList{})
}
