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

// Package test contains specific test utilities which can be used by operator
package test

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/kubic-project/registries-operator/pkg/test/assets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

// SkipUnlessIntegrationTesting should skip this specific test if we are not running in integration
// testing mode
func SkipUnlessIntegrationTesting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
}

// ShouldRunIntegrationSetupAndTeardown should skip this specific test if we are not running in
// integration testing mode
func ShouldRunIntegrationSetupAndTeardown(m *testing.M) bool {
	flag.Parse()
	return !testing.Short()
}

//Returns a secret object build from a certificate stored in the certificates directory
func BuildSecretFromCert(name string, certName string) (*corev1.Secret, error) {
	cert, ok := assets.Certs[certName]
	if !ok {
		return &corev1.Secret{}, fmt.Errorf("Certificate  %s not found", certName)
	}

	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string][]byte{"ca.crt": cert}}

	return secret, nil
}


// Prints Object is a readable format
func PrettyPrint(v interface{}) (err error) {
      b, err := json.MarshalIndent(v, "", "  ")
      if err == nil {
              fmt.Println(string(b))
      }
      return err
}
