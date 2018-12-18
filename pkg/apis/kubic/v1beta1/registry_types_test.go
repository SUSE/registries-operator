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

package v1beta1

import (
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"github.com/kubic-project/registries-operator/pkg/test"
	"testing"
)

func TestGetCertificateFound(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewFakeClient()

	r, err := GetTestRegistry("foo");

	if err != nil  {
		t.Errorf("Error Getting Registry %v", err)
	}


	expected, err := test.BuildSecretFromCert("foo-ca-crt","foo.crt")

	if err != nil  {
		t.Errorf("Error creating secret %v", err)
	}

	c.Create(context.TODO(), expected)

	s, err := r.GetCertificateSecret(c)

	g.Expect(s).To(Equal(expected))
}

func TestGetCertificateNotFound(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewFakeClient()

	r, err := GetTestRegistry("foo");
	if err != nil  {
		t.Errorf("Error Getting Registry %v", err)
	}

	_, err = r.GetCertificateSecret(c)

	g.Expect(err).Should(HaveOccurred())
}

func TestGetCertificateWithoutSecret(t *testing.T) {

	g := NewGomegaWithT(t)

	c := fake.NewFakeClient()

	r := Registry{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: RegistrySpec{
			HostPort: "foo.com:5000",
		},
	}

	s, err := r.GetCertificateSecret(c)

	g.Expect(s).Should(BeNil())
	g.Expect(err).ShouldNot(HaveOccurred())
}
