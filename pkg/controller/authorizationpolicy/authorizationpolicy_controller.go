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

package authorizationpolicy

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.com/IBM/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"
	common "github.com/IBM/ibmcloud-iam-operator/pkg/util"

    "github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
    "github.com/IBM-Cloud/bluemix-go/api/iam/iamv1"
    "github.com/IBM-Cloud/bluemix-go/api/iampap/iampapv1"
    "github.com/IBM-Cloud/bluemix-go/utils"

    "k8s.io/api/core/v1"
    kerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_authorizationpolicy")

const authorizationpolicyFinalizer = "authorizationpolicy.ibmcloud.ibm.com"
const syncPeriod = time.Second * 150

// ContainsFinalizer checks if the instance contains authorizationpolicy finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.AuthorizationPolicy) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, authorizationpolicyFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete authorizationpolicy finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.AuthorizationPolicy) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == authorizationpolicyFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new AuthorizationPolicy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAuthorizationPolicy{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("authorizationpolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AuthorizationPolicy
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.AuthorizationPolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.AuthorizationPolicy{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAuthorizationPolicy implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAuthorizationPolicy{}

// ReconcileAuthorizationPolicy reconciles a AuthorizationPolicy object
type ReconcileAuthorizationPolicy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AuthorizationPolicy object and makes changes based on the state read
// and what is in the AuthorizationPolicy.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAuthorizationPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling AuthorizationPolicy")

	// Fetch the AuthorizationPolicy instance
	instance := &ibmcloudv1alpha1.AuthorizationPolicy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerror.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Authorization Policy resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Authorization Policy")
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AuthorizationPolicyStatus{}) {
		instance.Status.State = "Pending"
		instance.Status.Message = "Processing Resource"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating initial status", "Failed", err.Error())
			return reconcile.Result{}, err
		}
	} 

	// Check that the spec is well-formed
	if !isWellFormed(*instance) {
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return reconcile.Result{}, nil
		}
		if instance.Status.State != "Failed" {
			instance.Status.State = "Failed"
			instance.Status.Message = fmt.Errorf("The spec is not well-formed").Error()
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for bad spec", "Failed", err.Error())
				return reconcile.Result{}, err
			}
	
		}
		return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
	}

	sess, myAccount, err := common.GetIAMAccountInfo(r.client, instance.ObjectMeta.Namespace)
	if err != nil {
		reqLogger.Info("Error getting IBM Cloud IAM account information", instance.Name, err.Error())
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.client.Update(context.Background(), instance); err != nil {
				log.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return reconcile.Result{}, nil
		}
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting IBM Cloud IAM account information"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing IAM account setup", "Failed", err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}
	
	statusPolicyID := instance.Status.PolicyID

	iampapClient, err := iampapv1.New(sess)
	if err != nil {
		reqLogger.Info("Error getting iampap Client", instance.Name, err.Error())

		return reconcile.Result{}, err
	}
	policyAPI := iampapClient.V1Policy()

	iamClient, err := iamv1.New(sess)
	if err != nil {
		reqLogger.Info("Error creating IAM Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	serviceIDAPI := iamClient.ServiceIds()
	serviceRolesAPI := iamClient.ServiceRoles()
	
	// Delete if necessary
 	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, authorizationpolicyFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			if (statusPolicyID != "") { //Policy must exist in IAM since status has an ID 
				err := deleteAuthorizationPolicy(statusPolicyID, policyAPI)
				if err != nil {			
					if !strings.Contains(err.Error(), "not found") {
						reqLogger.Info("Error deleting authorization policy", instance.Name, err.Error())
						return reconcile.Result{}, err
					}	
				}
				reqLogger.Info("Deleted authorization policy.","Policy ID:",statusPolicyID)
				if instance.Status.State != "Deleted" {
					instance.Status.State = "Deleted"
					instance.Status.Message = "IAM authorization policy deleted"
					instance.Status.PolicyID = ""   //clear out the policy ID since policy with this ID has been deleted
					if err := r.client.Update(context.Background(), instance); err != nil {
						reqLogger.Info("Error updating status for authorization policy deletion", "in deletion", err.Error())
						return reconcile.Result{}, err
					}
				}
			}

			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error removing finalizers", "in deletion", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
	}  

	/* Setting roles, resource and subject in Policy */
	policyRoles, err := getRoles(instance, serviceRolesAPI)
	if err != nil {
		reqLogger.Info("Error getting roles for authorization policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting roles for authorization policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get roles", "Failed", err.Error())
			return reconcile.Result{}, err
		}	
		return reconcile.Result{}, err
	}
	
	policyResource, err := getResource(instance, myAccount, serviceIDAPI)
	if err != nil {
		reqLogger.Info("Error getting resource for authorization policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting resource for authorization policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get resource", "Failed", err.Error())
			return reconcile.Result{}, err
		}	
		return reconcile.Result{}, err
	}

	policySubject, err  := getSubject(instance, myAccount, serviceIDAPI)
	if err != nil {
		reqLogger.Info("Error getting subject for authorization policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting subject for authorization policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get subject", "Failed", err.Error())
			return reconcile.Result{}, err
		}	
		return reconcile.Result{}, err
	}

	var policy = iampapv1.Policy{Roles: policyRoles, Resources: []iampapv1.Resource{policyResource}, Subjects: []iampapv1.Subject{policySubject}}
	policy.Type = iampapv1.AuthorizationPolicyType

	if (statusPolicyID != "") { //Policy must exist in IAM since status has an ID 
		retrievedPolicy, err := policyAPI.Get(statusPolicyID)
		etag := retrievedPolicy.Version
		if err != nil {
			reqLogger.Info("Error retrieving policy", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error retrieving policy"
			instance.Status.PolicyID = ""   //clear out the policy ID since policy with this ID can't be retrieved
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing authorization policy retrieval", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, err
		}	

		if (specChanged(instance) || policyChanged(policy, retrievedPolicy)) { // Spec change or a change via the IAM console means the authorization policy needs an update
			updatedPolicy, err := updateAuthorizationPolicy(statusPolicyID, policy, policyAPI, etag)
			if err != nil {
				reqLogger.Info("Error updating policy", "Failed", err.Error())
				instance.Status.State = "Failed"
				instance.Status.Message = "Error updating policy"
				if err := r.client.Status().Update(context.Background(), instance); err != nil {
					reqLogger.Info("Error updating status for failing authorization policy update", "Failed", err.Error())
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, err
			}
			reqLogger.Info("Updated authorization policy.","Policy ID:",updatedPolicy.ID,"Policy Href:",updatedPolicy.Href)

			instance.Status.State = "Online"
			instance.Status.Message = "IAM authorization policy updated"
			instance.Status.PolicyID = updatedPolicy.ID
			instance.Status.Source = instance.Spec.Source
			instance.Status.Roles = instance.Spec.Roles
			instance.Status.Target = instance.Spec.Target
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for authorization policy update", "Failed", err.Error())
				return reconcile.Result{}, err
			}
		} 
	} else { //Policy doesn't exist in IAM
		createdPolicy, err := createAuthorizationPolicy(policy, policyAPI)
		if err != nil {
			reqLogger.Info("Error creating policy", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error creating policy"
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing authorization policy creation", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}
		reqLogger.Info("Created authorization policy.","Policy ID:",createdPolicy.ID,"Policy Href:",createdPolicy.Href)

		instance.Status.State = "Online"
		instance.Status.Message = "New IAM authorization policy created"
		instance.Status.PolicyID = createdPolicy.ID
		instance.Status.Source = instance.Spec.Source
		instance.Status.Roles = instance.Spec.Roles
		instance.Status.Target = instance.Spec.Target
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for authorization policy creation", "Failed", err.Error())
			errr := deleteAuthorizationPolicy(createdPolicy.ID, policyAPI)
			if errr != nil {	
				if !strings.Contains(errr.Error(), "not found") {
					reqLogger.Info("Error deleting authorization policy", instance.Name, errr.Error())
					return reconcile.Result{}, errr
				}	
			} 
			reqLogger.Info("Deleted authorization policy.","Policy ID:",createdPolicy.ID)
			return reconcile.Result{}, err
		}
	}	
	return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, nil

}

func policyChanged(policy iampapv1.Policy, retrievedPolicy iampapv1.Policy) bool {
	if !reflect.DeepEqual(retrievedPolicy.Subjects, policy.Subjects) {
		log.Info("Authorization policy subject in IAM has changed")
		return true
	}

	for _, m := range retrievedPolicy.Roles {
		if !contains(policy.Roles,m) {
			log.Info("Authorization policy roles in IAM has changed")
			return true
		}
	}

	if !reflect.DeepEqual(retrievedPolicy.Resources, policy.Resources) {
		log.Info("Authorization policy resource in IAM has changed")
		return true
	}	
	return false
}

func contains(policyRoles []iampapv1.Role, e iampapv1.Role) bool {
    for _, a := range policyRoles {
        if reflect.DeepEqual(a.RoleID,e.RoleID) {
            return true
        }
    }
    return false
}

func specChanged(instance *ibmcloudv1alpha1.AuthorizationPolicy) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AuthorizationPolicyStatus{}) { // Object does not have a status field yet
		return false
	}

	if instance.Status.PolicyID == "" { // Object has not been fully created yet
		return false
	}
	if !reflect.DeepEqual(instance.Spec.Source, instance.Status.Source) {
		log.Info("Authorization policy subject in Spec has changed")
		return true
	}
	if !reflect.DeepEqual(instance.Spec.Roles, instance.Status.Roles) {
		log.Info("Authorization policy roles in Spec has changed")
		return true
	}
	if !reflect.DeepEqual(instance.Spec.Target, instance.Status.Target) {
		log.Info("Authorization policy resource in Spec has changed")
		return true
	}	
	return false
}

