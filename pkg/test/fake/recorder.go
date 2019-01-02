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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type FakeEvent struct {
	Object runtime.Object
	EventType string
	Reason string
	Message string
	Annotations map[string]string
}


type FakeRecorder struct {
	Events []FakeEvent
}


func NewTestRecorder() FakeRecorder {
	return FakeRecorder{Events: []FakeEvent{}}
}

func (e FakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	e.Events = append(e.Events, FakeEvent{object, eventtype, reason, message, map[string]string{}})
}

// Eventf is just like Event, but with Sprintf for the message field.
func (e FakeRecorder)Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}){
}

// PastEventf is just like Eventf, but with an option to specify the event's 'timestamp' field.
func (e FakeRecorder)PastEventf(object runtime.Object, timestamp metav1.Time, eventtype, reason, messageFmt string, args ...interface{}){
}

    // AnnotatedEventf is just like eventf, but with annotations attached
func (e FakeRecorder)AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}){
}


