/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ready is the top-level ready condition for a resource.
const Ready string = "Ready"

const finalizerName = "garage.getclustered.net/finalizer"

const (
	defaultReadyReason  = "Available"
	defaultReadyMessage = "All conditions met"
)

var errNameConflict = errors.New("name conflict with existing resource")

func initResourceConditions(allConditions []string, conditions *[]metav1.Condition) {
	for _, cond := range allConditions {
		if meta.FindStatusCondition(*conditions, cond) == nil {
			cond := metav1.Condition{
				Type:    cond,
				Status:  metav1.ConditionUnknown,
				Reason:  "Pending",
				Message: "Condition unknown",
			}
			meta.SetStatusCondition(conditions, cond)
		}
	}
}

func markNotReady(
	objMeta metav1.Object,
	conditions *[]metav1.Condition,
	condType string,
	reason,
	message string,
	args ...any) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            fmt.Sprintf(message, args...),
		ObservedGeneration: objMeta.GetGeneration(),
	}

	meta.SetStatusCondition(conditions, cond)
}
