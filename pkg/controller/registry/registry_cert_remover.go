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

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	kubicutil "github.com/kubic-project/registries-operator/pkg/util"
)

const (
	// a prefix for all the jobs created for removing certificates
	jobRemoveNamePrefix = "kubic-registry-remover"

	// some labels in jobs that remove certificates: the node where this job should run
	jobRemoveLabelHostPort = "kubic-registry-remover-host-port"

	// some labels in jobs that remove certificates: the hash of the CA.crt this Job is trying to remove
	jobRemoveLabelHash = "kubic-registry-remover-hash"

	// Name of the finalizer
	regsFinalizerName = "registry.finalizers.kubic.opensuse.org"
)

// reconcileCertMissing removes all the things created by the controller for a Registry
// Ensure that delete implementation is idempotent and safe to invoke
// multiple types for same object.
func (r *ReconcileRegistry) reconcileCertMissing(instance *kubicv1beta1.Registry, nodes map[string]*corev1.Node) error {

	mustRemove := false

	secretHash := instance.Status.Certificate.CurrentHash

	jobs, err := getAllJobsWithLabels(r, map[string]string{
		jobRemoveLabelHostPort: kubicutil.SafeID(instance.Spec.HostPort),
		jobRemoveLabelHash:     secretHash,
	})
	if err != nil {
		return err
	}
	glog.V(5).Infof("[kubic] %d removal Jobs found for %s '%s'", len(jobs), instance.Spec.HostPort, secretHash)

	if len(jobs) == 0 {
		if len(instance.Status.Certificate.CurrentHash) > 0 && instance.Status.Certificate.NumNodes != 0 {
			glog.V(3).Infof("[kubic] will start a removal Job")
			mustRemove = true
		}
	} else {
		// Process the certificate removal jobs
		job := jobs[0] // there should be not more than one job

		glog.V(3).Infof("[kubic] Job '%s': Active=%d, Failed=%d, Succeeded=%d",
			job.GetName(), job.Status.Active, job.Status.Failed, job.Status.Succeeded)

		if job.Status.Active > 0 {
			glog.V(3).Infof("[kubic] Job '%s' is still active... will let it finish", job.Name)
		} else {
			glog.V(3).Infof("[kubic] Job '%s' has finished", job.Name)

			if len(instance.Status.Certificate.CurrentHash) > 0 && instance.Status.Certificate.NumNodes > 0 {
				r.EventRecorder.Event(instance, corev1.EventTypeNormal,
					"Installed", fmt.Sprintf("Certificate '%s' successfully removed", secretHash))
				instance.Status.Certificate.CurrentHash = ""
				instance.Status.Certificate.NumNodes = 0
			}

			glog.V(3).Infof("[kubic] Job '%s' has completed its mission: removing it!", job.Name)
			if err = r.Delete(context.TODO(), &job); err != nil {
				if apierrors.IsGone(err) {
					return nil
				}
				return err
			}

			r.finalizerDone(instance)
		}
	}

	if mustRemove {
		// If no certs removal Jobs are runningg, start one
		glog.V(3).Infof("[kubic] deleting all the dependencies for %s", instance)
		r.EventRecorder.Event(instance, corev1.EventTypeNormal,
			"Removing", fmt.Sprintf("Removing certificate for '%s'...", instance))

		if err := r.removeCertForRegistry(instance, secretHash, len(nodes)); err != nil {
			return err
		}
	}

	// the instance must be updated at the upper layer
	return nil
}

// removeCertForRegistry creates a `Job` for removing the certificate in `registry`
func (r *ReconcileRegistry) removeCertForRegistry(registry *kubicv1beta1.Registry, secretHash string, numNodes int) error {
	var err error

	registryAddress := kubicutil.SafeID(registry.Spec.HostPort)
	jobName := kubicutil.SafeID(jobRemoveNamePrefix) + "-" + registryAddress

	dockerDstDir := filepath.Join(dockerCertsDir, registry.Spec.HostPort)
	podmanDstDir := filepath.Join(podmanCertsDir, registry.Spec.HostPort)

	commands := []string{
		fmt.Sprintf("echo Removing %s", dockerDstDir),
		fmt.Sprintf("rm -rf '%s'", dockerDstDir),
		fmt.Sprintf("echo Removing %s", podmanDstDir),
		fmt.Sprintf("rm -rf '%s'", podmanDstDir),
	}

	glog.V(3).Infof("[kubic] generating Job '%s'", jobName)
	job, err := getRunnerJobWithSecrets(&runnerWithSecrets{
		Commands:     []string{strings.Join(commands, " ; ")},
		JobName:      jobName,
		NumNodes:     int32(numNodes),
		JobNamespace: metav1.NamespaceSystem,
		Labels: map[string]string{
			jobRemoveLabelHostPort: registryAddress,
			jobRemoveLabelHash:     secretHash,
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
			glog.Infof("[kubic] error when generating YAML for Job: %s", err)
			return err
		}
		glog.Infof("[kubic] final Job produced:\n%s", marshalled)
	}

	glog.V(3).Infof("[kubic] creating Job '%s' for removing certificates", jobName)
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
