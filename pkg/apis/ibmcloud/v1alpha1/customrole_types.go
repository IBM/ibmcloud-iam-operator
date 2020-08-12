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

// CustomRoleSpec defines the desired state of CustomRole
type CustomRoleSpec struct {
	RoleName    string   `json:"roleName,required"`
	ServiceClass string  `json:"serviceClass,required"`
	DisplayName string   `json:"displayName,required"`
	Description string   `json:"description,required"`
	Actions     []string `json:"actions,required"`
}

// CustomRoleStatus defines the observed state of CustomRole
type CustomRoleStatus struct {
	resv1.ResourceStatus `json:",inline"`
	RoleID 		string 	 `json:"roleID,omitempty"`
	RoleCRN 	string 	 `json:"roleCRN,omitempty"`
	RoleName    string   `json:"roleName,omitempty"`
	ServiceClass string  `json:"serviceClass,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Actions     []string `json:"actions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomRole is the Schema for the customroles API
// +kubebuilder:resource:path=customroles,scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type CustomRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomRoleSpec   `json:"spec,omitempty"`
	Status CustomRoleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomRoleList contains a list of CustomRole
type CustomRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomRole `json:"items"`
}

// GetStatus returns the custom role status
func (s *CustomRole) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&CustomRole{}, &CustomRoleList{})
}

