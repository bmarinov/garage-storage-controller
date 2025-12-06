package controller

import (
	"fmt"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: a wrapper on bucket might be overkill here: no dependent objects
type Bucket struct {
	Object *garagev1alpha1.Bucket
}

// Ready is the top-level ready condition for a resource.
const Ready string = "Ready"

const (
	BucketReady string = "BucketReady"
)

func (b *Bucket) InitializeConditions() {
	conditions := []string{Ready, BucketReady}
	initResourceConditions(conditions, &b.Object.Status.Conditions)
}

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

func (b *Bucket) MarkBucketNotReady(
	reason,
	message string,
	args ...any) {
	cond := metav1.Condition{
		Type:               BucketReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            fmt.Sprintf(message, args...),
		ObservedGeneration: b.Object.Generation,
	}

	meta.SetStatusCondition(&b.Object.Status.Conditions, cond)
	b.updateReadyCondition()
}

func (b *Bucket) MarkBucketReady() {
	cond := metav1.Condition{
		Type:               BucketReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: b.Object.Generation,
		Reason:             "BucketReady",
		Message:            "Bucket resource is ready",
	}
	meta.SetStatusCondition(&b.Object.Status.Conditions, cond)
	b.updateReadyCondition()
}

func (b *Bucket) updateReadyCondition() {
	bucketCond := meta.FindStatusCondition(b.Object.Status.Conditions, BucketReady)

	readyStat := metav1.ConditionFalse
	readyReason := "WaitingForBucket"
	readyMessage := "Waiting for " + BucketReady

	if bucketCond != nil && bucketCond.Status == metav1.ConditionTrue {
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
		ObservedGeneration: b.Object.GetGeneration(),
	}
	meta.SetStatusCondition(&b.Object.Status.Conditions, readyCondition)

	if readyCondition.Status == metav1.ConditionTrue {
		b.Object.Status.ObservedGeneration = b.Object.GetGeneration()
	}
}
