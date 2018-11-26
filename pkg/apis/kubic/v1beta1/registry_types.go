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
	"fmt"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RegistrySpec defines the desired state of Registry
type RegistrySpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// HostPort is the registry HOST:PORT address (ie, "registry.suse.com:5000")
	HostPort string `json:"hostPort,omitempty"`

	// Name of the certificate (stored in a Secret) to use for this registry
	// +optional
	Certificate *v1.SecretReference `json:"certificate,omitempty"`
}

// RegistryStatus defines the observed state of Registry
type RegistryStatus struct {
	// Important: Run "make" to regenerate code after modifying this file
	Certificate RegistryCertificateStatus
}

// RegistryCertificateStatus defines the observed state of Registry
type RegistryCertificateStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// CertHash is the hash of the certificate that has been installed in the Nodes
	// When this hash changes, all the Nodes must be invalidated.
	// +optional
	CurrentHash string

	// Number of Nodes where this has been installed
	NumNodes int
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// Registry is the Schema for the registries API
// +k8s:openapi-gen=true
type Registry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistrySpec   `json:"spec,omitempty"`
	Status RegistryStatus `json:"status,omitempty"`
}

// GetCertificateSecret gets the certificate for a registry
func (registry Registry) GetCertificateSecret(r client.Client) (*v1.Secret, error) {
	if registry.Spec.Certificate == nil {
		return nil, nil
	}

	secret := &v1.Secret{}
	err := r.Get(nil, types.NamespacedName{Name: registry.Spec.Certificate.Name, Namespace: registry.Spec.Certificate.Namespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil

}

// String returns registry HOST:PORT formatted address
func (registry Registry) String() string {
	return fmt.Sprintf("%s", registry.Spec.HostPort)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// RegistryList contains a list of Registry
type RegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Registry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Registry{}, &RegistryList{})
}
