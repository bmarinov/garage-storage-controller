// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
