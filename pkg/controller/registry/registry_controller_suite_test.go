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
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubic-project/registries-operator/pkg/apis"
	"github.com/kubic-project/registries-operator/pkg/test"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	var t *envtest.Environment

	useExistingCluster := os.Getenv("KUBECONFIG") != ""

	if test.ShouldRunIntegrationSetupAndTeardown(m) {
		t = &envtest.Environment{
			UseExistingCluster: useExistingCluster,
			CRDDirectoryPaths:  []string{filepath.Join("..", "..", "..", "config", "crds")},
		}
		apis.AddToScheme(scheme.Scheme)

		var err error
		if cfg, err = t.Start(); err != nil {
			log.Fatal(err)
		}
	}

	code := m.Run()

	if test.ShouldRunIntegrationSetupAndTeardown(m) {
		t.Stop()
	}

	os.Exit(code)
}

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconciler(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	recFn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})

	return recFn, requests
}

func SetupTestManager(t *testing.T) (manager.Manager, chan struct{}) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})

	if err != nil {
		t.Fatalf("Error creating manager %v", err)
	}

	err = addRegController(mgr, newRegistryReconcilier(mgr))
	if err != nil {
		t.Fatalf("Error adding Controller %v", err)
	}

	stop := StartTestManager(t, mgr)

	return mgr, stop
}

// StartTestManager adds recFn
func StartTestManager(t *testing.T, mgr manager.Manager) chan struct{} {
	stop := make(chan struct{})
	go func() {
		err := mgr.Start(stop)
		if err != nil {
			t.Errorf("Failed to start Manager: %v", err)
		}
	}()
	return stop
}
