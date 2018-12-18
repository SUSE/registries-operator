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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"testing"
)

func TestMapSecretAllRegistries(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewTestClient()

	fooReg, err :=kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	barReg, err :=kubicv1beta1.GetTestRegistry("bar")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	c.Create(context.TODO(), fooReg)
	c.Create(context.TODO(), barReg)

	rm:= allRegistryMapper{c}

	event:= handler.MapObject{}
	requests := rm.Map(event)

        g.Expect(requests).To(HaveLen(2))

}

func TestMapSecreNoRegistry(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewTestClient()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}

	barReg, err :=kubicv1beta1.GetTestRegistry("bar")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	c.Create(context.TODO(), fooSec)
	c.Create(context.TODO(),barReg)

	event := handler.MapObject{Meta: &fooSec.ObjectMeta, Object: fooSec}

	rm := secretToRegistryMapper{c}
	requests := rm.Map(event)
	g.Expect(requests).To(HaveLen(0))

}

func TestMapSecretOneRegistry(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewTestClient()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}
	c.Create(context.TODO(), fooSec)

	fooReg, err :=kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}
	c.Create(context.TODO(),fooReg)

	barReg, err :=kubicv1beta1.GetTestRegistry("bar")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}
	c.Create(context.TODO(),barReg)

	event := handler.MapObject{Meta: &fooSec.ObjectMeta, Object: fooSec}

	rm := secretToRegistryMapper{c}
	requests := rm.Map(event)
	g.Expect(requests).To(HaveLen(1))

}


func TestMapSecretTwoRegistries(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewTestClient()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}

	fooReg, err :=kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}

	//Add a Registry and its certitifate
	c.Create(context.TODO(), fooSec)
	c.Create(context.TODO(),fooReg)

	//add a second Registry with the same certificate
	fooReg2 := fooReg.DeepCopy()
	fooReg2.ObjectMeta.Name = "foo2"
	c.Create(context.TODO(),fooReg2)

	event := handler.MapObject{Meta: &fooSec.ObjectMeta, Object: fooSec}

	rm := secretToRegistryMapper{c}
	requests := rm.Map(event)
	g.Expect(requests).To(HaveLen(2))

}


func TestMapSecretDifferentNs(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewTestClient()

	fooSec, err := test.BuildSecretFromCert("foo-ca-crt", "foo.crt")

	if err != nil {
		t.Errorf("Error creating secret %v", err)
	}
	c.Create(context.TODO(), fooSec)

	fooReg, err :=kubicv1beta1.GetTestRegistry("foo")
	if err != nil {
		t.Errorf("Error Getting Registry %v", err)
	}
	//change registry's certificate namespace to force mismatch
	fooReg.Spec.Certificate.Namespace = "fakeNs"
	c.Create(context.TODO(),fooReg)

	event := handler.MapObject{Meta: &fooSec.ObjectMeta, Object: fooSec}

	rm := secretToRegistryMapper{c}
	requests := rm.Map(event)
	g.Expect(requests).To(HaveLen(0))

}
