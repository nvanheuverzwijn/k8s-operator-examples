/*
Copyright 2021.

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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StatusReconciling                = "Reconciling"
	StatusFailedToResolveSource      = "Failed to resolve source"
	StatusFailedToResolveDestination = "Failed to resolve destination"
	StatusReady                      = "Ready"
)

// BackupClaimSpec defines the desired state of BackupClaim
type BackupClaimSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// source of the backup
	Source BackupClaimSourceSpec `json:"source,omitempty"`
	// destination for the backup
	Destination BackupClaimDestinationSpec `json:"destination,omitempty"`
}

// BackupClaimStatus defines the observed state of BackupClaim
type BackupClaimStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Current status of the claim
	Status string `json:"status,omitempty"`

	Error string `json:"error"`

	// When was this claim created
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// When was this backup claim resolved
	ResolvedAt *metav1.Time `json:"resolvedAt,omitempty"`
}

// SOURCE SPEC
//

type BackupClaimSourceSpec struct {
	S3 BackupClaimS3SourceSpec `json:"s3,omitempty"`
}

type BackupClaimS3SourceSpec struct {
	BucketName string `json:"bucketName,required"`
	Key        string `json:"key,required"`
}

// DESTINATION SPECS
//

type BackupClaimDestinationSpec struct {
	Pod         BackupClaimNewPodDestinationSpec      `json:"pod,omitempty"`
	ExistingPod BackupClaimExistingPodDestinationSpec `json:"existingPod,omitempty"`
}

type BackupClaimExistingPodDestinationSpec struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type BackupClaimNewPodDestinationSpec struct {
	// Create a new pod or use an existing one
	// Prefix to give to the pod
	NamePrefix string `json:"namePrefix,omitempty"`

	// Resources requested
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackupClaim is the Schema for the backupclaims API
type BackupClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupClaimSpec   `json:"spec,omitempty"`
	Status BackupClaimStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupClaimList contains a list of BackupClaim
type BackupClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupClaim{}, &BackupClaimList{})
}
