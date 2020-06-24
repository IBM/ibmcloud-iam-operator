[![Build Status](https://travis.ibm.com/seed/ibmcloud-iam-operator.svg?token=a5HZPMvujFJXhTzxH5Gq&branch=master)](https://travis.ibm.com/seed/ibmcloud-iam-operator)

# IBM Cloud's IAM Access Policy Management with Kubernetes Operators

# Index:
1. [High-level problem statement](#high-level-problem-statement)
2. [Requirements](#requirements)
3. [Installing the IBM Cloud IAM operator](#installing-the-ibm-cloud-iam-operator)
4. [Removing the IBM Cloud IAM operator](#removing-the-ibm-cloud-iam-operator)
5. [Using the IBM Cloud IAM Operator](#using-the-ibm-cloud-iam-operator)
6. [For security reasons: Using a Management Namespace](#for-security-reasons-using-a-management-namespace)
7. [Managing Access Groups, Custom Roles or Access Policies](#managing-access-groups-custom-roles-or-access-policies)
8. [Access Policy Reconciliation rules](#access-policy-reconciliation-rules)
9. [Tagging IAM Operator owned resources](#tagging-iam-operator-owned-resources)
10. [Examples](#examples)
11. [Testing](#testing)
12. [Impact Statement](#impact-statement)

## High-level problem statement

On IBM Public Cloud, Identity and Access Management (IAM) is used to give access permissions so that entities can interact with resources. These access policies are set via the CLI or on the Console and typically require many gestures (retrieving API keys/tokens, creating user groups, adding users to group, obtaining subject and target IDs, copy and pasting, etc...). 


IBM Cloud IAM Kubernetes Operator provides a **user-friendly Operator for IKS and OpenShift to automate scenarios for managing access to IBM Cloud  Resources** via Kubernetes CRD-Based APIs:
1. For access groups,
2. For custom  roles, and
3. For access policies

This will give users few advantages:
- The operator **makes it easier to specify access groups, custom roles, and access policies** at a high level in a Kubernetes  YAML  declaratively, 
- The operator **automates interactions with IAM APIs** without requiring the user to know specifics,
- The operator **enforces access groups, custom roles, and access policies** via the operator's desired-state management. For example, if a user is moved out of a group accidentally, the operator would move them back and remediate the issue,
- The operator **integrates IBM Cloud  IAM  tightly with Kubernetes'  built-in  support  for RBAC**.

## Requirements

The operator can be installed on any Kubernetes cluster with version >= 1.11. 

You need an [IBM Cloud account](https://cloud.ibm.com/registration) and the 
[IBM Cloud CLI](https://cloud.ibm.com/docs/cli?topic=cloud-cli-getting-started). 

You need also to have the [kubectl CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl/) already configured to access your cluster. 

Before installing the operator, you need to login to your IBM cloud account with the IBM Cloud CLI:

```bash
ibmcloud login
```

and set a default target environment for your resources with the command:

```bash
ibmcloud target --cf -g default
```

This will use the IBM Cloud ResourceGroup `default`. To specify a different ResourceGroup, use the following command:
```bash
ibmcloud target -g <resource-group>
```

Notice that the `org`must be included.

## Installing the IBM Cloud IAM Operator

To install the latest release of the IAM operator, run the following script:

```
curl -sL https://raw.github.ibm.com/seed/ibmcloud-iam-operator/master/hack/install-operator.sh | bash 
```

The script above first creates an IBM Cloud API Key and stores it in a Kubernetes secret that can be
accessed by the operator, then it sets defaults such as the default resource group and region 
used to provision IBM Cloud Services; finally, it deploys the operator in your cluster.


## Removing the IBM Cloud IAM Operator

To remove the operator, run the following script:

```
curl -sL https://raw.github.ibm.com/seed/ibmcloud-iam-operator/master/hack/uninstall-operator.sh | bash 
```

## Using the IBM Cloud IAM Operator

### 1. Access Group Yaml Elements [NEW!] 

The `Access Group` yaml includes the following elements:

Spec Fields | Is required | Format/Type | Comments
---------| ------------|-------------|-----------------
Name 	| Yes | string 	 | Specify the name of the new access group to be created
Description | Yes | string   | Specify a description for this new access group
UserEmails | No |   []string | Specify the email IDs of the IAM Users who will be members of this new group
ServiceIDs  | No |  []string | Specify the IAM IDs of Services that will be members of this new group e.g "ServiceId-3b9f026a-eb6e-495f-b104-95232d0c4a59"

### 2. Custom Role Yaml Elements [NEW!] 

The `Custom Role` yaml includes the following elements:

Spec Fields | Is required | Format/Type | Comments
---------| ------------|-------------|-----------------
RoleName | Yes | string | Specify the name of the new custom role to be created e.g "COSAdmin"
ServiceClass | Yes | string | Specify the name of the IBM Cloud service that is linked to this new role e.g "cloud-object-storage"
DisplayName | Yes | string | Specify the display name of the new custom role to be created e.g "COS Admin"
Description | Yes | string | Specify a description for this new custom role
Actions  | Yes | []string | Specify a list of actions that this role can perform (IAM actions as well as actions available for the service specified in ServiceClass)

### 3. Access Policy Yaml Elements

The `Access Policy` yaml includes the following elements:

Spec Fields | Is required | Format/Type | Comments
---------| ------------|-------------|-----------------
 Subject | Yes | Subject  | The type to specify the Subject of an access policy
 Roles   | Yes | Roles    | The type to specify a list of Roles of an access policy
 Target  | Yes | Target   | The type to specify the Target of an access policy
 
 
Subject Fields | Is required | Format/Type | Comments
---------| ------------|-------------|-----------------
UserEmail | No | string | Specify an email ID in this field if access policy is for one user only
ServiceID | No | string | Specify a Service ID (ID from IAM) in this field if access policy is for one service only
AccessGroupID | No | string  | The type to specify an access group ID for an already existing access group
AccessGroupDef | No | AccessGroupDef | The type to specify details for an operator managed access group 
   
*You must specify only **one of the above** four fields as Subject per access policy yaml spec.

AccessGroupDef Fields | Is required | Format/Type | Comments
------------| ------------|-------------|-----------------
AccessGroupName | Yes | string | Specify the name of an access group custom resource running in the cluster 
AccessGroupNamespace | Yes | string | Specify the namespace of the access group custom resource running in the cluster 

Roles Fields | Is required | Format/Type | Comments
---------| ------------|-------------|-----------------
DefinedRoles | No | []string | Specify a list of existing defined roles (platform and/or service) using their display names
CustomRolesDName | No |[]string | Specify a list of existing custom roles using their display names
CustomRolesDef | No |[]CustomRolesDef | The type to specify details for an operator managed custom role
 
CustomRolesDef Fields | Is required | Format/Type | Comments
------------| ------------|-------------|-----------------
CustomRoleName | Yes | string | Specify the name of a custom role custom resource running in the cluster 
CustomRoleNamespace | Yes | string | Specify the namespace of the custom role custom resource running in the cluster 

Target Fields | Is required | Format/Type | Comments
-------------| ------------|-------------|-----------------
ResourceGroup | No | string | Specify a Resource Group like, "Default"
ServiceClass | No | string | Specify a name of IBM Cloud shared service like, "cloud-object-storage"
ServiceID | No | string | Specify the ServiceID of an instance of the service in ServiceClass like "ServiceID-xyz"
ResourceName | No | string | Specify the name of a resource in a shared service like, "bucket"
ResourceID | No | string | Specify the ResourceID of the resource in ResourceName like, "my-cos-bucket" (Not an ID)
ResourceKey | No | string | Specify the attribute of a resource as a key in a shared service like, "namespace"
ResourceValue | No | string | Specify the value of the ResourceKey like, "dev" (Not an ID)

Each `paramater` is treated as a `RawExtension` by the Operator and parsed into JSON.

The IBM Cloud IAM Operator needs an account context, which indicates the `api-key` and the details of the IBM Public Cloud
account to be used for service instantiation. The `api-key` is contained in a Secret called `secret-ibmcloud-iam-operator` that is created when the IBM Cloud IAM Operator is installed. Details of the account (such as organization, space, resource group) are held in a ConfigMap called `config-ibmcloud-iam-operator`. To find the secret and configmap the IBM Cloud Operator first looks at the namespace of the resource being created, and if not found, in a management namespace (see below for more details on management namespaces). If there is no management namespace, then the operator looks for the secret and configmap in the `default` namespace. 


## For security reasons: Using a Management Namespace

Different Kubernetes namespaces can contain different secrets `secret-ibmcloud-iam-operator` and configmap `config-ibmcloud-iam-operator`, corresponding to different IBM Public Cloud accounts. So each namespace can be set up for a different account. 

In some scenarios, however, there is a need for hiding the `api-keys` from users. In this case, a management namespace can be set up that contains all the secrets and configmaps corresponding to each namespace, with a naming convention. 

To configure a management namespace named `safe`, there must be a configmap named `ibmcloud-iam-operator` created in the same namespace as the IBM Cloud Operator itself. This configmap indicates the name of the management namespace, in a field `namespace`. To create such a config map, execute the following:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: ibmcloud-iam-operator
  namespace: <namespace where IBM Cloud IAM Operator has been installed>
  labels:
    app.kubernetes.io/name: ibmcloud-iam-operator
data:
  namespace: safe
EOF
```

This configmap indicates to the operator where to find the management namespace, in this case `safe`.
Next the `safe` namespace needs to contain secrets and configmaps corresponding to each namespace that will contain access policies. The naming convention is as follows:

```
<namespace>-secret-ibmcloud-iam-operator
<namespace>-config-ibmcloud-iam-operator
```

These can be created similary to what is done in `hack/configure-operator.sh`.

If we create an access policy resource in a namespace `XYZ`, the IBM Cloud IAM Operator first looks in the `XYZ` namespace to find `secret-ibmcloud-iam-operator` and `config-ibmcloud-iam-operator`, for account context. If they are missing in `XYZ`, it looks for the `ibmcloud-iam-operator` configmap in the namespace where the operator is installed, to see if there is a management namespace. If there is, it looks in the management namespace for the secret and configmap with the naming convention:
`XYZ-secret-ibmcloud-iam-operator` and `XYZ-config-ibmcloud-iam-operator`. If there is no management namespace, the operator looks in the `default` namespace for the secret and configmap (`secret-ibmcloud-iam-operator` and `config-ibmcloud-iam-operator`).

## Managing Access Groups, Custom Roles or Access Policies

### Creating an Access Group, Custom Role or Access Policy

You can create an access policy with name `cosuserpolicy` for user `avarghese@us.ibm.com` to access a Cloud Object Storage instance's bucket as an `Administrator`  using the following custom resource written in an yaml file `cosuserpolicy.yaml`:


```apiVersion: ibmcloud.ibm.com/v1alpha1
kind: AccessPolicy
metadata:
  name: cosuserpolicy
spec:
  subject:
    userEmail: avarghese@us.ibm.com
  roles:
    definedRoles:
      - Administrator
  target:
    resourceGroup: Default
    serviceClass: cloud-object-storage
    serviceID: 1cdd19ff-c033-4767-b6b7-4fe2fc58c6a1
    resourceName: bucket
    resourceID: cos-standard-ansu
```
and then run the command:
```kubectl create -f cosuserpolicy.yaml```

To find the status of your access policy, you can run the command:

```kubectl get accesspolicies.ibmcloud 
NAME                 STATUS   AGE
cosuserpolicy        Online   25s
```

Here's another example to create all three custom resources: You can create an access policy with name `demonewgrouppolicy` for access group resource `demonewgroup` in namespace `default` to access an Event Stream instance's topic `topic-ansu` as a custom role `ES Admin` that is running in the cluster as a custom role resource `democustomrole` in namespace `default` using the following yaml file [`accesspolicy_example_EventStreams_demo.yaml`](deploy/examples/accesspolicy_example_EventStreams_demo.yaml) :

```apiVersion: ibmcloud.ibm.com/v1alpha1
kind: AccessGroup
metadata:
  name: demonewgroup
spec:
  name: demonewgroup
  description: A new access group to test access group controller
  userEmails:
      - avarghese@us.ibm.com
      - mvaziri@us.ibm.com
  serviceIDs:
    - ServiceId-3b9f026a-eb6e-495f-b104-95232d0c4a59
    - ServiceId-fa27c539-a6cf-41d2-8cb0-2916da5f8e8a   

---
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: CustomRole
metadata:
  name: democustomrole
spec:
  roleName: ESAdmin
  serviceClass: messagehub
  displayName: ES Admin
  description: Event Streams admin is an admin that only has a subset of the privileges of an Admin role
  actions: 
    - iam.policy.create
    - iam.policy.update
    - messagehub.group.read

---
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: AccessPolicy
metadata:
  name: demonewgrouppolicy
spec:
  subject:
    accessGroupDef: 
      accessGroupName: demonewgroup
      accessGroupNamespace: default
  roles: 
    definedRoles:
      - Viewer
    customRolesDef:
      - customRoleName: democustomrole
        customRoleNamespace: default
  target: 
    resourceGroup: Default    
    serviceClass: messagehub
    serviceID: 9f9d6641-d5ad-4fb2-8d49-c1e97bcfb631
    resourceName: topic
    resourceID: topic-ansu    
```

and then run the command:
```kubectl create -f accesspolicy_example_EventStreams_demo.yaml```

To find the status of your access policy, you can run the command:

```kubectl get accessgroups.ibmcloud 
NAME                 STATUS   AGE
demonewgroup        Online   25s

kubectl get customroles.ibmcloud 
NAME                 STATUS   AGE
democustomrole        Online   25s

kubectl get accesspolicies.ibmcloud 
NAME                 STATUS   AGE
demonewgrouppolicy        Online   25s
```

### Updating an Access Group, Custom Role or Access Policy

You can update an existing access policy custom resource, say, if you'd like to change an existing subject or role or resource target in an existing IAM access policy. You can either edit the yaml specification, and then run the command:
```kubectl apply -f cosuserpolicy.yaml```

Or you can run the kubectl edit command directly to update the resource:
```kubectl edit accesspolicies.ibmcloud cosuserpolicy```

And similarly, for access groups and custom roles.

### Deleting an Access Group, Custom Role or Access Policy

Deleting an access policy custom resource, deletes the access policy instance in IBM Cloud's IAM. 

To delete an access policy with name `cosuserpolicy`, run:

```kubectl delete accesspolicies.ibmcloud cosuserpolicy```

The operator uses finalizers to remove the custom resource only after the access policy is deleted from IAM. 

And similarly, for access groups and custom roles.

## Access Policy Reconciliation rules

Deleting a IBM Cloud entity that is part of an access policy managed by an IAM Operator behaves as below:
1.	Deleting a IBM Cloud User (access policy subject) causes the access policy status to change to FAILED during the next reconciliation cycle. The IAM Operator does not control the creation of deleted IAM user entities.
2.	Deleting a IBM Cloud Service ID (access policy subject) - same as #1 above.
3.	Deleting a IBM Cloud existing Access Group ID (access policy subject) that was not created by the operator - same as #1 above
4.	Deleting a IBM Cloud Access Group (access policy subject) also managed by the operator leads to IAM operator recreating the access group during the next reconciliation cycle. The group's members and policy will also be fixed.
5. 	Deleting a IBM Cloud Custom Role also managed by the operator leads to IAM operator recreating the custom role during the next reconciliation cycle. 
6. 	Deleting the IBM Cloud Access Policy itself managed by this operator leads to recreating the access policy during the next reconciliation cycle. 

## Tagging IAM Operator owned resources 

In order to differentiate IBM Cloud IAM Operator managed resources from user controlled resources created via IBM Clous console UIs, REST APIs, IBM Cloud CLIs etc, the operator adds a prefix string "OPERATOR OWNED: " to the Description fields in both Custom Resource and Access Groups. Access Policies do not have a Description field provided by IAM today. Also, as of this operator's writing, IAM does not support a tag feature for its resources. Hence, why we have used free text field Description for defining the source of truth for operator managed resources.

## Examples

You can find [additional samples here.](deploy/examples)
#### Examples of creating access policies for various IBM Cloud Resources
1. [Mongodb,](deploy/examples/accesspolicy_example_MongoDB.yaml)
2. [Redis,](deploy/examples/accesspolicy_example_Redis.yaml)
3. [Postgres,](deploy/examples/accesspolicy_example_Postgres.yaml)
4. [Elastic Search,](deploy/examples/accesspolicy_example_ElasticSearch.yaml)
5. [Cloudant,](deploy/examples/accesspolicy_example_Cloudant.yaml)
6. [KeyProtect,](deploy/examples/accesspolicy_example_KeyProtect.yaml)
7. [Cloud Object Storage,](deploy/examples/accesspolicy_example_COS.yaml)
8. [COS buckets,](deploy/examples/accesspolicy_example_COS_bucket.yaml)
9. [Event Streams,](deploy/examples/accesspolicy_example_EventStreams.yaml)
10. [Event-stream topics](deploy/examples/accesspolicy_example_EventStreams_topic.yaml)

## Testing
### How to run Unit Tests

```make unittest```

### How to run End-to-end Tests

```make e2etest```

## Impact Statement


Operators are a cornerstone of OpenShift v4, and a key element of the OpenShift developer catalog and DevX. By advancing Operators technology, we expect to further reduce friction for developers and increase IBM Cloud Adoption, therefore having a direct impact on the IBM / Red Hat synergy.
