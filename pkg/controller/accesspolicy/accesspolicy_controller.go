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

package accesspolicy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	ibmcloudv1alpha1 "github.com/IBM/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"
	common "github.com/IBM/ibmcloud-iam-operator/pkg/util"

	"github.com/IBM-Cloud/bluemix-go/api/account/accountv1"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/iam/iamv1"
	"github.com/IBM-Cloud/bluemix-go/api/iampap/iampapv1"
	"github.com/IBM-Cloud/bluemix-go/api/iampap/iampapv2"
	"github.com/IBM-Cloud/bluemix-go/api/iamuum/iamuumv2"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/utils"

	v1 "k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_accesspolicy")

const accesspolicyFinalizer = "accesspolicy.ibmcloud.ibm.com"
const syncPeriod = time.Second * 150

// ContainsFinalizer checks if the instance contains accesspolicy finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.AccessPolicy) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, accesspolicyFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete accesspolicy finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.AccessPolicy) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == accesspolicyFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new AccessPolicy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAccessPolicy{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("accesspolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AccessPolicy
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.AccessPolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloudv1alpha1.AccessPolicy{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAccessPolicy implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAccessPolicy{}

// ReconcileAccessPolicy reconciles a AccessPolicy object
type ReconcileAccessPolicy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AccessPolicy object and makes changes based on the state read
// and what is in the AccessPolicy.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAccessPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Access Policy")

	// Fetch the AccessPolicy instance
	instance := &ibmcloudv1alpha1.AccessPolicy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if kerror.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Access Policy resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Access Policy")
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AccessPolicyStatus{}) {
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

	roleClient, err := iampapv2.New(sess)
	if err != nil {
		reqLogger.Info("Error creating Role Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	customRolesAPI := roleClient.IAMRoles()

	accClient1, err := accountv1.New(sess)
	if err != nil {
		reqLogger.Info("Error getting account Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	accountAPIV1 := accClient1.Accounts()

	iamuumClient, err := iamuumv2.New(sess)
	if err != nil {
		reqLogger.Info("Error getting iamuum Client", instance.Name, err.Error())
		return reconcile.Result{}, err
	}
	accessGroupAPI := iamuumClient.AccessGroup()

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, accesspolicyFinalizer)
			if err := r.client.Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			if statusPolicyID != "" { //Policy must exist in IAM since status has an ID
				err := deleteAccessPolicy(statusPolicyID, policyAPI)
				if err != nil {
					if !strings.Contains(err.Error(), "not found") {
						reqLogger.Info("Error deleting access policy", instance.Name, err.Error())
						return reconcile.Result{}, err
					}
				}
				reqLogger.Info("Deleted access policy.", "Policy ID:", statusPolicyID)
				if instance.Status.State != "Deleted" {
					instance.Status.State = "Deleted"
					instance.Status.Message = "IAM access policy deleted"
					instance.Status.PolicyID = "" //clear out the policy ID since policy with this ID has been deleted
					if err := r.client.Update(context.Background(), instance); err != nil {
						reqLogger.Info("Error updating status for access policy deletion", "in deletion", err.Error())
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
	policyRoles, err := getRoles(instance, r, myAccount, serviceRolesAPI, customRolesAPI)
	if err != nil {
		reqLogger.Info("Error getting roles for access policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting roles for access policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get roles", "Failed", err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	policyResource, err := getResource(instance, serviceIDAPI)
	if err != nil {
		reqLogger.Info("Error getting resource for access policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting resource for access policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get resource", "Failed", err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	policySubject, err := getSubject(instance, r, myAccount, accountAPIV1, serviceIDAPI, accessGroupAPI)
	if err != nil {
		reqLogger.Info("Error getting subject for access policy", "Failed", err.Error())
		instance.Status.State = "Failed"
		instance.Status.Message = "Error getting subject for access policy"
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for failing get subject", "Failed", err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	var policy = iampapv1.Policy{Roles: policyRoles, Resources: []iampapv1.Resource{policyResource}}
	policy.Resources[0].SetAccountID(myAccount.GUID)
	policy.Type = iampapv1.AccessPolicyType
	policy.Subjects = policySubject

	if statusPolicyID != "" { //Policy must exist in IAM since status has an ID
		retrievedPolicy, err := policyAPI.Get(statusPolicyID)
		etag := retrievedPolicy.Version
		if err != nil {
			reqLogger.Info("Error retrieving policy", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error retrieving policy"
			instance.Status.PolicyID = "" //clear out the policy ID since policy with this ID can't be retrieved
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing access policy retrieval", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, err
		}

		if specChanged(instance) || policyChanged(policy, retrievedPolicy) { // Spec change or a change via the IAM console means the acccess policy needs an update
			updatedPolicy, err := updateAccessPolicy(statusPolicyID, policy, policyAPI, etag)
			if err != nil {
				reqLogger.Info("Error updating policy", "Failed", err.Error())
				instance.Status.State = "Failed"
				instance.Status.Message = "Error updating policy"
				if err := r.client.Status().Update(context.Background(), instance); err != nil {
					reqLogger.Info("Error updating status for failing access policy update", "Failed", err.Error())
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, err
			}
			reqLogger.Info("Updated access policy.", "Policy ID:", updatedPolicy.ID, "Policy Href:", updatedPolicy.Href)

			instance.Status.State = "Online"
			instance.Status.Message = "IAM access policy updated"
			instance.Status.PolicyID = updatedPolicy.ID
			instance.Status.Subject = instance.Spec.Subject
			instance.Status.Roles = instance.Spec.Roles
			instance.Status.Target = instance.Spec.Target
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for access policy update", "Failed", err.Error())
				return reconcile.Result{}, err
			}
		}
	} else { //Policy doesn't exist in IAM
		createdPolicy, err := createAccessPolicy(policy, policyAPI)
		if err != nil {
			reqLogger.Info("Error creating policy", "Failed", err.Error())
			instance.Status.State = "Failed"
			instance.Status.Message = "Error creating policy"
			if err := r.client.Status().Update(context.Background(), instance); err != nil {
				reqLogger.Info("Error updating status for failing access policy creation", "Failed", err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}
		reqLogger.Info("Created access policy.", "Policy ID:", createdPolicy.ID, "Policy Href:", createdPolicy.Href)

		instance.Status.State = "Online"
		instance.Status.Message = "New IAM access policy created"
		instance.Status.PolicyID = createdPolicy.ID
		instance.Status.Subject = instance.Spec.Subject
		instance.Status.Roles = instance.Spec.Roles
		instance.Status.Target = instance.Spec.Target
		if err := r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Info("Error updating status for access policy creation", "Failed", err.Error())
			errr := deleteAccessPolicy(createdPolicy.ID, policyAPI)
			if errr != nil {
				if !strings.Contains(errr.Error(), "not found") {
					reqLogger.Info("Error deleting access policy", instance.Name, errr.Error())
					return reconcile.Result{}, errr
				}
			}
			reqLogger.Info("Deleted access policy.", "Policy ID:", createdPolicy.ID)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{Requeue: true, RequeueAfter: syncPeriod}, nil
}

func policyChanged(policy iampapv1.Policy, retrievedPolicy iampapv1.Policy) bool {
	if !reflect.DeepEqual(retrievedPolicy.Subjects, policy.Subjects) {
		log.Info("Access policy subject in IAM has changed")
		return true
	}

	for _, m := range retrievedPolicy.Roles {
		if !contains(policy.Roles, m) {
			log.Info("Access policy roles in IAM has changed")
			return true
		}
	}

	if !reflect.DeepEqual(retrievedPolicy.Resources, policy.Resources) {
		log.Info("Access policy resource in IAM has changed")
		return true
	}
	return false
}

func contains(policyRoles []iampapv1.Role, e iampapv1.Role) bool {
	for _, a := range policyRoles {
		if reflect.DeepEqual(a.RoleID, e.RoleID) {
			return true
		}
	}
	return false
}

func specChanged(instance *ibmcloudv1alpha1.AccessPolicy) bool {
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.AccessPolicyStatus{}) { // Object does not have a status field yet
		return false
	}

	if instance.Status.PolicyID == "" { // Object has not been fully created yet
		return false
	}
	if !reflect.DeepEqual(instance.Spec.Subject, instance.Status.Subject) {
		log.Info("Access policy subject in Spec has changed")
		return true
	}
	if !reflect.DeepEqual(instance.Spec.Roles, instance.Status.Roles) {
		log.Info("Access policy roles in Spec has changed")
		return true
	}
	if !reflect.DeepEqual(instance.Spec.Target, instance.Status.Target) {
		log.Info("Access policy resource in Spec has changed")
		return true
	}
	return false
}

func createAccessPolicy(policy iampapv1.Policy, policyAPI iampapv1.V1PolicyRepository) (*iampapv1.Policy, error) {
	createdPolicy, err := policyAPI.Create(policy)
	if err != nil {
		return nil, err
	}
	return &createdPolicy, nil
}

func updateAccessPolicy(statusPolicyID string, policy iampapv1.Policy, policyAPI iampapv1.V1PolicyRepository, etag string) (*iampapv1.Policy, error) {
	updatedPolicy, err := policyAPI.Update(statusPolicyID, policy, etag)
	if err != nil {
		return nil, err
	}
	return &updatedPolicy, nil
}

func deleteAccessPolicy(statusPolicyID string, policyAPI iampapv1.V1PolicyRepository) error {
	err := policyAPI.Delete(statusPolicyID)
	if err != nil {
		return err
	}

	//TODO: Should user be removed from account after policy deletion? What if user is a member of another group or policy?
	/*     if instance.Spec.Subject.UserEmail != "" {
		userDetails, err := accountAPIV1.FindAccountUserByUserId(myAccount.GUID, instance.Spec.Subject.UserEmail)
		if err != nil {
			return err
		}

		if (myAccount.OwnerUserID != instance.Spec.Subject.UserEmail) { //cannot delete account owner
			err = accountAPIV1.DeleteAccountUser(myAccount.GUID, userDetails.Id)
			if err != nil {
				return err
			}
		}
	} */
	return nil
}

func getSubject(instance *ibmcloudv1alpha1.AccessPolicy, r *ReconcileAccessPolicy, myAccount *accountv2.Account, account accountv1.Accounts, serviceIDAPI iamv1.ServiceIDRepository, accessGroupAPI iamuumv2.AccessGroupRepository) ([]iampapv1.Subject, error) {
	/* Getting attributes for Subject depending on fields set in Subject spec */
	if instance.Spec.Subject.UserEmail != "" {
		_, err := account.InviteAccountUser(myAccount.GUID, instance.Spec.Subject.UserEmail)
		if err != nil {
			return nil, err
		}

		userDetails, err := account.FindAccountUserByUserId(myAccount.GUID, instance.Spec.Subject.UserEmail)
		if err != nil {
			return nil, err
		}

		if userDetails == nil || userDetails.Id == "" {
			return nil, errors.New("User email is not valid.")
		}

		if (userDetails.UserId == "" || userDetails.IbmUniqueId == "" || userDetails.State == "PENDING") && (userDetails.Id != "") {
			err = account.DeleteAccountUser(myAccount.GUID, userDetails.Id)
			if err != nil {
				return nil, err
			}
			return nil, errors.New("User email is not valid.")
		}

		return []iampapv1.Subject{
			{
				Attributes: []iampapv1.Attribute{
					{
						Name:  "iam_id",
						Value: userDetails.IbmUniqueId,
					},
				},
			},
		}, nil
	} else if instance.Spec.Subject.ServiceID != "" {

		sID, err := serviceIDAPI.Get(instance.Spec.Subject.ServiceID)
		if err != nil {
			return nil, err
		}

		return []iampapv1.Subject{
			{
				Attributes: []iampapv1.Attribute{
					{
						Name:  "iam_id",
						Value: sID.IAMID,
					},
				},
			},
		}, nil
	} else if instance.Spec.Subject.AccessGroupID != "" {

		ags, _, err := accessGroupAPI.Get(instance.Spec.Subject.AccessGroupID)
		if err != nil {
			return nil, err
		}

		return []iampapv1.Subject{
			{
				Attributes: []iampapv1.Attribute{
					{
						Name:  "access_group_id",
						Value: ags.ID,
					},
				},
			},
		}, nil
	} else if &instance.Spec.Subject.AccessGroupDef != nil {
		accessgroup, err := r.getAccessGroupInstance(instance)
		if err != nil {
			log.Info("Access Policy could not read access group", instance.Spec.Subject.AccessGroupDef.AccessGroupName, err.Error())
			return nil, err
		}

		return []iampapv1.Subject{
			{
				Attributes: []iampapv1.Attribute{
					{
						Name:  "access_group_id",
						Value: accessgroup.Status.GroupID,
					},
				},
			},
		}, nil
	}
	return nil, nil
}

func getRoles(instance *ibmcloudv1alpha1.AccessPolicy, r *ReconcileAccessPolicy, myAccount *accountv2.Account, serviceRolesAPI iamv1.ServiceRoleRepository, customRolesAPI iampapv2.RoleRepository) ([]iampapv1.Role, error) {
	/* Getting roles for Subject */
	var policyRoles []iampapv1.Role

	if instance.Spec.Roles.DefinedRoles != nil {
		//log.Info("Spec contains defined roles")
		var definedRoles []models.PolicyRole
		var err error
		if instance.Spec.Target.ServiceClass == "" {
			definedRoles, err = serviceRolesAPI.ListSystemDefinedRoles()
			if err != nil {
				log.Info("Error getting defined system roles")
				return nil, err
			}
		} else {
			definedRoles, err = serviceRolesAPI.ListServiceRoles(instance.Spec.Target.ServiceClass)
			if err != nil {
				log.Info("Error getting defined system roles")
				return nil, err
			}
		}

		filterDRoles, err := utils.GetRolesFromRoleNames(instance.Spec.Roles.DefinedRoles, definedRoles)
		if err != nil {
			log.Info("Error getting defined roles in spec")
			return nil, err
		}
		policyRoles = iampapv1.ConvertRoleModels(filterDRoles)
	}

	if instance.Spec.Roles.CustomRolesDName != nil {
		//log.Info("Spec contains non-operator managed custom roles")
		customCRoles, err := customRolesAPI.ListCustomRoles(myAccount.GUID, "")
		if err != nil {
			log.Info("Error getting custom roles")
			return nil, err
		}

		filterCRoles, err := utils.GetRolesFromRoleNamesV2(instance.Spec.Roles.CustomRolesDName, customCRoles)
		if err != nil {
			log.Info("Error getting custom roles in spec")
			return nil, err
		}
		policyRoles = append(policyRoles, iampapv1.ConvertV2RoleModels(filterCRoles)...)
	}

	if instance.Spec.Roles.CustomRolesDef != nil {
		//log.Info("Spec contains operator managed custom roles")
		var myCustomRoles []string
		var customDefRoles []iampapv2.Role
		for _, element := range instance.Spec.Roles.CustomRolesDef {
			customRole, err := r.getCustomRoleInstance(instance, &element)
			if err != nil {
				log.Info("Access Policy could not read custom role", element.CustomRoleName, err.Error())
				return nil, err
			}
			myCustomRoles = append(myCustomRoles, customRole.Spec.DisplayName)

			var croles []iampapv2.Role
			croles, err = customRolesAPI.ListAll(iampapv2.RoleQuery{AccountID: myAccount.GUID, ServiceName: customRole.Spec.ServiceClass})
			if err != nil {
				log.Info("Error getting custom system roles")
				return nil, err
			}
			customDefRoles = append(customDefRoles, croles...)
		}

		filterDefRoles, err := utils.GetRolesFromRoleNamesV2(myCustomRoles, customDefRoles)
		if err != nil {
			log.Info("Error getting custom roles in spec")
			return nil, err
		}
		policyRoles = append(policyRoles, iampapv1.ConvertV2RoleModels(filterDefRoles)...)
	}
	return policyRoles, nil
}

func getResource(instance *ibmcloudv1alpha1.AccessPolicy, serviceIDAPI iamv1.ServiceIDRepository) (iampapv1.Resource, error) {
	/* Getting attributes for Resource */
	policyResource := iampapv1.Resource{}

	if instance.Spec.Target.ServiceClass != "" {
		policyResource.SetAttribute("serviceName", instance.Spec.Target.ServiceClass)
	}
	if instance.Spec.Target.ServiceID != "" {
		policyResource.SetAttribute("serviceInstance", instance.Spec.Target.ServiceID)
	}
	if instance.Spec.Target.ResourceName != "" {
		policyResource.SetAttribute("resourceType", instance.Spec.Target.ResourceName)
	}
	if instance.Spec.Target.ResourceID != "" {
		policyResource.SetAttribute("resource", instance.Spec.Target.ResourceID)
	}
	if instance.Spec.Target.ResourceGroup != "" {
		policyResource.SetResourceGroupID(instance.Spec.Target.ResourceGroup)
	}
	if instance.Spec.Target.Region != "" {
		policyResource.SetAttribute("region", instance.Spec.Target.Region)
	}
	if instance.Spec.Target.ResourceKey != "" && instance.Spec.Target.ResourceValue != "" {
		policyResource.SetAttribute(instance.Spec.Target.ResourceKey, instance.Spec.Target.ResourceValue)
	}
	//policyResource.SetServiceType("service")
	return policyResource, nil
}

func (r *ReconcileAccessPolicy) getAccessGroupInstance(instance *ibmcloudv1alpha1.AccessPolicy) (*ibmcloudv1alpha1.AccessGroup, error) {
	accessGroupNameSpace := instance.ObjectMeta.Namespace
	if instance.Spec.Subject.AccessGroupDef.AccessGroupNamespace != "" {
		accessGroupNameSpace = instance.Spec.Subject.AccessGroupDef.AccessGroupNamespace
	}
	accessGroupInstance := &ibmcloudv1alpha1.AccessGroup{}
	err := r.client.Get(context.Background(), types.NamespacedName{Name: instance.Spec.Subject.AccessGroupDef.AccessGroupName, Namespace: accessGroupNameSpace}, accessGroupInstance)
	if err != nil {
		log.Info("Error getting access group resource instance")
		return &ibmcloudv1alpha1.AccessGroup{}, err
	}
	return accessGroupInstance, nil
}

func (r *ReconcileAccessPolicy) getCustomRoleInstance(instance *ibmcloudv1alpha1.AccessPolicy, roleinstance *ibmcloudv1alpha1.CustomRolesDef) (*ibmcloudv1alpha1.CustomRole, error) {
	customRoleNameSpace := instance.ObjectMeta.Namespace
	if roleinstance.CustomRoleNamespace != "" {
		customRoleNameSpace = roleinstance.CustomRoleNamespace
	}
	customRoleInstance := &ibmcloudv1alpha1.CustomRole{}
	err := r.client.Get(context.Background(), types.NamespacedName{Name: roleinstance.CustomRoleName, Namespace: customRoleNameSpace}, customRoleInstance)
	if err != nil {
		log.Info("Error getting custom role resource instance")
		return &ibmcloudv1alpha1.CustomRole{}, err
	}
	return customRoleInstance, nil
}

func isWellFormed(instance ibmcloudv1alpha1.AccessPolicy) bool {
	accessgroupdef := &instance.Spec.Subject.AccessGroupDef
	if instance.Spec.Subject.UserEmail != "" && (instance.Spec.Subject.ServiceID != "" || instance.Spec.Subject.AccessGroupID != "" || accessgroupdef.AccessGroupName != "") {
		return false
	}

	if instance.Spec.Subject.ServiceID != "" && (instance.Spec.Subject.UserEmail != "" || instance.Spec.Subject.AccessGroupID != "" || accessgroupdef.AccessGroupName != "") {
		return false
	}

	if instance.Spec.Subject.AccessGroupID != "" && (instance.Spec.Subject.UserEmail != "" || instance.Spec.Subject.ServiceID != "" || accessgroupdef.AccessGroupName != "") {
		return false
	}

	if accessgroupdef.AccessGroupName != "" && (instance.Spec.Subject.UserEmail != "" || instance.Spec.Subject.ServiceID != "" || instance.Spec.Subject.AccessGroupID != "") {
		return false
	}

	return true
}
