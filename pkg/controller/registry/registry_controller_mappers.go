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

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
)

// A mapper that returns all the Registries
type allRegistryMapper struct {
	client.Client
}

func (nrm allRegistryMapper) Map(obj handler.MapObject) []reconcile.Request {
	res := []reconcile.Request{}

	registries := &kubicv1beta1.RegistryList{}
	if err := nrm.List(context.TODO(), &client.ListOptions{}, registries); err != nil {
		glog.V(1).Infof("[kubic] ERROR: when getting the list of Registries in the cluster: %s", err)
		return res
	}

	// Add all the Registries that use this Secret
	for _, registry := range registries.Items {
		res = append(res, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      registry.GetName(),
				Namespace: registry.GetNamespace(),
			},
		})
	}
	return res
}

// A mapper from Secret to Registries that use that Secret
type secretToRegistryMapper struct {
	client.Client
}

func (srm secretToRegistryMapper) Map(obj handler.MapObject) []reconcile.Request {
	res := []reconcile.Request{}
	secret, ok := obj.Object.(*corev1.Secret)
	if !ok {
		return res // This wasn't a Secret
	}

	registries := &kubicv1beta1.RegistryList{}
	if err := srm.List(context.TODO(), &client.ListOptions{}, registries); err != nil {
		glog.V(1).Infof("[kubic] ERROR: when getting the list of Registries in the cluster: %s", err)
		return res
	}

	// Add all the Registries that use this Secret
	for _, registry := range registries.Items {
		if registry.Spec.Certificate.Name == secret.GetName() && registry.Spec.Certificate.Namespace == secret.GetNamespace() {
			res = append(res, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registry.GetName(),
					Namespace: registry.GetNamespace(),
				},
			})
		}
	}
	return res
}
