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
	"path/filepath"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubicutil "github.com/kubic-project/registries-operator/pkg/util"
)

const (
	// the image used in the Job for performing the copies
	jobImage = "busybox:latest"

	// directory in the Job where secrets will be mounted
	jobSecretsDir = "/secrets"
)

var (
	// Template for the Job
	jobTemplate = batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "unset", // this will be set
			Labels: map[string]string{},
		},
		Spec: batchv1.JobSpec{
			//Parallelism: 0,  // to be set
			//Completions: 0,  // to be set
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Tolerations: []corev1.Toleration{
						{
							Key:      "node-role.kubernetes.io/master",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
						{
							Key:      "CriticalAddonsOnly",
							Operator: corev1.TolerationOpExists,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "unset", // this will be set
							Image:           jobImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{}, // this will be set...
						},
					},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									TopologyKey: "kubernetes.io/hostname",
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											// to be set
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

type runnerWithSecrets struct {
	Commands     []string
	JobName      string
	JobNamespace string
	NumNodes     int32
	Secrets      map[string]*corev1.Secret
	Labels       map[string]string
	AntiAffinity map[string]string
	HostPaths    []string
}

// getRunnerJobWithSecrets gets a Job for running some commands on a specific node
// mounting some secrets
func getRunnerJobWithSecrets(cfg *runnerWithSecrets) (*batchv1.Job, error) {
	job := jobTemplate.DeepCopy()

	job.Name = cfg.JobName
	job.Namespace = cfg.JobNamespace
	job.Spec.Completions = &cfg.NumNodes
	job.Spec.Parallelism = &cfg.NumNodes

	jobSpec := &job.Spec.Template.Spec
	jobCont0 := &jobSpec.Containers[0]

	jobCont0.Name = cfg.JobName
	jobCont0.Args = cfg.Commands

	// copy all the labels
	for k, v := range cfg.Labels {
		job.ObjectMeta.Labels[k] = v
	}

	// mount all the secrets in the "/secrets" directory
	jobSpec.Volumes = []corev1.Volume{}
	jobCont0.VolumeMounts = []corev1.VolumeMount{}

	for hostPort, secret := range cfg.Secrets {
		name := kubicutil.SafeID(hostPort)

		newVolume := corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secret.GetName(),
				},
			},
		}
		jobSpec.Volumes = append(jobSpec.Volumes, newVolume)

		newVolumeMount := corev1.VolumeMount{
			Name:      name,
			MountPath: filepath.Join(jobSecretsDir, hostPort),
			ReadOnly:  true,
		}
		jobCont0.VolumeMounts = append(jobCont0.VolumeMounts, newVolumeMount)
	}

	jobSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels = cfg.AntiAffinity

	// add all the extra "hostPaths"
	for hostPathNum, hostPath := range cfg.HostPaths {
		name := fmt.Sprintf("host-path-%d", hostPathNum)
		newVolume := corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
				},
			},
		}
		jobSpec.Volumes = append(jobSpec.Volumes, newVolume)

		newVolumeMount := corev1.VolumeMount{
			Name:      name,
			MountPath: hostPath,
		}
		jobCont0.VolumeMounts = append(jobCont0.VolumeMounts, newVolumeMount)
	}

	return job, nil
}
