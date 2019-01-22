package v1beta1

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var fooRegistry = &Registry{
	ObjectMeta: metav1.ObjectMeta{
		Name: "foo",
		Namespace: metav1.NamespaceSystem,
	},
	Spec: RegistrySpec{
		HostPort: "foo.com:5000", 
		Certificate: &corev1.SecretReference{
			Name: "foo-ca-crt", 
			Namespace: metav1.NamespaceSystem,
		},
	},
}

var barRegistry = &Registry{
	ObjectMeta: metav1.ObjectMeta{
		Name: "bar",
		Namespace: metav1.NamespaceSystem,
	},
	Spec: RegistrySpec{
		HostPort: "bar.com:5000",
		Certificate: &corev1.SecretReference{
			Name: "bar-ca-crt",
			Namespace: metav1.NamespaceSystem,
		},
	},
}

var registries = map[string]*Registry{
	"foo": fooRegistry,
	"bar": barRegistry,
}

func GetTestRegistry(name string) (*Registry, error){
	reg, ok := registries[name]
	if !ok {
		return &Registry{}, fmt.Errorf("Registry  %s not found", name)
	}

	//add a second Registry with the same certificate
	return reg.DeepCopy(), nil
}
