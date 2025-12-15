package controller

import (
	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccessPolicy struct {
	Object *garagev1alpha1.AccessPolicy
}

// PolicyAssignment conditions
const (
	PolicyAssignmentReady string = "PolicyReady"

	PolicyBucketReady    string = "PolicyBucketReady"
	PolicyAccessKeyReady string = "PolicyAccessKeyReady"
)

const (
	ReasonAccessKeyMissing string = "AccessKeyMissing"
	ReasonBucketMissing    string = "BucketMissing"

	// Will not recover without spec change
	ReasonDependencyFailed string = "DependencyFailed"
	// Not ready, waiting
	ReasonDependencyNotReady string = "DependencyNotReady"
	// Transient errors
	ReasonDegraded string = "DependencyDegraded"
)

func initializePolicyConditions(p *garagev1alpha1.AccessPolicy) {
	conditions := []string{Ready, PolicyAccessKeyReady, PolicyBucketReady, PolicyAssignmentReady}
	initResourceConditions(conditions, &p.Status.Conditions)
}

// markPolicyConditionNotReady sets a condition type specific to the AccessPolicy to not ready.
func markPolicyConditionNotReady(k *garagev1alpha1.AccessPolicy,
	condType,
	reason,
	message string,
	args ...any,
) {
	markNotReady(
		k,
		&k.Status.Conditions,
		condType,
		reason,
		message,
		args...)
}

func updateAccessPolicyCondition(k *garagev1alpha1.AccessPolicy) {
	readyStat := metav1.ConditionFalse
	readyReason := ReasonDependencyNotReady
	readyMessage := "Waiting for conditions " + AccessKeyReady + " and " + BucketReady

	accessKeyCond := meta.FindStatusCondition(k.Status.Conditions, PolicyAccessKeyReady)
	bucketCond := meta.FindStatusCondition(k.Status.Conditions, PolicyBucketReady)
	policyCond := meta.FindStatusCondition(k.Status.Conditions, PolicyAssignmentReady)

	if accessKeyCond.Status == metav1.ConditionTrue &&
		bucketCond.Status == metav1.ConditionTrue &&
		policyCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = "ResourcesReady"
		readyMessage = "All conditions met"
	}

	readyCondition := metav1.Condition{
		Type:               Ready,
		Status:             readyStat,
		Reason:             readyReason,
		Message:            readyMessage,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: k.GetGeneration(),
	}
	meta.SetStatusCondition(&k.Status.Conditions, readyCondition)

	if readyCondition.Status == metav1.ConditionTrue {
		k.Status.ObservedGeneration = k.GetGeneration()
	}
}
