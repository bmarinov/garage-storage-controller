package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// checkCondition finds a condition of given type and checks whether
// it has the expected status.
func checkCondition(
	conditions []metav1.Condition,
	condType string,
	status metav1.ConditionStatus,
) error {
	cond := meta.FindStatusCondition(conditions, condType)
	if cond == nil {
		return fmt.Errorf("condition of type %s not found", condType)
	}
	if cond.Status != status {
		return fmt.Errorf("condition %s has status %s, expected %s", condType, cond.Status, status)
	}

	return nil
}
