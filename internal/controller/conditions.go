package controller

import (
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
