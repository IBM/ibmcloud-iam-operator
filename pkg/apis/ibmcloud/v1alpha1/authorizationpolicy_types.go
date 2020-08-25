/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	resv1 "github.com/IBM/ibmcloud-iam-operator/pkg/lib/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Info struct {
	ServiceClass  string `json:"serviceClass,required"`
	ServiceID     string `json:"serviceID,omitempty"`
	ResourceName  string `json:"resourceName,omitempty"`
	ResourceID    string `json:"resourceID,omitempty"`
	ResourceKey   string `json:"resourceKey,omitempty"`
	ResourceValue string `json:"resourceValue,omitempty"`
	ResourceGroup string `json:"resourceGroup,omitempty"` // mutually exclusive with ServiceID
}

// AuthorizationPolicySpec defines the desired state of AuthorizationPolicy
type AuthorizationPolicySpec struct {
	Source Info     `json:"source,required"`
	Roles  []string `json:"roles,required"`
	Target Info     `json:"target,required"`
}

// AuthorizationPolicyStatus defines the observed state of AuthorizationPolicy
type AuthorizationPolicyStatus struct {
	resv1.ResourceStatus `json:",inline"`
	PolicyID             string   `json:"policyID,omitempty"`
	Source               Info     `json:"source,omitempty"`
	Roles                []string `json:"roles,omitempty"`
	Target               Info     `json:"target,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthorizationPolicy is the Schema for the authorizationpolicies API
// +kubebuilder:resource:path=authorizationpolicies,scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type AuthorizationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthorizationPolicySpec   `json:"spec,omitempty"`
	Status AuthorizationPolicyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthorizationPolicyList contains a list of AuthorizationPolicy
type AuthorizationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthorizationPolicy `json:"items"`
}

// GetStatus returns the authorization policy status
func (s *AuthorizationPolicy) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&AuthorizationPolicy{}, &AuthorizationPolicyList{})
}