func createAuthorizationPolicy(policy iampapv1.Policy, policyAPI iampapv1.V1PolicyRepository) (*iampapv1.Policy, error) {
	createdPolicy, err := policyAPI.Create(policy)
	if err != nil {
		return nil, err
	}
	return &createdPolicy, nil
}

func updateAuthorizationPolicy(statusPolicyID string, policy iampapv1.Policy, policyAPI iampapv1.V1PolicyRepository, etag string) (*iampapv1.Policy, error) {
	updatedPolicy, err := policyAPI.Update(statusPolicyID, policy, etag)
	if err != nil {
		return nil, err
	}
	return &updatedPolicy, nil
}

func deleteAuthorizationPolicy(statusPolicyID string, policyAPI iampapv1.V1PolicyRepository) (error) {
	err := policyAPI.Delete(statusPolicyID)
	if err != nil {
		return err
	}
	return nil
}

func getSubject(instance *ibmcloudv1alpha1.AuthorizationPolicy, myAccount *accountv2.Account, serviceIDAPI iamv1.ServiceIDRepository) (iampapv1.Subject, error) {
	/* Getting attributes for Resource */
	policySubject := iampapv1.Subject{}

 	if instance.Spec.Source.ServiceClass != "" {
		policySubject.SetAttribute("serviceName",instance.Spec.Source.ServiceClass)
	}
	if instance.Spec.Source.ServiceID != "" {
		policySubject.SetAttribute("serviceInstance",instance.Spec.Source.ServiceID)
	}  
	if instance.Spec.Source.ResourceName != "" {
		policySubject.SetAttribute("resourceType",instance.Spec.Source.ResourceName)
	} 
	if instance.Spec.Source.ResourceID != "" {
		policySubject.SetAttribute("resource",instance.Spec.Source.ResourceID)
	} 
  	if instance.Spec.Source.ResourceGroup != "" {
		policySubject.SetResourceGroupID(instance.Spec.Source.ResourceGroup)
	}
	if instance.Spec.Source.ResourceKey != "" && instance.Spec.Source.ResourceValue != "" {
		policySubject.SetAttribute(instance.Spec.Source.ResourceKey, instance.Spec.Source.ResourceValue)
	}
	policySubject.SetAttribute("accountId", myAccount.GUID)
	return policySubject, nil
}

