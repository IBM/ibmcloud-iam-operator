package v1alpha1

import (
	resv1 "github.ibm.com/seed/ibmcloud-iam-operator/pkg/lib/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccessGroupDef struct {
	AccessGroupName 		string 	`json:"accessGroupName"`
	AccessGroupNamespace 	string 	`json:"accessGroupNamespace"`
}

type Subject struct {
	UserEmail   	string      	`json:"userEmail,omitempty"`
	ServiceID   	string      	`json:"serviceID,omitempty"`
	AccessGroupID 	string 			`json:"accessGroupID,omitempty"`
	AccessGroupDef 	AccessGroupDef 	`json:"accessGroupDef,omitempty"`
}

type Target struct {
	ResourceGroup 	string `json:"resourceGroup,omitempty"`
	ServiceClass   	string `json:"serviceClass,omitempty"`
	ServiceID     	string `json:"serviceID,omitempty"`
	ResourceName  	string `json:"resourceName,omitempty"` 
	ResourceID    	string `json:"resourceID,omitempty"`
	ResourceKey    	string `json:"resourceKey,omitempty"`
	ResourceValue  	string `json:"resourceValue,omitempty"`
}

type CustomRolesDef struct {
	CustomRoleName 		string 	`json:"customRoleName"`
	CustomRoleNamespace string 	`json:"customRoleNamespace"`
}

type Roles struct {
	DefinedRoles 		[]string 	   		`json:"definedRoles,omitempty"`
	CustomRolesDName 	[]string			`json:"customRolesDName,omitempty"`
	CustomRolesDef  	[]CustomRolesDef 	`json:"customRolesDef,omitempty"`
}

// AccessPolicySpec defines the desired state of AccessPolicy
type AccessPolicySpec struct {
	Subject Subject `json:"subject,required"`
	Roles   Roles 	`json:"roles,required"`
	Target  Target  `json:"target,required"`
}

// AccessPolicyStatus defines the observed state of AccessPolicy
type AccessPolicyStatus struct {
	resv1.ResourceStatus `json:",inline"`
	PolicyID 	string 	 `json:"policyID,omitempty"`
	Subject 	Subject  `json:"subject,omitempty"`
	Roles   	Roles 	 `json:"roles,omitempty"`
	Target  	Target   `json:"target,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessPolicy is the Schema for the accesspolicies API
// +kubebuilder:resource:path=accesspolicies,scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type AccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessPolicySpec   `json:"spec,omitempty"`
	Status AccessPolicyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessPolicyList contains a list of AccessPolicy
type AccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessPolicy `json:"items"`
}

// GetStatus returns the access policy status
func (s *AccessPolicy) GetStatus() resv1.Status {
	return &s.Status
}

func init() {
	SchemeBuilder.Register(&AccessPolicy{}, &AccessPolicyList{})
}

