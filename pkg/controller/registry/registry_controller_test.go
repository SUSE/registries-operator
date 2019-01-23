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
	"fmt"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"
	"testing"
	"time"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	"github.com/kubic-project/registries-operator/pkg/test"
	kubicutil "github.com/kubic-project/registries-operator/pkg/util"
)

var jobKey = types.NamespacedName{
	Name:      "kubic-registry-installer-foo-com-5000",
	Namespace: metav1.NamespaceSystem,
}

var checkJobKey = types.NamespacedName{
	Name:      "kubic-registry-checker-foo-com-5000",
	Namespace: metav1.NamespaceSystem,
}

const timeout = time.Second * 60 * 1

func testReconcileSetup(t *testing.T, mgr manager.Manager) {

	c := mgr.GetClient()

	// Create Secret
	secret, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Fatalf("Error creating secret %v", err)
	}

	err = c.Create(context.TODO(), secret)

	if apierrors.IsInvalid(err) {
		t.Fatalf("failed to create secret: %v", err)
	}

	// Create the Registry object
	instance, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Fatalf("Error Getting Registry %v", err)
	}
	err = c.Create(context.TODO(), instance)

	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

}

func cleanupJob(t *testing.T, c client.Client, jobName types.NamespacedName) {

	job := &batchv1.Job{}
	err := c.Get(context.TODO(), jobName, job)
	if err == nil {
		err = c.Delete(context.TODO(), job)
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Error deleting job %v", err)
		}
	}
}

func testReconcileCleanup(t *testing.T, mgr manager.Manager) {

	c := mgr.GetClient()

	registryName := types.NamespacedName{Name: "foo"}
	registry := &kubicv1beta1.Registry{}

	err := c.Get(context.TODO(), registryName, registry)

	//Get may fail if registry wasn't created, so ignore
	if err == nil {
		//Ensure the finalizers are removed to prevent delete to be
		//deadlocked by failed operator
		registry.ObjectMeta.Finalizers = []string{}
		err = c.Update(context.TODO(), registry)
		if err != nil {
			t.Logf("Error updating registry %v", err)
		}

		err = c.Delete(context.TODO(), registry)
		if err != nil {
			t.Logf("Error deleting registry %v", err)
		}
	}

	secretName := types.NamespacedName{
		Name:      "foo-ca-crt",
		Namespace: metav1.NamespaceSystem,
	}
	secret := &corev1.Secret{}
	err = c.Get(context.TODO(), secretName, secret)
	//Get may fail if registry wasn't created, so ignore
	if err == nil {
		err = c.Delete(context.TODO(), secret)
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

	cleanupJob(t, c, jobKey)

	cleanupJob(t, c, checkJobKey)
}

func createCertificateCheckingJob(t *testing.T, c client.Client, regName string) (*batchv1.Job, error) {

	job := &batchv1.Job{}

	registry := &kubicv1beta1.Registry{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: regName}, registry)
	if err != nil {
		return job, err
	}

	registryAddress := kubicutil.SafeID(registry.Spec.HostPort)
	secret, err := registry.GetCertificateSecret(c)
	if err != nil {
		return job, err
	}

	nodes, err := getAllNodes(c)
	if err != nil {
		return job, err
	}
	numNodes := len(nodes)

	dockerDstDir := filepath.Join(dockerCertsDir, registry.Spec.HostPort)
	podmanDstDir := filepath.Join(podmanCertsDir, registry.Spec.HostPort)

	cmdTemplate := "if [[ ! -f '%s/ca.crt' ]]; then echo 'cert not found at %s'; exit 1; fi "
	commands := []string{
		fmt.Sprintf(cmdTemplate, dockerDstDir, dockerDstDir),
		fmt.Sprintf(cmdTemplate, podmanDstDir, podmanDstDir),
	}

	job, err = getRunnerJobWithSecrets(&runnerWithSecrets{
		Commands:     []string{strings.Join(commands, " ; ")},
		JobName:      checkJobKey.Name,
		NumNodes:     int32(numNodes),
		JobNamespace: metav1.NamespaceSystem,
		Secrets:      map[string]*corev1.Secret{},
		Labels: map[string]string{
			jobInstallLabelHostPort: registryAddress,
			jobInstallLabelHash:     getSecretHash(secret),
		},
		HostPaths: []string{
			"/etc/docker",
			"/etc/containers",
		},
		AntiAffinity: map[string]string{
			jobInstallLabelHostPort: registryAddress,
		},
	})

	return job, err
}

func waitRegistryCreated(c client.Client, registryName string, timeout time.Duration) error {

	registry := &kubicv1beta1.Registry{}
	var err error
	wait.Poll(100*time.Millisecond, timeout, func() (bool, error) {
		err = c.Get(context.TODO(), types.NamespacedName{Name: registryName}, registry)

		if err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}

		return true, nil
	})
	return err
}

func waitRegistryUpdates(c client.Client, registryName string, expected int32, timeout time.Duration) (int32, error) {

	registry := &kubicv1beta1.Registry{}
	numNodes := int32(-1)
	var err error
	wait.Poll(100*time.Millisecond, timeout, func() (bool, error) {
		err = c.Get(context.TODO(), types.NamespacedName{Name: registryName}, registry)
		if err == nil {
			numNodes = int32(registry.Status.Certificate.NumNodes)
		} else if !apierrors.IsNotFound(err) {
			return false, err
		}
		err = nil
		return (numNodes == expected), nil
	})
	return numNodes, err
}

func waitJobCompletations(c client.Client, jobName string, expected int32, timeout time.Duration) (int32, error) {

	job := &batchv1.Job{}
	numNodes := int32(0)
	var err error
	wait.Poll(100*time.Millisecond, timeout, func() (bool, error) {
		err = c.Get(context.TODO(), types.NamespacedName{Name: jobName, Namespace: metav1.NamespaceSystem}, job)
		if err == nil {
			numNodes = job.Status.Succeeded
		} else if !apierrors.IsNotFound(err) {
			return false, err
		}
		err = nil
		return numNodes == expected, nil

	})
	return numNodes, err
}

func TestReconcile(t *testing.T) {

	test.SkipUnlessIntegrationTesting(t)
	g := gomega.NewGomegaWithT(t)

	mgr := SetupTestManager(t)
	c := mgr.GetClient()

	//recFn, requests := SetupTestReconciler(newRegistryReconcilier(mgr))
	//err := addRegController(mgr, recFn)
	//to get reconcilier's requests substitute the line below by the two lines above
	err := addRegController(mgr, newRegistryReconcilier(mgr))
	if err != nil {
		t.Fatalf("Error adding Controller %v", err)
	}

	defer close(StartTestManager(t, mgr))

	testReconcileSetup(t, mgr)

	//this wait is required because sometimes the registry is not
	//found inmediatly after creating.
	err = waitRegistryCreated(c, "foo", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	defer testReconcileCleanup(t, mgr)

	job, err := createCertificateCheckingJob(t, c, "foo")
	if err != nil {
		t.Fatalf("Error creating certificate checking job: %v", err)
	}

	numNodes := job.Spec.Completions

	updates, err := waitRegistryUpdates(c, "foo", *numNodes, timeout)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(updates).Should(gomega.Equal(*numNodes))

	err = c.Create(context.TODO(), job)
	if err != nil {
		t.Fatalf("Error starting certificate checking job: %v", err)
	}

	completed, err := waitJobCompletations(c, job.Name, *numNodes, timeout)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(completed).Should(gomega.Equal(*numNodes))

}