func getRoles(instance *ibmcloudv1alpha1.AuthorizationPolicy, serviceRolesAPI iamv1.ServiceRoleRepository) ([]iampapv1.Role, error) {
	/* Getting roles for Source */
	var policyRoles []iampapv1.Role

	definedRoles, err := serviceRolesAPI.ListAuthorizationRoles(instance.Spec.Source.ServiceClass, instance.Spec.Target.ServiceClass)
	if err != nil {
		log.Info("Error getting defined system roles")
		return nil, err
	}

	filterRoles, err := utils.GetRolesFromRoleNames(instance.Spec.Roles, definedRoles)
	if err != nil {
		log.Info("Error getting defined roles in spec")
		return nil, err
	}
	policyRoles = iampapv1.ConvertRoleModels(filterRoles)

	return policyRoles, nil
}

func getResource(instance *ibmcloudv1alpha1.AuthorizationPolicy, myAccount *accountv2.Account, serviceIDAPI iamv1.ServiceIDRepository) (iampapv1.Resource, error) {
	/* Getting attributes for Resource */
	policyResource := iampapv1.Resource{}

 	if instance.Spec.Target.ServiceClass != "" {
		policyResource.SetAttribute("serviceName",instance.Spec.Target.ServiceClass)
	}
	if instance.Spec.Target.ServiceID != "" {
		policyResource.SetAttribute("serviceInstance",instance.Spec.Target.ServiceID)
	}  
	if instance.Spec.Target.ResourceName != "" {
		policyResource.SetAttribute("resourceType",instance.Spec.Target.ResourceName)
	} 
	if instance.Spec.Target.ResourceID != "" {
		policyResource.SetAttribute("resource",instance.Spec.Target.ResourceID)
	} 
  	if instance.Spec.Target.ResourceGroup != "" {
		policyResource.SetResourceGroupID(instance.Spec.Target.ResourceGroup)
	}
	if instance.Spec.Target.ResourceKey != "" && instance.Spec.Target.ResourceValue != "" {
		policyResource.SetAttribute(instance.Spec.Target.ResourceKey, instance.Spec.Target.ResourceValue)
	}
	policyResource.SetAttribute("accountId", myAccount.GUID)
	return policyResource, nil
}

func isWellFormed(instance ibmcloudv1alpha1.AuthorizationPolicy) bool {
	if (instance.Spec.Source.ResourceGroup != "" && instance.Spec.Source.ServiceID != "") {
		return false
	}

	if (instance.Spec.Source.ResourceGroup == "" && instance.Spec.Source.ServiceID == "" && (instance.Spec.Source.ResourceName != "" || instance.Spec.Source.ResourceID != "" || instance.Spec.Source.ResourceKey != "" || instance.Spec.Source.ResourceValue != "")) { 
		return false
	}

	if (instance.Spec.Target.ResourceGroup != "" && instance.Spec.Target.ServiceID != "") {
		return false
	}

	if (instance.Spec.Target.ResourceGroup == "" && instance.Spec.Target.ServiceID == "" && (instance.Spec.Target.ResourceName != "" || instance.Spec.Target.ResourceID != "" || instance.Spec.Target.ResourceKey != "" || instance.Spec.Target.ResourceValue != "")) { 
		return false
	}

	return true
}