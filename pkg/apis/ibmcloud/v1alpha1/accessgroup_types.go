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

// AccessGroupSpec defines the desired state of AccessGroup
type AccessGroupSpec struct {
	Name 			string 	 `json:"name"`
	Description 	string   `json:"description"`
	UserEmails    	[]string `json:"userEmails,omitempty"`
	ServiceIDs    	[]string `json:"serviceIDs,omitempty"`
}

// AccessGroupStatus defines the observed state of AccessGroup
type AccessGroupStatus struct {
	resv1.ResourceStatus `json:",inline"`
	GroupID 		string 	 `json:"GroupID,omitempty"`
	Name 			string 	 `json:"name,omitempty"`
	Description 	string   `json:"description,omitempty"`
	UserEmails    	[]string `json:"userEmails,omitempty"`
	ServiceIDs    	[]string `json:"serviceIDs,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessGroup is the Schema for the accessgroup API
// +kubebuilder:resource:path=accessgroups,scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type AccessGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessGroupSpec   `json:"spec,omitempty"`
	Status AccessGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessGroupList contains a list of AccessGroup
type AccessGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessGroup `json:"items"`
}

// GetStatus returns the access group status
func (s *AccessGroup) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&AccessGroup{}, &AccessGroupList{})
}

