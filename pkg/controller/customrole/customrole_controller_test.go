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
	logtest1 "log"
	"testing"
	"time"
	"path/filepath"
	"fmt"
	
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	context "github.ibm.com/seed/ibmcloud-iam-operator/pkg/context"
	resv1 "github.ibm.com/seed/ibmcloud-iam-operator/pkg/lib/resource/v1"

	"github.ibm.com/seed/ibmcloud-iam-operator/pkg/apis"
	test "github.ibm.com/seed/ibmcloud-iam-operator/test"
)

var (
	c         client.Client
	cfgg     *rest.Config
	namespace string
	scontext  context.Context
	t         *envtest.Environment
	stop      chan struct{}
	metricsHost = "0.0.0.0"
	metricsPort int32 = 8083
)

func TestCustomRole(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(20 * time.Second)
	SetDefaultEventuallyTimeout(180 * time.Second)

	RunSpecs(t, "CustomRole Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))
	useExistingCluster := true

	t = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "deploy", "crds")},
		ControlPlaneStartTimeout: 2 * time.Minute,
		KubeAPIServerFlags: append([]string(nil), "--admission-control=MutatingAdmissionWebhook"),
		UseExistingCluster: &useExistingCluster,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfgg, err = t.Start(); err != nil {
		logtest1.Fatal(err)
	}

	mgr, err := manager.New(cfgg, manager.Options{
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	recFn := newReconciler(mgr)
	Expect(add(mgr, recFn)).NotTo(HaveOccurred())

	stop = test.StartTestManager(mgr)

	namespace = test.SetupKubeOrDie(cfgg, "ibmcloud-iam-")
	scontext = context.New(c, reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})

})

var _ = AfterSuite(func() {
	clientset := test.GetClientsetOrDie(cfgg)
	test.DeleteNamespace(clientset.CoreV1().Namespaces(), namespace)
	close(stop)
	t.Stop()
})

var _ = Describe("customrole", func() {
   DescribeTable("should be ready",
	   func(CustomRolefile string) {
		   // now test creation of CustomRole
		   ap := test.LoadCustomRole("crtestdata/" + CustomRolefile)
		   apobj := test.PostInNs(scontext, &ap, true, 0)

		   // check CustomRole is online
		   Eventually(test.GetState(scontext, apobj)).Should(Equal(resv1.ResourceStateOnline))
	   },

	   Entry("string param", "coscustomrole.yaml"),
   )

   DescribeTable("should delete",
	   func(CustomRolefile string) {
		   ap := test.LoadCustomRole("crtestdata/" + CustomRolefile)
		   ap.Namespace = namespace

		   // delete CustomRole
		   test.DeleteObject(scontext, &ap, true)
		   Eventually(test.GetObject(scontext, &ap)).Should((BeNil()))
	   },

	   Entry("string param", "coscustomrole.yaml"),	
   )

   DescribeTable("should fail",
	   func(CustomRolefile string) {
		   ap := test.LoadCustomRole("crtestdata/" + CustomRolefile)
		   apobj := test.PostInNs(scontext, &ap, true, 0)

		   Eventually(test.GetState(scontext, apobj)).Should(Equal(resv1.ResourceStateFailed))
	   },

	   Entry("string param", "cosbadspec_1.yaml"),
	   Entry("string param", "cosbadspec_2.yaml"),
   )

   DescribeTable("should delete",
	   func(CustomRolefile string) {
		   ap := test.LoadCustomRole("crtestdata/" + CustomRolefile)
		   ap.Namespace = namespace

		   // delete CustomRole
		   test.DeleteObject(scontext, &ap, true)
		   Eventually(test.GetObject(scontext, &ap)).Should((BeNil()))
	   },

	   Entry("string param", "cosbadspec_1.yaml"),
	   Entry("string param", "cosbadspec_2.yaml"),
   )   
},
) 