package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ready is the top-level ready condition for a resource.
const Ready string = "Ready"

const (
	defaultReadyReason  = "Ready"
	defaultReadyMessage = "All conditions met"
)

func initResourceConditions(allConditions []string, conditions *[]metav1.Condition) {
	now := metav1.Now()

	for _, cond := range allConditions {
		if meta.FindStatusCondition(*conditions, cond) == nil {
			cond := metav1.Condition{
				Type:               cond,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: now,
				Reason:             "Pending",
				Message:            "Condition unknown",
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
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            fmt.Sprintf(message, args...),
		ObservedGeneration: objMeta.GetGeneration(),
	}

	meta.SetStatusCondition(conditions, cond)
}
