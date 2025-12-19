package controller

import (
	"fmt"

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
	ReasonDependenciesNotReady string = "DependenciesNotReady"
	// Transient errors
	ReasonDegraded string = "DependencyDegraded"
)

func initializePolicyConditions(p *garagev1alpha1.AccessPolicy) {
	conditions := []string{Ready, PolicyAccessKeyReady, PolicyBucketReady, PolicyAssignmentReady}
	initResourceConditions(conditions, &p.Status.Conditions)
}

// markPolicyConditionNotReady sets a condition type specific to the AccessPolicy to not ready.
func markPolicyConditionNotReady(p *garagev1alpha1.AccessPolicy,
	condType,
	reason,
	message string,
	args ...any,
) {
	markNotReady(
		p,
		&p.Status.Conditions,
		condType,
		reason,
		message,
		args...)
}

// markPolicyKeyReady to indicate that referenced AccessKey is ready.
func markPolicyKeyReady(p *garagev1alpha1.AccessPolicy) {
	cond := metav1.Condition{
		Type:               PolicyAccessKeyReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: p.GetGeneration(),
		Reason:             "AccessKeyReady",
		Message:            "AccessKey is ready for assignment",
	}
	meta.SetStatusCondition(&p.Status.Conditions, cond)
}

// markPolicyBucketReady to indicate that referenced Bucket is ready.
func markPolicyBucketReady(p *garagev1alpha1.AccessPolicy) {
	cond := metav1.Condition{
		Type:               PolicyBucketReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: p.GetGeneration(),
		Reason:             "BucketReady",
		Message:            "Bucket is ready for assignment",
	}
	meta.SetStatusCondition(&p.Status.Conditions, cond)
}

func markPolicyAssignmentReady(p *garagev1alpha1.AccessPolicy) {
	cond := metav1.Condition{
		Type:               PolicyAssignmentReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: p.GetGeneration(),
		Reason:             "AccessPolicyAssigned",
		Message:            "Access policy configured in external system.",
	}
	meta.SetStatusCondition(&p.Status.Conditions, cond)
}

func updateAccessPolicyCondition(p *garagev1alpha1.AccessPolicy) {
	readyStat := metav1.ConditionFalse
	readyReason := ReasonDependenciesNotReady
	readyMessage := "Waiting for conditions " + AccessKeyReady + " and " + BucketReady

	accessKeyCond := meta.FindStatusCondition(p.Status.Conditions, PolicyAccessKeyReady)
	bucketCond := meta.FindStatusCondition(p.Status.Conditions, PolicyBucketReady)
	policyCond := meta.FindStatusCondition(p.Status.Conditions, PolicyAssignmentReady)

	if accessKeyCond.Status == metav1.ConditionTrue &&
		bucketCond.Status == metav1.ConditionTrue &&
		policyCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = defaultReadyReason
		readyMessage = defaultReadyMessage
	} else {
		switch {
		case bucketCond.Status == metav1.ConditionFalse:
			readyReason = bucketCond.Reason
			readyMessage = fmt.Sprintf("Bucket not ready: %s", bucketCond.Message)
		case accessKeyCond.Status == metav1.ConditionFalse:
			readyReason = accessKeyCond.Reason
			readyMessage = fmt.Sprintf("Access key not ready: %s", accessKeyCond.Message)
		case policyCond.Status == metav1.ConditionFalse:
			readyReason = policyCond.Reason
			readyMessage = fmt.Sprintf("Policy assignment failed: %s", policyCond.Message)
		}
	}

	readyCondition := metav1.Condition{
		Type:               Ready,
		Status:             readyStat,
		Reason:             readyReason,
		Message:            readyMessage,
		ObservedGeneration: p.GetGeneration(),
	}
	meta.SetStatusCondition(&p.Status.Conditions, readyCondition)

	if readyCondition.Status == metav1.ConditionTrue {
		p.Status.ObservedGeneration = p.GetGeneration()
	}
}
