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

var installJobName =  "kubic-registry-installer-foo-com-5000"

var checkJobName = "kubic-registry-checker-foo-com-5000"

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

	//this wait is required because sometimes the registry is not
	//found inmediatly after creating.
	err = waitRegistryCreated(c, "foo", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
}

func cleanupJob(t *testing.T, c client.Client, jobName string) {

	job := &batchv1.Job{}
	nsName := types.NamespacedName{
			Name: jobName,
			Namespace: metav1.NamespaceSystem,
	}
	err := c.Get(context.TODO(), nsName, job)
	if err == nil {
		err = c.Delete(context.TODO(), job)
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Error deleting job %v", err)
		}
	}
}

func deleteRegistry(c client.Client, registryName string) error {

	registry := &kubicv1beta1.Registry{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: registryName}, registry)

	if err == nil {
		//Ensure the finalizers are removed to prevent delete to be
		//deadlocked by failed operator
		registry.ObjectMeta.Finalizers = []string{}
		err = c.Update(context.TODO(), registry)
		if err == nil {
			err = c.Delete(context.TODO(), registry)
		}
	}

	return err

}

func deleteSecret(c client.Client, secretName string) error {

	nsName := types.NamespacedName{
		Name: secretName,
		Namespace: metav1.NamespaceSystem,
	}
	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), nsName, secret)
	if err == nil {
		err = c.Delete(context.TODO(), secret)
	}

	return err
}

func testReconcileCleanup(t *testing.T, mgr manager.Manager) {

	c := mgr.GetClient()

	err := deleteRegistry(c, "foo")
	if err != nil {
		t.Logf("Error deleting Registry %v", err)
	}



	crdName := "registries.kubic.opensuse.org"
	crdClient, _ := clientset.NewForConfig(mgr.GetConfig())

	err = crdClient.ApiextensionsV1beta1().
		CustomResourceDefinitions().
		Delete(crdName, &metav1.DeleteOptions{})

	if err != nil {
		t.Logf("Error deleting Registry CRD: %v", err)
	}

	cleanupJob(t, c, installJobName)

	cleanupJob(t, c, checkJobName+"-install")

	cleanupJob(t, c, checkJobName+"-remove")
}

func createCertificateCheckingJob(t *testing.T, c client.Client, regName string, checkExist bool) (*batchv1.Job, error) {

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
	var checkOper string
	var jobSuffix string
	if checkExist {
		checkOper = "! -f"
		jobSuffix = "-install"
	} else {
		checkOper = "-f"
		jobSuffix = "-remove"
	}

	cmdTemplate := "if [[ %s '%s/ca.crt' ]]; then echo 'Cert found at %s'; exit 1; fi "
	commands := []string{
		fmt.Sprintf(cmdTemplate, checkOper, dockerDstDir, dockerDstDir),
		fmt.Sprintf(cmdTemplate, checkOper, podmanDstDir, podmanDstDir),
	}


	job, err = getRunnerJobWithSecrets(&runnerWithSecrets{
		Commands:     []string{strings.Join(commands, " ; ")},
		JobName:      checkJobName+jobSuffix,
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
			return (numNodes == expected), nil
		}

		if !apierrors.IsNotFound(err) {
			return false, err
		}

		return false, nil
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
			return numNodes == expected, nil
		}

		if !apierrors.IsNotFound(err) {
			return false, err
		}

		return false, nil

	})
	return numNodes, err
}

func  checkReconcileJobs(t *testing.T, c client.Client, install bool, expected int32) (int32, error) {

	job, err := createCertificateCheckingJob(t, c, "foo", install)
	if err != nil {
		t.Fatalf("Error creating certificate checking job: %v", err)
	}

	err = c.Create(context.TODO(), job)
	if err != nil {
		t.Fatalf("Error starting certificate checking job: %v", err)
	}

	completed, err := waitJobCompletations(c, job.Name, expected, timeout)

	return completed, err
}

func removeCertificate(c client.Client) error {
	registry := &kubicv1beta1.Registry{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "foo"}, registry)

	if err == nil {
		//TODO: check best way to remove certificate from Registry
		registry.Spec.Certificate =  nil 
		err = c.Update(context.TODO(), registry)
		if err == nil {
			err = c.Delete(context.TODO(), registry)
		}
	}

	return err
}

func TestReconcile(t *testing.T) {

	test.SkipUnlessIntegrationTesting(t)
	g := gomega.NewGomegaWithT(t)

	mgr, stop := SetupTestManager(t)

	defer close(stop)

	testReconcileSetup(t, mgr)

	defer testReconcileCleanup(t, mgr)

	c := mgr.GetClient()

	nodes, err  := getAllNodes(c)
	if err != nil {
		t.Fatalf("Error getting list of nodes %v", err)
	}
	expected := int32(len(nodes))

	updates, err := waitRegistryUpdates(c, "foo", expected, timeout)
	if err != nil {
		t.Fatalf("Error waiting for certificates intalls: %v", err)
	}
	if updates != expected {
		t.Fatalf("Error waiting for registry installs. Expected: %d Actual: %d", expected, updates)
	}

    completed, err := checkReconcileJobs(t, c, true, expected)

	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(completed).Should(gomega.Equal(expected))

	err = deleteSecret(c, "foo-ca-crt")
	if err != nil {
		t.Fatalf("Error removing certificate: %v", err)
	}

	completed, err = checkReconcileJobs(t, c, false, expected)

	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(completed).Should(gomega.Equal(expected))
}

