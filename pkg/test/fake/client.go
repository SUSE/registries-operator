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

package fake 

import (
	"context"
	"fmt"
	"github.com/kubic-project/registries-operator/pkg/apis/kubic/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"strings"
)

/*
 * The testClient wraps the fakeClient provided by the controller-runtime and backports
 * the fix for handling a sigfault in the List method if explicit metadata about the list's
 * elements is not provided
 *
 * TODO: Remove this wrapper once the project is upgrded to a runtime v0.1.8 or up
 */
type testClient struct {
	fake   client.Client
	scheme *runtime.Scheme
}

func NewTestClient(initObjs ...runtime.Object) client.Client {
	s := scheme.Scheme
	v1beta1.SchemeBuilder.AddToScheme(s)
	return &testClient{fake: fake.NewFakeClient(initObjs...), scheme: s}
}

func (c *testClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return c.fake.Get(ctx, key, obj)
}

func (c *testClient) Create(ctx context.Context, obj runtime.Object) error {
	return c.fake.Create(ctx, obj)
}

func (c *testClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOptionFunc) error {
	return c.fake.Delete(ctx, obj, opts...)
}

func (c *testClient) Update(ctx context.Context, obj runtime.Object) error {
	return c.fake.Update(ctx, obj)
}

type testStatusWriter struct {
	client *testClient
}

func (c *testClient) Status() client.StatusWriter {
	return &testStatusWriter{client: c}
}

func (sw *testStatusWriter) Update(ctx context.Context, obj runtime.Object) error {
	return sw.client.Update(ctx, obj)
}

func (c *testClient) List(ctx context.Context, opts *client.ListOptions, list runtime.Object) error {
	gvk, err := getGVKFromList(list, c.scheme)
	if err != nil {
		// The old fake client required GVK info in Raw.TypeMeta, so check there
		// before giving up
		if opts.Raw.TypeMeta.APIVersion == "" || opts.Raw.TypeMeta.Kind == "" {
			return err
		}
		gvk = opts.Raw.TypeMeta.GroupVersionKind()
	}

	opts.Raw = &metav1.ListOptions{TypeMeta: metav1.TypeMeta{APIVersion: gvk.Group + "/" + gvk.Version, Kind: gvk.Kind}}

	return c.fake.List(ctx, opts, list)
}

//This function is copied from the fakeClient v0.1.8
//it provides the type metadata for the elements of a list
func getGVKFromList(list runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvk, err := apiutil.GVKForObject(list, scheme)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	if !strings.HasSuffix(gvk.Kind, "List") {
		return schema.GroupVersionKind{}, fmt.Errorf("non-list type %T (kind %q) passed as output", list, gvk)
	}
	// we need the non-list GVK, so chop off the "List" from the end of the kind
	gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	return gvk, nil
}
