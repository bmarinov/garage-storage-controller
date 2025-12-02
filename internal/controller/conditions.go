package controller

import (
	"fmt"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Bucket struct {
	Object *garagev1alpha1.Bucket
}

type BucketConditionType string

const (
	ConditionReady       BucketConditionType = "Ready"
	ConditionBucketReady BucketConditionType = "BucketReady"
)

func (b *Bucket) InitializeConditions() {
	if b.Object.Status.Conditions == nil {
		b.Object.Status.Conditions = []metav1.Condition{}
	}

	now := metav1.Now()

	conditions := []BucketConditionType{ConditionReady, ConditionBucketReady}
	for _, cond := range conditions {
		if meta.FindStatusCondition(b.Object.Status.Conditions, string(cond)) == nil {
			cond := metav1.Condition{
				Type:               string(cond),
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: now,
				Reason:             "Pending",
				Message:            "Condition unknown",
			}
			meta.SetStatusCondition(&b.Object.Status.Conditions, cond)
		}
	}
}

func (b *Bucket) MarkBucketNotReady(
	reason,
	message string,
	args ...any) {
	cond := metav1.Condition{
		Type:               string(ConditionBucketReady),
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            fmt.Sprintf(message, args...),
		ObservedGeneration: b.Object.Generation, // TODO: verify correct generation used
	}

	meta.SetStatusCondition(&b.Object.Status.Conditions, cond)
	b.updateReadyCondition()
}

func (b *Bucket) MarkBucketReady() {
	cond := metav1.Condition{
		Type:               string(ConditionBucketReady),
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
	// Top-level ready -> Bucket Ready -> Key Ready

	bucketCond := meta.FindStatusCondition(b.Object.Status.Conditions, string(ConditionBucketReady))

	readyStat := metav1.ConditionFalse
	readyReason := "WaitingForBucket"
	readyMessage := "Waiting for " + string(ConditionBucketReady)

	if bucketCond != nil && bucketCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = "ResourcesReady"
		readyMessage = "All conditions met"
	}

	readyCondition := metav1.Condition{
		Type:               string(ConditionReady),
		Status:             readyStat,
		Reason:             readyReason,
		Message:            readyMessage,
		LastTransitionTime: metav1.Now(),
	}
	meta.SetStatusCondition(&b.Object.Status.Conditions, readyCondition)
	b.Object.Status.ObservedGeneration = b.Object.GetGeneration()
}
