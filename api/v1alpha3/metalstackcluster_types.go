/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MetalStackClusterSpec defines the desired state of MetalStackCluster
type MetalStackClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint v1alpha3.APIEndpoint `json:"controlPlaneEndpoint"`

	// todo: This should be required after the implementation.
	// +optional
	NetworkID *string `json:"networkID,omitempty"`

	// Partition is the physical location where the cluster will be created
	Partition *string `json:"partition,omitempty"`

	// ProjectID is th projectID of MetalStackCluster. Edit MetalStackCluster_types.go to remove/update
	ProjectID *string `json:"projectID,omitempty"`

	// TODO: Remove the following members. Complete the logic for the NetworkID above.

	// AdditionalNetworks this cluster should be part of
	// +optional
	AdditionalNetworks []string `json:"additionalNetworks,omitempty"`

	// PrivateNetworkID is the id if the network which connects the machine together
	// +optional
	PrivateNetworkID *string `json:"privateNetworkID,omitempty"`
}

// MetalStackClusterStatus defines the observed state of MetalStackCluster
type MetalStackClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready denotes that the cluster (infrastructure) is ready.
	// +optional
	Ready bool `json:"ready"`

	// todo: Consider CR Firewall.
	FirewallReady bool `json:"firewallReady,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true

// MetalStackCluster is the Schema for the MetalStackclusters API
type MetalStackCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetalStackClusterSpec   `json:"spec,omitempty"`
	Status MetalStackClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MetalStackClusterList contains a list of MetalStackCluster
type MetalStackClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetalStackCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MetalStackCluster{}, &MetalStackClusterList{})
}
