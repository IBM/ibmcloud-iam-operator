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

package util

import (
	"context"
	"os"
	"strings"

	icv1 "github.ibm.com/seed/ibmcloud-iam-operator/pkg/lib/ibmcloud/v1"
	
	bx "github.com/IBM-Cloud/bluemix-go"
	bxendpoints "github.com/IBM-Cloud/bluemix-go/endpoints"
	"github.com/IBM-Cloud/bluemix-go/api/account/accountv2"
	"github.com/IBM-Cloud/bluemix-go/api/mccp/mccpv2"
	"github.com/IBM-Cloud/bluemix-go/session"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logc = logf.Log.WithName("utils")
var controllerNamespace string

const seedInstall = "ibmcloud-iam-operator"
const seedSecret = "secret-ibmcloud-iam-operator"
const seedDefaults = "config-ibmcloud-iam-operator"

func GetIAMAccountInfo(r client.Client, namespace string) (*session.Session, *accountv2.Account, error) {
	// Get Bx Config
	bxConfig, err := getBxConfig(r, namespace)
	if err != nil {
		logc.Info("Error getting Bluemix config")
		return nil, nil, err
	}
	
	ibmCloudContext, err := getIBMCloudDefaultContext(r, namespace)
	if err != nil {
		logc.Info("Error getting IBM Cloud context")
		return nil, nil, err
	}

	sess, err := session.New(&bxConfig)
	if err != nil {
		logc.Info("Error creating new session")
		return nil, nil, err
	}

	client, err := mccpv2.New(sess)
	if err != nil {
		logc.Info("Error creating new client")
		return nil, nil, err
	}

	orgAPI := client.Organizations()
	myorg, err := orgAPI.FindByName(ibmCloudContext.Org, sess.Config.Region)
	if err != nil {
		logc.Info("Error getting my org")
		return nil, nil, err
	}

	accClient, err := accountv2.New(sess)
	if err != nil {
		logc.Info("Error getting account Client")
		return nil, nil, err
	}

	accountAPI := accClient.Accounts()
	myAccount, err := accountAPI.FindByOrg(myorg.GUID, sess.Config.Region)
	if err != nil {
		logc.Info("Error getting my account")
		return nil, nil, err
	}

	return sess, myAccount, nil
}

func getBxConfig(r client.Client, secretNS string) (bx.Config, error) {
	config := bx.Config{
		EndpointLocator: bxendpoints.NewEndpointLocator("us-south"), // TODO: hard wired to us-south!!
		//Debug: true,
	}

	secretName := seedSecret
	secretNameSpace := secretNS

	secret := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNameSpace}, secret)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			namespace := getDefaultNamespace(r)
			if namespace != "default" {
				secretName = secretNameSpace + "-" + secretName
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			if err != nil {
				logc.Info("Unable to get secret in namespace", namespace, err)
				return config, err
			}
		} else {
			logc.Info("Unable to get secret", "Error", err)
			return config, err
		}
	}

	APIKey := string(secret.Data["api-key"])

	regionb, ok := secret.Data["region"]
	if !ok {
		logc.Info("set default region to us-south")
		regionb = []byte("us-south")
	}
	region := string(regionb)

	config.Region = region
	config.BluemixAPIKey = APIKey

	return config, nil
}

func getDefaultNamespace(r client.Client) string {
	if controllerNamespace == "" {
		controllerNamespace = os.Getenv("CONTROLLER_NAMESPACE")
	}
	cm := &v1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{Namespace: controllerNamespace, Name: seedInstall}, cm)

	if err != nil {
		return "default"
	}

	// There exists an ico-management configmap in the controller namespace
	return cm.Data["namespace"]
}

func getIBMCloudDefaultContext(r client.Client, configmapNS string) (icv1.ResourceContext, error) {
	cm := &v1.ConfigMap{}
	cmName := seedDefaults
	cmNameSpace := configmapNS

	err := r.Get(context.Background(), types.NamespacedName{Namespace: cmNameSpace, Name: cmName}, cm)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			namespace := getDefaultNamespace(r)
			if namespace != "default" {
				cmName = cmNameSpace + "-" + cmName
			}
			err = r.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: namespace}, cm)
			if err != nil {
				logc.Info("Failed to find ConfigMap in namespace (in Service)", namespace, err)
				return icv1.ResourceContext{}, err
			}
		} else {
			logc.Info("Failed to find ConfigMap in namespace (in Service)", cmNameSpace, err)
			return icv1.ResourceContext{}, err
		}

	}
	ibmCloudContext := getIBMCloudContext(cm)
	return ibmCloudContext, nil
}

func getIBMCloudContext(cm *v1.ConfigMap) icv1.ResourceContext {
	resourceGroup := cm.Data["resourceGroup"]
	if resourceGroup == "" {
		resourceGroup = "default"
	}
	newContext := icv1.ResourceContext{
		Org:           cm.Data["org"],
		Space:         cm.Data["space"],
		Region:        cm.Data["region"],
		ResourceGroup: resourceGroup,
	}

	return newContext
}