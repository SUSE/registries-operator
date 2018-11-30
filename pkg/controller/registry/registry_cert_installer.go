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
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	kubicutil "github.com/kubic-project/registries-operator/pkg/util"
)

const (
	// a prefix for all the jobs created for installing certificates
	jobInstallNamePrefix = "kubic-registry-installer"

	// some labels in jobs that install certificates: the node where this job should run
	jobInstallLabelHostPort = "kubic-registry-installer-host-port"

	// some labels in jobs that install certificates: the hash of the CA.crt this Job is trying to install
	jobInstallLabelHash = "kubic-registry-installer-hash"
)

// reconcileCertPresent reconciles the Certificate for this Registry
func (r *ReconcileRegistry) reconcileCertPresent(registry *kubicv1beta1.Registry,
	curNodes map[string]*corev1.Node,
	specSecret *corev1.Secret) (reconcile.Result, error) {

	// 1. Check if the certificate in this Registry has changed or has never been installed
	specSecretHash := getSecretHash(specSecret)
	mustInstall := false

	// 1. Check if the certificate in this Registry has changed
	if len(registry.Status.Certificate.CurrentHash) > 0 && specSecretHash != registry.Status.Certificate.CurrentHash {
		glog.V(3).Infof("[kubic] sCA.crt for '%s' as changed: (re)deploying", registry)
		//  flush the current status, and mark that we must re-deploy the certificate
		mustInstall = true
		registry.Status.Certificate.CurrentHash = ""
		registry.Status.Certificate.NumNodes = 0
	}

	// 2. Check if maybe we have not installed this Registry in some new nodes
	if registry.Status.Certificate.NumNodes != len(curNodes) {
		if !mustInstall {
			glog.V(5).Infof("[kubic] some nodes do not have current CA.crt for '%s' yet: (re)deploying", registry)
			mustInstall = true
		}
	}

	// 3. Process all the Jobs that were launched from this controller
	jobs, err := getAllJobsWithLabels(r, map[string]string{
		jobInstallLabelHostPort: kubicutil.SafeID(registry.Spec.HostPort),
		jobInstallLabelHash:     specSecretHash,
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	glog.V(3).Infof("[kubic] %d installation Jobs found for %s", len(jobs), registry.Spec.HostPort)
	if len(jobs) > 0 {
		// Process the certificate installation jobs
		job := jobs[0] // there should be not more than one job

		glog.V(3).Infof("[kubic] Job '%s': Active=%d, Failed=%d, Succeeded=%d",
			job.GetName(), job.Status.Active, job.Status.Failed, job.Status.Succeeded)

		if job.Status.Active > 0 {
			// let the Job finish. Once it is done, it will be processed on the
			// following "... else if jobState.Terminated ..."
			glog.V(5).Infof("[kubic] Job '%s' is still active... will let it finish", job.Name)

			// there is no need to trigger an installation
			mustInstall = false
		} else if job.Status.Failed > 0 {
			glog.V(3).Infof("[kubic] Job '%s' has failed to install '%s's CA.crt", job.Name, registry)

			glog.V(3).Infof("[kubic] removing Job '%s': we will trigger another one...", job.Name)
			if err = r.Delete(context.TODO(), &job); err != nil {
				return reconcile.Result{}, err
			}

			r.EventRecorder.Event(registry, corev1.EventTypeNormal,
				"Failed", fmt.Sprintf("Certificate installation of '%s' failed... retrying",
					specSecretHash))

			mustInstall = true // and mark the node for another try...

		} else if job.Status.Succeeded > 0 {
			glog.V(3).Infof("[kubic] Job '%s' has finished", job.Name)

			if registry.Status.Certificate.CurrentHash != specSecretHash && registry.Status.Certificate.NumNodes != int(job.Status.Succeeded) {
				r.EventRecorder.Event(registry, corev1.EventTypeNormal,
					"Installed", fmt.Sprintf("Certificate '%s' successfully installed", specSecretHash))

				registry.Status.Certificate.CurrentHash = specSecretHash
				registry.Status.Certificate.NumNodes = int(job.Status.Succeeded)
			}

			glog.V(3).Infof("[kubic] Job '%s' has completed its mission: removing it!", job.Name)
			if err = r.Delete(context.TODO(), &job); err != nil {
				if apierrors.IsGone(err) {
					return reconcile.Result{}, nil
				}
				return reconcile.Result{}, err
			}

			// The Job has copied the current certificate
			// so there is no need to mark the node as "dirty"
			mustInstall = false
		} else {
			glog.V(5).Infof("[kubic] Job '%s' has a unknown state", job.Name)
			mustInstall = false
		}
	}

	// lunch jobs that install all the `ca.crt`s in all the nodes
	if mustInstall {
		r.EventRecorder.Event(registry, corev1.EventTypeNormal,
			"Starting", fmt.Sprintf("Starting certificate installation for '%s'",
				specSecretHash))
		err := r.installCertForRegistry(registry, specSecret, len(curNodes))
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				glog.V(3).Infof("[kubic] the Job already exists")
				return reconcile.Result{}, err
			}

			glog.V(1).Infof("[kubic] ERROR: when trying to create the Job: %s", err)
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil

}

// installCertForRegistry creates a `Job` for installing certificates at node `Node`
func (r *ReconcileRegistry) installCertForRegistry(registry *kubicv1beta1.Registry, secret *corev1.Secret, numNodes int) error {
	var err error

	registryAddress := kubicutil.SafeID(registry.Spec.HostPort)
	jobName := kubicutil.SafeID(jobInstallNamePrefix) + "-" + registryAddress

	// note: docker cannot mount directories with colons (like "registry.suse.de:5000")
	//       so we will use the path "/certs/this-registry/ca.crt"
	registryDir := "this-registry"
	srcDir := filepath.Join(jobSecretsDir, registryDir)
	dockerDstDir := filepath.Join(dockerCertsDir, registry.Spec.HostPort)
	podmanDstDir := filepath.Join(podmanCertsDir, registry.Spec.HostPort)

	// commands executed for installing the certificate for Docker and Podman
	commands := []string{
		fmt.Sprintf("echo Removing %s", dockerDstDir),
		fmt.Sprintf("[ -d '%s' ] && rm -rf '%s'", dockerDstDir, dockerDstDir),
		fmt.Sprintf("mkdir -p '%s'", dockerDstDir),
		fmt.Sprintf("echo Copying %s/ca.crt to %s/ ...", srcDir, dockerDstDir),
		fmt.Sprintf("cp '%s/ca.crt' '%s/'", srcDir, dockerDstDir),

		fmt.Sprintf("echo Removing %s", podmanDstDir),
		fmt.Sprintf("[ -d '%s' ] && rm -rf '%s'", podmanDstDir, podmanDstDir),
		fmt.Sprintf("mkdir -p '%s'", podmanDstDir),
		fmt.Sprintf("echo Copying %s/ca.crt to %s/ ...", srcDir, podmanDstDir),
		fmt.Sprintf("cp '%s/ca.crt' '%s/'", srcDir, podmanDstDir),

		fmt.Sprintf("echo Done"),
	}

	glog.V(3).Infof("[kubic] generating Job '%s'", jobName)
	job, err := getRunnerJobWithSecrets(&runnerWithSecrets{
		Commands:     []string{strings.Join(commands, " ; ")},
		JobName:      jobName,
		NumNodes:     int32(numNodes),
		JobNamespace: metav1.NamespaceSystem,
		Secrets: map[string]*corev1.Secret{
			registryDir: secret,
		},
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
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(registry, job, r.scheme); err != nil {
		glog.V(1).Infof("[kubic] ERROR: when setting Controller reference: %s", err)
		return err
	}

	// just for debugging: pretty-print the YAML
	if glog.V(8) {
		marshalled, err := kubeadmutil.MarshalToYamlForCodecs(job, batchv1.SchemeGroupVersion, scheme.Codecs)
		if err != nil {
			glog.V(1).Infof("[kubic] ERROR: when generating YAML for Job: %s", err)
			return err
		}
		glog.Infof("[kubic] final Job produced:\n%s", marshalled)
	}

	glog.V(3).Infof("[kubic] creating Job '%s' for installing certificates", jobName)
	err = r.Create(context.TODO(), job)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.V(1).Infof("[kubic] ERROR: when creating Job '%s': %s", jobName, err)
		return err
	}

	return nil
}
