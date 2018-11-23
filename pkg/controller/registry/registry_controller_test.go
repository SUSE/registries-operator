/*
 * Copyright 2018 SUSE LINUX GmbH, Nuernberg, Germany..
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package registry

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	"github.com/kubic-project/registries-operator/pkg/test"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceSystem}}

//TODO: reuse logic for naming from registry_cert_instaler.go to avoid hardcoding
var jobKey = types.NamespacedName{Name: "kubic-registry-installer-foo-com-5000", Namespace: metav1.NamespaceSystem}

const timeout = time.Second * 60 * 3

func TestReconcile(t *testing.T) {

	test.SkipUnlessIntegrationTesting(t)

	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newRegistryReconcilier(mgr))
	g.Expect(addRegController(mgr, recFn)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	// Create Secret
	secret, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}

	err = c.Create(context.TODO(), secret)

	if apierrors.IsInvalid(err) {
		t.Errorf("failed to create secret: %v", err)
	}

	// Create the Registry object and expect the Reconcile and Deployment to be created
	instance, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}
	err = c.Create(context.TODO(), instance)

	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Errorf("failed to create registry: %v", err)
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())

	defer func() {

		registryName := types.NamespacedName{Name: "foo"}
		created := &kubicv1beta1.Registry{}

		err0 := c.Get(context.TODO(), registryName, created)
		if err0 == nil {
			//Ensure the finalizers are removed to prevent delete to be
			//deadlocked by failed operator
			created.ObjectMeta.Finalizers = []string{}
			c.Update(context.TODO(), created)

			c.Delete(context.TODO(), instance)
		}

		c.Delete(context.TODO(), secret)

		crdName := "registries.kubic.opensuse.org"
		crdClient, err0 := clientset.NewForConfig(mgr.GetConfig())
		err0 = crdClient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crdName, &metav1.DeleteOptions{})

	}()

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	job := &batchv1.Job{}
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job) }, timeout).
		Should(gomega.Succeed())

	// Delete the Job and expect Reconcile to be called for Job deletion
	g.Expect(c.Delete(context.TODO(), job)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job) }, timeout).
		Should(gomega.Succeed())

	// Manually delete job since GC isn't enabled in the test control plane
	g.Expect(c.Delete(context.TODO(), job)).To(gomega.Succeed())

}
