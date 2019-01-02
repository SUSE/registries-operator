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
	kubicv1beta1 "github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	"github.com/kubic-project/registries-operator/pkg/test"
	"github.com/kubic-project/registries-operator/pkg/test/fake"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type FakeCertReconciler struct {
	reconcileMissing bool
	reconcilePresent bool
}

func NewFakeCertReconciler() *FakeCertReconciler {
	return &FakeCertReconciler{false, false}
}

func newTestReconcileRegistry() ReconcileRegistry {
	return ReconcileRegistry{
		fake.NewTestClient(),
		fake.NewTestRecorder(),
		scheme.Scheme,
		NewFakeCertReconciler(),
	}
}

func (r *FakeCertReconciler) ReconcileCertPresent(registry *kubicv1beta1.Registry,
	curNodes map[string]*corev1.Node,
	specSecret *corev1.Secret) (reconcile.Result, error) {

	r.reconcilePresent = true
	return reconcile.Result{}, nil
}

func (r *FakeCertReconciler) ReconcileCertMissing(instance *kubicv1beta1.Registry, nodes map[string]*corev1.Node) error {
	r.reconcileMissing = true
	return nil

}

func (r FakeCertReconciler) ReconcileCertPresentCalled() bool {
	return r.reconcilePresent && !r.reconcileMissing
}

func (r FakeCertReconciler) ReconcileCertMissingCalled() bool {
	return r.reconcileMissing && !r.reconcilePresent
}

func (r FakeCertReconciler) ReconcileCertNotCalled() bool {
	return !r.reconcileMissing && !r.reconcilePresent
}

//checks if the registry is finalizing
func (r *ReconcileRegistry) isFinalizing(registry string) (bool, error) {

	instance := &kubicv1beta1.Registry{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: registry}, instance)

	if err == nil {
		return instance.ObjectMeta.Finalizers[0] == regsFinalizerName, nil
	} else {
		return false, err
	}
}

func TesRegistryNotFound(t *testing.T) {

	g := NewGomegaWithT(t)

	r := newTestReconcileRegistry()

	req := reconcile.Request{types.NamespacedName{Name: "TestRegistry", Namespace: ""}}
	res, _ := r.Reconcile(req)

	g.Expect(res).To(Equal(reconcile.Result{}))
	//neither of reconcile methods should be called
	cr, _ := r.certReconciler.(*FakeCertReconciler)
	g.Expect(cr.ReconcileCertNotCalled()).Should(Equal(true))

}

func TestRegistryFinalizing(t *testing.T) {

	g := NewGomegaWithT(t)

	r := newTestReconcileRegistry()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}

	fooReg, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}
	//simulate registry is being deleted
	timestamp := metav1.Now()
	fooReg.ObjectMeta.SetDeletionTimestamp(&timestamp)

	//simulate the registry had a certificated deployed
	fooReg.Status.Certificate.CurrentHash = getSecretHash(fooSec)

	c := r.Client
	c.Create(context.TODO(), fooSec)
	c.Create(context.TODO(), fooReg)

	req := reconcile.Request{types.NamespacedName{Name: fooReg.Name, Namespace: fooReg.Namespace}}
	res, _ := r.Reconcile(req)

	g.Expect(res).To(Equal(reconcile.Result{}))
	//reconcile cert should be called
	cr, _ := r.certReconciler.(*FakeCertReconciler)
	g.Expect(cr.ReconcileCertMissingCalled()).Should(Equal(true))

}

func TestRegistryFound(t *testing.T) {

	g := NewGomegaWithT(t)

	r := newTestReconcileRegistry()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}

	fooReg, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	c := r.Client
	c.Create(context.TODO(), fooSec)
	c.Create(context.TODO(), fooReg)

	req := reconcile.Request{types.NamespacedName{Name: fooReg.Name, Namespace: fooReg.Namespace}}
	res, _ := r.Reconcile(req)

	g.Expect(res).To(Equal(reconcile.Result{}))
	//reconcile cert should be called
	cr, _ := r.certReconciler.(*FakeCertReconciler)
	g.Expect(cr.ReconcileCertPresentCalled()).Should(Equal(true))
	g.Expect(r.isFinalizing(fooReg.GetName())).Should(Equal(true))

}

func TestCertRemoved(t *testing.T) {

	g := NewGomegaWithT(t)

	r := newTestReconcileRegistry()

	fooReg, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")
	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}
	//simulate certificate was installed and then removed from Registry
	fooReg.Status.Certificate.CurrentHash = getSecretHash(fooSec)
	fooReg.Spec.Certificate = nil

	c := r.Client
	c.Create(context.TODO(), fooReg)

	req := reconcile.Request{types.NamespacedName{Name: fooReg.Name, Namespace: fooReg.Namespace}}
	res, _ := r.Reconcile(req)

	g.Expect(res).To(Equal(reconcile.Result{}))
	//reconcile cert should be called
	cr, _ := r.certReconciler.(*FakeCertReconciler)
	g.Expect(cr.ReconcileCertMissingCalled()).Should(Equal(true))

}

func TestRegistryWithoutCert(t *testing.T) {

	g := NewGomegaWithT(t)

	r := newTestReconcileRegistry()

	fooReg, err := kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	fooReg.Spec.Certificate = nil

	c := r.Client
	c.Create(context.TODO(), fooReg)

	req := reconcile.Request{types.NamespacedName{Name: fooReg.Name, Namespace: fooReg.Namespace}}
	res, _ := r.Reconcile(req)

	g.Expect(res).To(Equal(reconcile.Result{}))
	//neither of reconcile methods should be called
	cr, _ := r.certReconciler.(*FakeCertReconciler)
	g.Expect(cr.ReconcileCertNotCalled()).Should(Equal(true))
}
