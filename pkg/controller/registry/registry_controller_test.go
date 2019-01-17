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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	"github.com/kubic-project/registries-operator/pkg/test"
)


//TODO: reuse logic for naming from registry_cert_instaler.go to avoid hardcoding
var jobKey = types.NamespacedName{
	Name: "kubic-registry-installer-foo-com-5000",
	Namespace: metav1.NamespaceSystem,
}

const timeout = time.Second * 60 * 3

func testReconcileSetup(t *testing.T, mgr manager.Manager) {

	c := mgr.GetClient()

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

}


func testReconcileCleanup(t *testing.T, mgr manager.Manager){

	c := mgr.GetClient()

	registryName := types.NamespacedName{Name: "foo"}
	registry := &kubicv1beta1.Registry{}

	err := c.Get(context.TODO(), registryName, registry)

	//Get may fail if registry wasn't created, so ignore
	if err == nil {
		//Ensure the finalizers are removed to prevent delete to be
		//deadlocked by failed operator
		registry.ObjectMeta.Finalizers = []string{}
		err  = c.Update(context.TODO(), registry)
		if err != nil {
		  t.Logf("Error updating registry %v", err)
		} else {

			err = c.Delete(context.TODO(), registry)
			if err != nil {
			  t.Logf("Error deleting registry %v", err)
			}
		}
	}

	secretName := types.NamespacedName{
		Name: "foo-ca-crt",
		Namespace: metav1.NamespaceDefault,
	}
	secret := &corev1.Secret{}
	err = c.Get(context.TODO(),secretName, secret)
	//Get may fail if registry wasn't created, so ignore
	if err == nil {
		err = c.Delete(context.TODO(),secret)
		if err != nil {
		  t.Logf("Error deleting secret %v", err)
		}
	}

	crdName := "registries.kubic.opensuse.org"
	crdClient, _ := clientset.NewForConfig(mgr.GetConfig())

	err = crdClient.ApiextensionsV1beta1().
	                CustomResourceDefinitions().
		        Delete(crdName, &metav1.DeleteOptions{})

	if err != nil {
		t.Logf("Error deleting Registry CRD: %v", err)
	}

	job := &batchv1.Job{}
	err = c.Get(context.TODO(), jobKey, job)
	if err == nil {
		err = c.Delete(context.TODO(),job)
		if err != nil {
		  t.Logf("Error deleting job %v", err)
		}
	}

}

func TestReconcile(t *testing.T) {

	test.SkipUnlessIntegrationTesting(t)
	g := gomega.NewGomegaWithT(t)

	mgr := SetupTestManager(t)
	recFn, requests := SetupTestReconciler(newRegistryReconcilier(mgr))
	err := addRegController(mgr, recFn)

	if err != nil {
		t.Errorf("Error adding Controller %v", err)
	}

	defer close(StartTestManager(t, mgr))

	testReconcileSetup(t, mgr)

	defer testReconcileCleanup(t, mgr)

	expectedRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "foo", 
			Namespace: metav1.NamespaceSystem,
		},
	}
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	c := mgr.GetClient()
	job := &batchv1.Job{}
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job)}, timeout).Should(gomega.Succeed())

}
