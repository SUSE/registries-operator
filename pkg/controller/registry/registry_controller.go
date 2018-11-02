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

	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
)

const (
	// the registries controller name
	regsControllerName = "KubicRegistriesController"

	// certificates directory
	dockerCertsDir = "/etc/docker/certs.d/"
)

var (
	errAlreadyRunning = fmt.Errorf("Job already running")
)

// newRegistryReconcilier returns a new reconcile.Reconciler
func newRegistryReconcilier(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRegistry{
		Client:        mgr.GetClient(),
		EventRecorder: mgr.GetRecorder(regsControllerName),
		scheme:        mgr.GetScheme(),
	}
}

// Add creates a new Registry Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return addRegController(mgr, newRegistryReconcilier(mgr))
}

// addRegController adds a new Controller to mgr with r as the reconcile.Reconciler
func addRegController(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	regController, err := controller.New(regsControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Registry
	err = regController.Watch(&source.Kind{Type: &kubicv1beta1.Registry{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch the Nodes, in particular Nodes creations
	nodePredicates := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true // the only important thing: a new node appears
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false // there is nothing we can do when the node is deleted
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
	if err = regController.Watch(&source.Kind{Type: &corev1.Node{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: allRegistryMapper{mgr.GetClient()}},
		nodePredicates); err != nil {
		return err
	}

	// Watch the Secrets, just in case the secret in the registry changes
	if err = regController.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: secretToRegistryMapper{mgr.GetClient()},
	}); err != nil {
		return err
	}

	// Watch the Job created by Registry
	err = regController.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kubicv1beta1.Registry{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRegistry{}

// ReconcileRegistry reconciles a Registry object
type ReconcileRegistry struct {
	client.Client
	record.EventRecorder
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Registry object and makes changes based
// on the state read and what is in the Registry.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Jobs
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubic.opensuse.org,resources=registries,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileRegistry) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	glog.V(5).Infof("[kubic] trying to reconcile %s", request.NamespacedName)

	// Fetch the Registry instance
	registry := &kubicv1beta1.Registry{}
	ctx := context.Background()
	if err := r.Get(ctx, request.NamespacedName, registry); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			glog.V(3).Infof("[kubic] %s not found (%s)... ignoring", request.NamespacedName, err)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Get the list of nodes in the cluster
	curNodes, err := getAllNodes(r)
	if err != nil {
		glog.V(1).Infof("[kubic] ERROR: when getting the list of Nodes in the cluster: %s", err)
		return reconcile.Result{}, err
	}

	// check if the object is being removed and, in this case, delete all related objects
	finalizing, err := r.finalizerCheck(registry)
	if err != nil {
		return reconcile.Result{}, err
	}

	if finalizing {
		if len(registry.Status.Certificate.CurrentHash) > 0 {
			err = r.reconcileCertMissing(registry, curNodes)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if registry.Spec.Certificate != nil {
			specSecret, err := registry.GetCertificateSecret(r)
			if err != nil {
				return reconcile.Result{}, err
			}

			rr, err := r.reconcileCertPresent(registry, curNodes, specSecret)
			if err != nil {
				return rr, err
			}
		} else {
			// trigger a certificate removal when Spec.Certificate=nil and Status.Certificate!=nil
			if len(registry.Status.Certificate.CurrentHash) > 0 {
				glog.V(3).Infof("[kubic] certificate has disappeared for %s: removing certificate", registry)
				err = r.reconcileCertMissing(registry, curNodes)
				if err != nil {
					return reconcile.Result{}, err
				}
			} else {
				glog.V(3).Infof("[kubic] no certificate for %s: not reconciliation needed", registry)
			}
		}

	}

	if err := r.Update(ctx, registry); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
	}

	return reconcile.Result{}, nil
}

// finalizerCheck checks if the object is being finalized and, in that case,
// remove all the related objects
func (r *ReconcileRegistry) finalizerCheck(instance *kubicv1beta1.Registry) (bool, error) {
	// Helper functions to check and remove string from a slice of strings.
	containsString := func(slice []string, s string) bool {
		for _, item := range slice {
			if item == s {
				return true
			}
		}
		return false
	}

	finalizing := false
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !containsString(instance.ObjectMeta.Finalizers, regsFinalizerName) {
			glog.V(3).Infof("[kubic] '%s' does not have finalizer '%s' registered: adding it", instance.GetName(), regsFinalizerName)
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, regsFinalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				return false, err
			}
		}
	} else {
		glog.V(3).Infof("[kubic] '%s' is being deleted", instance.GetName())
		finalizing = true
	}

	return finalizing, nil
}

// finalizerDone marks the instance as "we are done with it, you can remove it now"
// Removal of the `instance` is blocked until we run this function, so make sure you don't
// forget about calling it...
func (r *ReconcileRegistry) finalizerDone(instance *kubicv1beta1.Registry) error {
	// Helper functions to check and remove string from a slice of strings.
	removeString := func(slice []string, s string) (result []string) {
		for _, item := range slice {
			if item == s {
				continue
			}
			result = append(result, item)
		}
		return
	}

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		panic(fmt.Sprintf("logic error: called finalizerDone() on %s when it was not being destroyed", instance.GetName()))
	}

	glog.V(3).Infof("[kubic] we are done with '%s': it can be safely terminated now.", instance.GetName())
	// remove our finalizer from the list and update it.
	instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, regsFinalizerName)

	return nil
}
