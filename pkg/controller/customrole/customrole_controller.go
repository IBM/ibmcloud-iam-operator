/*

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

package customrole

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
	"errors"

	ibmcloudv1alpha1 "github.com/IBM/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"
	common "github.com/IBM/ibmcloud-iam-operator/pkg/util"

	"github.com/IBM-Cloud/bluemix-go/api/iampap/iampapv2"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"

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

var log = logf.Log.WithName("controller_customrole")

const customroleFinalizer = "customrole.ibmcloud.ibm.com"
const syncPeriod = time.Second * 150

// ContainsFinalizer checks if the instance contains customrole finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.CustomRole) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, customroleFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete customrole finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.CustomRole) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == customroleFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new CustomRole Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCustomRole{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("customrole-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CustomRole
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.CustomRole{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.CustomRole{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileCustomRole implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCustomRole{}

// ReconcileCustomRole reconciles a CustomRole object
type ReconcileCustomRole struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CustomRole object and makes changes based on the state read
// and what is in the CustomRole.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCustomRole) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Custom Role")

	// Fetch the CustomRole instance
	instance := &ibmcloudv1alpha1.CustomRole{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerror.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Custom Role resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Custom Role")
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.CustomRoleStatus{}) {
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

	// Enforce immutability for role name and service class, restore the spec if it has changed
	if immutableSpecChanged(instance) {
		reqLogger.Info("Role Name and Service Class are immutable", "Restoring", instance.ObjectMeta.Name)
		instance.Spec.RoleName = instance.Status.RoleName
		instance.Spec.ServiceClass = instance.Status.ServiceClass
		if err := r.client.Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for immutable spec", "Failed", err.Error())
			return reconcile.Result{}, err
		}
	}

	sess, myAccount, err := common.GetIAMAccountInfo(r.client, instance.ObjectMeta.Namespace)
	if err != nil {
		reqLogger.Info("Error getting IBM Cloud IAM account information", instance.Name, err.Error())
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error removing finalizers", "in deletion", err.Error())
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

	statusRoleID := instance.Status.RoleID

	roleClient, err := iampapv2.New(sess)
	if err != nil {
		reqLogger.Info("Error creating Role Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	customRoleAPI := roleClient.IAMRoles()
	
	// Delete if necessary
 	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, customroleFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			if (statusRoleID != "") { //Role must exist in IAM since status has an ID 
				err := deleteCustomRole(statusRoleID, customRoleAPI)
				if err != nil {
					if !strings.Contains(err.Error(), "not found") {
						reqLogger.Info("Error deleting custom role", instance.Name, err.Error())
						return reconcile.Result{}, err
					}
				}
				reqLogger.Info("Deleted custom role.","Role ID:",statusRoleID)
				if instance.Status.State != "Deleted" {
					instance.Status.State = "Deleted"
					instance.Status.Message = "IAM custom role deleted"
					instance.Status.RoleID = ""   //clear out the role ID since role with this ID has been deleted
					if err := r.client.Update(context.Background(), instance); err != nil {
						reqLogger.Info("Error updating status for custom role deletion", "in deletion", err.Error())
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

	if (statusRoleID != "") { //Role must exist in IAM since status has an ID 	
		retrievedRole, etag, err := customRoleAPI.Get(statusRoleID)
		if err != nil {
			reqLogger.Info("Error retrieving custom role", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error retrieving custom role"
			instance.Status.RoleID = ""   //clear out the role ID since role with this ID can't be retrieved
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing custom role retrieval", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, err
		}
		
		if (mutableSpecChanged(instance) || roleChanged(instance, retrievedRole)) { // Spec change or a change via the IAM console means the custom role needs an update
			updatedRole, err := updateCustomRole(instance, customRoleAPI, etag)
			if err != nil {
				reqLogger.Info("Error updating custom role", instance.Name, err.Error())
				instance.Status.State = "Failed"
				instance.Status.Message = "Error updating custom role"
				if err := r.client.Status().Update(context.Background(), instance); err != nil {
					reqLogger.Info("Error updating status for failing custom role update", "Failed", err.Error())
					//TODO ??? delete custom role?
					return reconcile.Result{}, err
				}				
				return reconcile.Result{}, err
			}
			reqLogger.Info("Updated custom role.","Role ID:",statusRoleID)

			instance.Status.State = "Online"
			instance.Status.Message = "IAM custom role updated"
			instance.Status.RoleID = updatedRole.ID
			instance.Status.RoleCRN = updatedRole.Crn
			instance.Status.DisplayName = instance.Spec.DisplayName
			instance.Status.Description = instance.Spec.Description
			instance.Status.Actions = instance.Spec.Actions
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for custom role update", "Failed", err.Error())
				//TODO ??? delete custom role?
				return reconcile.Result{}, err
			}
		} 
	} else { //Role doesn't exist in IAM
		createdRole, err := createCustomRole(instance, myAccount, customRoleAPI)
		if err != nil {
			reqLogger.Info("Error creating custom role", instance.Name, err.Error())	
			instance.Status.State = "Failed"
			instance.Status.Message = "Error creating custom role"
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing custom role creation", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}
		reqLogger.Info("Created custom role.","Role ID:",createdRole.ID)

		instance.Status.State = "Online"
		instance.Status.Message = "New IAM custom role created"
		instance.Status.RoleID = createdRole.ID
		instance.Status.RoleCRN = createdRole.Crn
		instance.Status.RoleName = instance.Spec.RoleName
		instance.Status.ServiceClass = instance.Spec.ServiceClass
		instance.Status.DisplayName = instance.Spec.DisplayName
		instance.Status.Description = instance.Spec.Description
		instance.Status.Actions = instance.Spec.Actions
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for custom role creation", "Failed", err.Error())
			errr := deleteCustomRole(createdRole.ID, customRoleAPI)
			if errr != nil {
				if !strings.Contains(errr.Error(), "not found") {
					reqLogger.Info("Error deleting custom role", instance.Name, errr.Error())
					return reconcile.Result{}, errr
				}
			}
			reqLogger.Info("Deleted custom role.","Role ID:", createdRole.ID)
			return reconcile.Result{}, err
		}
	}	
	return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

func roleChanged(instance *ibmcloudv1alpha1.CustomRole, retrievedRole iampapv2.Role) bool {
	description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
	if !reflect.DeepEqual(retrievedRole.CreateRoleRequest.DisplayName,instance.Spec.DisplayName) {
		log.Info("Custom role display name in IAM has changed")
		return true
	}

	if !reflect.DeepEqual(retrievedRole.CreateRoleRequest.Description,description) {
		log.Info("Custom role description in IAM has changed")
		return true
	}

	if !reflect.DeepEqual(retrievedRole.CreateRoleRequest.Actions,instance.Spec.Actions) {
		log.Info("Custom role actions in IAM has changed")
		return true
	}

	//Cannot update other fields of a Custom Role spec
	return false
}

func mutableSpecChanged(instance *ibmcloudv1alpha1.CustomRole) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.CustomRoleStatus{}) { // Object does not have a status field yet
		return false
	}
	
	if instance.Status.RoleID == "" { // Object has not been fully created yet
		return false
	}

	if !reflect.DeepEqual(instance.Spec.DisplayName,instance.Status.DisplayName) {
		log.Info("Custom role display name in Spec has changed")
		return true
	}

	if !reflect.DeepEqual(instance.Spec.Description,instance.Status.Description) {
		log.Info("Custom role description in Spec has changed")
		return true
	}

	if !reflect.DeepEqual(instance.Spec.Actions,instance.Status.Actions) {
		log.Info("Custom role actions in Spec has changed")
		return true
	}

	//Cannot update other fields of a Custom Role spec
	return false
}

func immutableSpecChanged(instance *ibmcloudv1alpha1.CustomRole) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AccessPolicyStatus{}) { // Object does not have a status field yet
		return false
	}

	if instance.Status.RoleID == "" { // Object has not been fully initialized yet
		return false
	}

	if !reflect.DeepEqual(instance.Spec.RoleName,instance.Status.RoleName) {
		log.Info("Custom role role name in Spec has changed.")
		return true
	}

	if !reflect.DeepEqual(instance.Spec.ServiceClass,instance.Status.ServiceClass) {
		log.Info("Custom role service class in Spec has changed")
		return true
	}

	return false
}

func createCustomRole(instance *ibmcloudv1alpha1.CustomRole, myAccount *accountv2.Account, customRoleAPI iampapv2.RoleRepository) (*iampapv2.Role, error) {
	description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
	roleReq := iampapv2.CreateRoleRequest{
		Name:        instance.Spec.RoleName,
		ServiceName: instance.Spec.ServiceClass,
		AccountID: 	 myAccount.GUID,
		DisplayName: instance.Spec.DisplayName,
		Description: description,
		Actions:     instance.Spec.Actions,
	}
	
	listresp, err := customRoleAPI.ListCustomRoles(myAccount.GUID, instance.Spec.ServiceClass)
	if err != nil {
		return nil, err
	}

	for _, element := range listresp {
		if (reflect.DeepEqual(roleReq,element.CreateRoleRequest)) {
			return nil, errors.New("Custom role with the same name already exists.")
		}
	}
	//Custom role by that name does not exist so create it
	customrole, err := customRoleAPI.Create(roleReq)
	if err != nil {
		return nil, err
	}
	
	return &customrole, nil
}

func updateCustomRole(instance *ibmcloudv1alpha1.CustomRole, customRoleAPI iampapv2.RoleRepository, etag string) (*iampapv2.Role, error) {
	customroleID := instance.Status.RoleID
	description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
	updateReq := iampapv2.UpdateRoleRequest{
		DisplayName: instance.Spec.DisplayName,
		Description: description,
		Actions: instance.Spec.Actions,
	}

	updatedRole, err := customRoleAPI.Update(updateReq, customroleID, etag)
	if err != nil {
		return nil, err
	}

	return &updatedRole, nil
}

func deleteCustomRole(customroleID string, customRoleAPI iampapv2.RoleRepository) (error) {
	err := customRoleAPI.Delete(customroleID)
	if err != nil {
		return err
	}
	
	return nil
}

func isWellFormed(instance ibmcloudv1alpha1.CustomRole) bool {
	if instance.Spec.RoleName == "" { //TODO:Improve checks
		return false
	}
	return true
}