// Copyright 2020 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	"github.com/IBM/ibmcloud-iam-operator/pkg/apis"
	operator "github.com/IBM/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"

	
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	retryInterval        = time.Second * 20
	timeout              = time.Second * 240
	cleanupRetryInterval = time.Second * 90
	cleanupTimeout       = time.Second * 200
)

func TestAccessPolicy(t *testing.T) {
	accesspolicyList := &operator.AccessPolicy{}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, accesspolicyList)
	if err != nil {
		t.Fatalf("failed to add custom resource scheme to framework: %v", err)
	}
	// run subtests
	t.Run("accesspolicy-group", func(t *testing.T) {
		t.Run("Cluster", accesspolicyCluster)
		//t.Run("Cluster2", accesspolicyCluster)
	})
}

func accesspolicyTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return fmt.Errorf("could not get namespace: %v", err)
	}
	// create accesspolicy custom resource
	exampleAccesspolicy := &operator.AccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cosuserpolicy",
			Namespace: namespace,
		},
		Spec: operator.AccessPolicySpec{
			Subject: operator.Subject{
				UserEmail: "avarghese@us.ibm.com",
			},
			Roles: operator.Roles {
				DefinedRoles: []string{"Viewer","Administrator"},
			},
			Target: operator.Target{
				ResourceGroup: "Default",
				ServiceClass: "cloud-object-storage",
				ServiceID: "1cdd19ff-c033-4767-b6b7-4fe2fc58c6a1",
				ResourceName: "bucket",
				ResourceID: "cos-standard-ansu",
			},
		},
	}
	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), exampleAccesspolicy, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		return err
	}
	// wait for example-Accesspolicy to run
 	err = waitForAccessPolicy(t, f, namespace, "cosuserpolicy", retryInterval, timeout)
	if err != nil {
	  	return err
    }

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "cosuserpolicy", Namespace: namespace}, exampleAccesspolicy)
	if err != nil {
		return err
	}
	
	exampleAccesspolicy.Spec.Roles.DefinedRoles = []string{"Viewer"}
	err = f.Client.Update(goctx.TODO(), exampleAccesspolicy)
	if err != nil {
		return err
	} 

	// wait for cosuserpolicy to update role
	err = waitForAccessPolicy(t, f, namespace, "cosuserpolicy", retryInterval, timeout)
	if err != nil {
	  	return err
    }
	
	return nil
}

func accesspolicyCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
 	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	} 
	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	// get global framework variables
	f := framework.Global

	// wait for ibmcloud-iam-operator to be ready
	err = e2eutil.WaitForOperatorDeployment(t, f.KubeClient, namespace, "ibmcloud-iam-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	if err = accesspolicyTest(t, f, ctx); err != nil {
		t.Fatal(err)
	}
}

func waitForAccessPolicy(t *testing.T, f *framework.Framework, namespace, name string, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		exampleAccesspolicy := &operator.AccessPolicy{}
		err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, exampleAccesspolicy)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of Access Policy: %s in Namespace: %s \n", name, namespace)
				return false, nil
			}
			return false, err
		}

		if exampleAccesspolicy.Status.State == "Online" {
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("AccessPolicy available")
	return nil
}