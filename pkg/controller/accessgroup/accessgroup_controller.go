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

package accessgroup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
	"errors"

	ibmcloudv1alpha1 "github.ibm.com/seed/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"
	common "github.ibm.com/seed/ibmcloud-iam-operator/pkg/util"

 	"github.com/IBM-Cloud/bluemix-go/api/account/accountv1"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/iam/iamv1"
	"github.com/IBM-Cloud/bluemix-go/api/iamuum/iamuumv2"
	"github.com/IBM-Cloud/bluemix-go/models"

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

var log = logf.Log.WithName("controller_accessgroup")

const accessgroupFinalizer = "accessgroup.ibmcloud.ibm.com"
const syncPeriod = time.Second * 150

// ContainsFinalizer checks if the instance contains accessgroup finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.AccessGroup) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, accessgroupFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete accessgroup finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.AccessGroup) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == accessgroupFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new AccessGroup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAccessGroup{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("accessgroup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AccessGroup
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.AccessGroup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.AccessGroup{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAccessGroup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAccessGroup{}

// ReconcileAccessGroup reconciles a AccessGroup object
type ReconcileAccessGroup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AccessGroup object and makes changes based on the state read
// and what is in the AccessGroup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAccessGroup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Access Group")

	// Fetch the AccessGroup instance
	instance := &ibmcloudv1alpha1.AccessGroup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerror.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Access Group resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Access Group")
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AccessGroupStatus{}) {
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

	statusGroupID := instance.Status.GroupID

	iamClient, err := iamv1.New(sess)
	if err != nil {
		reqLogger.Info("Error creating IAM Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	serviceIDAPI := iamClient.ServiceIds()

	accClient1, err := accountv1.New(sess)
	if err != nil {
		reqLogger.Info("Error creating account Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	accountAPIV1 := accClient1.Accounts()

	iamuumClient, err := iamuumv2.New(sess)
	if err != nil {
		reqLogger.Info("Error creating iamuum Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	accessGroupAPI := iamuumClient.AccessGroup()
	accessGroupMemAPI := iamuumClient.AccessGroupMember()
	
	// Delete if necessary
 	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, accessgroupFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			if (statusGroupID != "") { //Group must exist in IAM since status has an ID 
				err := deleteAccessGroup(statusGroupID, myAccount, accountAPIV1, accessGroupAPI)
				if err != nil {
					if !strings.Contains(err.Error(), "Failed to find") {
						reqLogger.Info("Error deleting access group", instance.Name, err.Error())
						return reconcile.Result{}, err
					}
				}
				reqLogger.Info("Deleted access group.","Group ID:", statusGroupID)
				if instance.Status.State != "Deleted" {
					instance.Status.State = "Deleted"
					instance.Status.Message = "IAM access group deleted"
					instance.Status.GroupID = ""   //clear out the group ID since group with this ID has been deleted
					if err := r.client.Update(context.Background(), instance); err != nil {
						reqLogger.Info("Error updating status for access group deletion", "in deletion", err.Error())
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

	if (statusGroupID != "") { //Group must exist in IAM since status has an ID 
		retrievedGroup, etag, err := accessGroupAPI.Get(statusGroupID)
		if err != nil {
			reqLogger.Info("Error retrieving access group", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error retrieving access group"
			instance.Status.GroupID = ""   //clear out the group ID since group with this ID can't be retrieved
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing access group retrieval", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, err
		}

		retrievedMembers, err := accessGroupMemAPI.List(retrievedGroup.ID)
		if err != nil {
			reqLogger.Info("Error retrieving access group members", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error retrieving access group members"
			instance.Status.GroupID = ""   //clear out the group ID since group members with this ID can't be retrieved
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing access group members retrieval", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, err
		}

		if (specChanged(instance) || groupChanged(instance, retrievedGroup, retrievedMembers, myAccount, accountAPIV1, serviceIDAPI)) { // Spec change or a change via the IAM console means the acccess group needs an update
			updatedgroup, err := updateAccessGroup(instance, myAccount, accountAPIV1, serviceIDAPI, accessGroupAPI, accessGroupMemAPI, etag, retrievedMembers)
			if err != nil {
				reqLogger.Info("Error updating access group", instance.Name, err.Error())
				instance.Status.State = "Failed"
				instance.Status.Message = "Error updating access group"
				if err := r.client.Status().Update(context.Background(), instance); err != nil {
					reqLogger.Info("Error updating status for failing access group update", "Failed", err.Error())
					//TODO ??? delete access group
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, err
			}
			reqLogger.Info("Updated access group.","Group ID:",statusGroupID)

			instance.Status.State = "Online"
			instance.Status.Message = "IAM access group updated"
			instance.Status.GroupID = updatedgroup.ID
			instance.Status.Name = instance.Spec.Name
			instance.Status.Description = instance.Spec.Description
			instance.Status.UserEmails = instance.Spec.UserEmails
			instance.Status.ServiceIDs = instance.Spec.ServiceIDs
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for access group update", "Failed", err.Error())
				//TODO ??? delete access group
				return reconcile.Result{}, err
			}
		} 
	} else { //Group doesn't exist in IAM
		createdGroup, err := createAccessGroup(instance, myAccount, accountAPIV1, serviceIDAPI, accessGroupAPI, accessGroupMemAPI)
		if err != nil {
			reqLogger.Info("Error creating access group", instance.Name, err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error creating access group"
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing access group creation", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}
		reqLogger.Info("Created access group.","Group ID:",createdGroup.ID)

		instance.Status.State = "Online"
		instance.Status.Message = "New IAM access group created"
		instance.Status.GroupID = createdGroup.ID
		instance.Status.Name = instance.Spec.Name
		instance.Status.Description = instance.Spec.Description
		instance.Status.UserEmails = instance.Spec.UserEmails
		instance.Status.ServiceIDs = instance.Spec.ServiceIDs
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for access group creation", "Failed", err.Error())
			errr := deleteAccessGroup(createdGroup.ID, myAccount, accountAPIV1, accessGroupAPI)
 			if errr != nil {
				if !strings.Contains(errr.Error(), "Failed to find") {
					reqLogger.Info("Error deleting access group", instance.Name, errr.Error())
					return reconcile.Result{}, errr
				}
			} 
			reqLogger.Info("Deleted access group.","Group ID:", createdGroup.ID)
			return reconcile.Result{}, err
		}
	}	
	return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

func groupChanged(instance *ibmcloudv1alpha1.AccessGroup, retrievedGroup *models.AccessGroupV2, retrievedMembers []models.AccessGroupMemberV2, myAccount *accountv2.Account, accountAPIV1 accountv1.Accounts, serviceIDAPI iamv1.ServiceIDRepository) bool {
	description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
	if !reflect.DeepEqual(retrievedGroup.AccessGroup.Name,instance.Spec.Name) {
		log.Info("Access group name in IAM has changed")
		return true
	}

	if !reflect.DeepEqual(retrievedGroup.AccessGroup.Description, description) {
		log.Info("Access group description in IAM has changed")
		return true
	}

	var specMembers []models.AccessGroupMemberV2
	for _, element := range instance.Spec.UserEmails {	
		userDetails, _ := accountAPIV1.FindAccountUserByUserId(myAccount.GUID, element)

		if userDetails != nil && userDetails.UserId != "" && userDetails.IbmUniqueId != "" && userDetails.State != "PENDING" {
			grpmem := models.AccessGroupMemberV2{
				ID:   userDetails.IbmUniqueId,
				Type: iamuumv2.AccessGroupMemberUser,
			}
			specMembers = append(specMembers, grpmem)
		}
	}	

	for _, element := range instance.Spec.ServiceIDs {
		sID, _ := serviceIDAPI.Get(element)
		grpmem := models.AccessGroupMemberV2{
			ID:   sID.IAMID,
			Type: iamuumv2.AccessGroupMemberService,
		}
		specMembers= append(specMembers, grpmem)
	}	

	if (len(specMembers) != len(retrievedMembers)) {
		return true
	}
	for _, m := range specMembers {
		if !contains(retrievedMembers,m) {
			log.Info("Access group members in IAM has changed")
			return true
		}
	}

	return false
}

func specChanged(instance *ibmcloudv1alpha1.AccessGroup) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AccessGroupStatus{}) { // Object does not have a status field yet
		return false
	}

	if instance.Status.GroupID == "" { // Object has not been fully created yet
		return false
	}
	if !reflect.DeepEqual(instance.Spec.Name,instance.Status.Name) {
		log.Info("Access group name in Spec has changed")
		return true
	}

	if !reflect.DeepEqual(instance.Spec.Description,instance.Status.Description) {
		log.Info("Access group description in Spec has changed")
		return true
	}

	if !reflect.DeepEqual(instance.Spec.UserEmails,instance.Status.UserEmails) {
		log.Info("Access group user members in Spec has changed")
		return true
	}
	
	if !reflect.DeepEqual(instance.Spec.ServiceIDs,instance.Status.ServiceIDs) {
		log.Info("Access group service members in Spec has changed")
		return true
	}	

	return false
}

func createAccessGroup(instance *ibmcloudv1alpha1.AccessGroup, myAccount *accountv2.Account, accountAPIV1 accountv1.Accounts, serviceIDAPI iamv1.ServiceIDRepository, accessGroupAPI iamuumv2.AccessGroupRepository, accessGroupMemAPI iamuumv2.AccessGroupMemberRepositoryV2) (*models.AccessGroupV2, error) {
	var newaccessgroup *models.AccessGroupV2

	accessgroups, err := accessGroupAPI.FindByName(instance.Spec.Name, myAccount.GUID)
	if len(accessgroups) == 0 { //Access group by that name does not exist so create it
		description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
		data := models.AccessGroupV2{
			AccessGroup: models.AccessGroup{
				Name: instance.Spec.Name,
				Description: description,
			},
			AccountID: 	 myAccount.GUID,
		}
		newaccessgroup, err = accessGroupAPI.Create(data, myAccount.GUID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("Access group with the same name already exists.")
	}

	var members []models.AccessGroupMemberV2
	log.Info("Adding members to the new Access Group")
	for _, element := range instance.Spec.UserEmails {
		_, err := accountAPIV1.InviteAccountUser(myAccount.GUID, element)
		if err != nil {
			_ = accessGroupAPI.Delete(newaccessgroup.ID,true)									
			return nil, err
		}

		userDetails, err := accountAPIV1.FindAccountUserByUserId(myAccount.GUID, element)
		if err != nil {
			_ = accessGroupAPI.Delete(newaccessgroup.ID,true)				
			return nil, err
		}

		if userDetails == nil || userDetails.UserId == "" || userDetails.IbmUniqueId == "" || userDetails.State == "PENDING" {
			err = accountAPIV1.DeleteAccountUser(myAccount.GUID, userDetails.Id)
			err = accessGroupAPI.Delete(newaccessgroup.ID,true)				
			return nil, errors.New("User email is not valid.")
		}

		grpmem1 := models.AccessGroupMemberV2{
			ID:   userDetails.IbmUniqueId,
			Type: iamuumv2.AccessGroupMemberUser,
		}

		members = append(members, grpmem1)
	}	

	for _, element := range instance.Spec.ServiceIDs {
		sID, err := serviceIDAPI.Get(element)
		if err != nil {
			_ = accessGroupAPI.Delete(newaccessgroup.ID,true)	
			return nil, err
		}
		grpmem2 := models.AccessGroupMemberV2{
			ID:   sID.IAMID,
			Type: iamuumv2.AccessGroupMemberService,
		}

		members = append(members, grpmem2)
	}	

	//Add members from Spec to access group
	addRequest := iamuumv2.AddGroupMemberRequestV2{
		Members: members,
	}
	accessGroupMemAPI.Add(newaccessgroup.ID, addRequest)

	return newaccessgroup, nil
}

func updateAccessGroup(instance *ibmcloudv1alpha1.AccessGroup, myAccount *accountv2.Account, accountAPIV1 accountv1.Accounts, serviceIDAPI iamv1.ServiceIDRepository, accessGroupAPI iamuumv2.AccessGroupRepository, accessGroupMemAPI iamuumv2.AccessGroupMemberRepositoryV2, etag string, currentMembers []models.AccessGroupMemberV2) (*models.AccessGroupV2, error) {
	accessgroupID := instance.Status.GroupID
	description := "OPERATOR OWNED: "+instance.Spec.Description //Adding Operator owned TAG
	data := iamuumv2.AccessGroupUpdateRequest {
			Name: instance.Spec.Name,
			Description: description,
	}
	accessgroup, err := accessGroupAPI.Update(accessgroupID, data, etag)
	if err != nil {
		return nil, err
	}

	var newMembers []models.AccessGroupMemberV2
	for _, element := range instance.Spec.UserEmails {
		_, err := accountAPIV1.InviteAccountUser(myAccount.GUID, element)
		if err != nil {									
			return nil, err
		}

		userDetails, err := accountAPIV1.FindAccountUserByUserId(myAccount.GUID, element)
		if err != nil {			
			return nil, err
		}

		if userDetails == nil || userDetails.UserId == "" || userDetails.IbmUniqueId == "" || userDetails.State == "PENDING" {
			err = accountAPIV1.DeleteAccountUser(myAccount.GUID, userDetails.Id)				
			return nil, errors.New("User email is not valid:"+element)
		}

		grpmem1 := models.AccessGroupMemberV2{
			ID:   userDetails.IbmUniqueId,
			Type: iamuumv2.AccessGroupMemberUser,
		}

		newMembers = append(newMembers, grpmem1)
	}	

	for _, element := range instance.Spec.ServiceIDs {
		sID, err := serviceIDAPI.Get(element)
		if err != nil {	
			return nil, errors.New("Service ID is not valid:"+element)
		}
		grpmem2 := models.AccessGroupMemberV2{
			ID:   sID.IAMID,
			Type: iamuumv2.AccessGroupMemberService,
		}

		newMembers= append(newMembers, grpmem2)
	}	
	
	//First, Remove members from access group that are not in the new Spec
	for _, m := range currentMembers {
		if !contains(newMembers,m) {
			if (reflect.DeepEqual(m.Type, iamuumv2.AccessGroupMemberUser)) {
				//TODO: remove user from Account? What if they're a member of another group in the same account?
			}
			accessGroupMemAPI.Remove(accessgroupID, m.ID)
		}
	}

	//Second, Add new members from Spec to access group
	var newCurrentMembers []models.AccessGroupMemberV2
	for _, m := range newMembers {
		if !contains(currentMembers,m) {
			newCurrentMembers= append(newCurrentMembers, m)
		}
	}
	addRequest := iamuumv2.AddGroupMemberRequestV2{
		Members: newCurrentMembers,
	}
	accessGroupMemAPI.Add(accessgroupID, addRequest)

	return &accessgroup, nil
}

func deleteAccessGroup(accessgroupID string, myAccount *accountv2.Account, accountAPIV1 accountv1.Accounts, accessGroupAPI iamuumv2.AccessGroupRepository) (error) {
	/* 	TODO: What if user member of another group? Cannot delete user from account in that case...
	for _, element := range instance.Spec.UserEmails {
		userDetails, err := accountAPIV1.FindAccountUserByUserId(myAccount.GUID, element)
		if err != nil {
			return err
		}

		if (myAccount.OwnerUserID != element) { //cannot delete account owner
			err = accountAPIV1.DeleteAccountUser(myAccount.GUID, userDetails.Id)
			if err != nil {
				return err
			}
		}
	} */

	err := accessGroupAPI.Delete(accessgroupID, true)
	if err != nil {
		return err
	}
	return nil
}

func contains(s []models.AccessGroupMemberV2, e models.AccessGroupMemberV2) bool {
    for _, a := range s {
        if reflect.DeepEqual(a.ID,e.ID) {
            return true
        }
    }
    return false
}

func isWellFormed(instance ibmcloudv1alpha1.AccessGroup) bool {
	if instance.Spec.Name != "" && (instance.Spec.UserEmails == nil && instance.Spec.ServiceIDs == nil) {
		return false
	}
	return true
}