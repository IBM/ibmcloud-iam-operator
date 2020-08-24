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

package test

import (
	"io/ioutil"
	"time"

	yaml2 "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1alpha1 "github.com/IBM/ibmcloud-iam-operator/pkg/apis/ibmcloud/v1alpha1"

	rcontext "github.com/IBM/ibmcloud-iam-operator/pkg/context"
)

// PostInNs the object
func PostInNs(context rcontext.Context, obj runtime.Object, async bool, delay time.Duration) runtime.Object {
	obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetNamespace(context.Namespace())
	return post(context, obj, async, delay)
}

// Post the object
func post(context rcontext.Context, obj runtime.Object, async bool, delay time.Duration) runtime.Object {
	done := make(chan bool)

	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		err := context.Client().Create(context, obj)
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
	return obj
}

// DeleteObject deletes an object
func DeleteObject(context rcontext.Context, obj runtime.Object, async bool) {
	done := make(chan bool)

	go func() {
		err := context.Client().Delete(context, obj)
		if err != nil {
			panic(err)
		}
		done <- true
	}()

	if !async {
		<-done
	}
}

// LoadAuthorizationPolicy loads the YAML spec into obj
func LoadAuthorizationPolicy(filename string) v1alpha1.AuthorizationPolicy {
	return *LoadObject(filename, &v1alpha1.AuthorizationPolicy{}).(*v1alpha1.AuthorizationPolicy)
}

// LoadAccessPolicy loads the YAML spec into obj
func LoadAccessPolicy(filename string) v1alpha1.AccessPolicy {
	return *LoadObject(filename, &v1alpha1.AccessPolicy{}).(*v1alpha1.AccessPolicy)
}

// LoadAccessPolicy loads the YAML spec into obj
func LoadAccessGroup(filename string) v1alpha1.AccessGroup {
	return *LoadObject(filename, &v1alpha1.AccessGroup{}).(*v1alpha1.AccessGroup)
}

// LoadAccessPolicy loads the YAML spec into obj
func LoadCustomRole(filename string) v1alpha1.CustomRole {
	return *LoadObject(filename, &v1alpha1.CustomRole{}).(*v1alpha1.CustomRole)
}

// LoadObject loads the YAML spec into obj
func LoadObject(filename string, obj runtime.Object) runtime.Object {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	yaml2.Unmarshal(bytes, obj)
	return obj
}
