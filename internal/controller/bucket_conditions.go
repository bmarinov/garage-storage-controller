package controller

import (
	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BucketReady                 string = "BucketReady"
	BucketConfigMapReady        string = "BucketConfigMapReady"
)

func initializeBucketConditions(b *garagev1alpha1.Bucket) {
	conditions := []string{Ready, BucketReady}
	initResourceConditions(conditions, &b.Status.Conditions)
}

func markBucketNotReady(b *garagev1alpha1.Bucket,
	reason,
	message string,
	args ...any,
) {
	markNotReady(
		b,
		&b.Status.Conditions,
		BucketReady,
		reason,
		message,
		args...)
}

func markBucketReady(b *garagev1alpha1.Bucket) {
	cond := metav1.Condition{
		Type:               BucketReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: b.Generation,
		Reason:             "BucketReady",
		Message:            "Bucket resource is ready",
	}
	meta.SetStatusCondition(&b.Status.Conditions, cond)
}

func updateBucketReadyCondition(b *garagev1alpha1.Bucket) {
	bucketCond := meta.FindStatusCondition(b.Status.Conditions, BucketReady)

	readyStat := metav1.ConditionFalse
	readyReason := "WaitingForBucket"
	readyMessage := "Waiting for " + BucketReady

	if bucketCond != nil && bucketCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = defaultReadyReason
		readyMessage = defaultReadyMessage
	}

	readyCondition := metav1.Condition{
		Type:               Ready,
		Status:             readyStat,
		Reason:             readyReason,
		Message:            readyMessage,
		ObservedGeneration: b.GetGeneration(),
	}
	meta.SetStatusCondition(&b.Status.Conditions, readyCondition)

	if readyCondition.Status == metav1.ConditionTrue {
		b.Status.ObservedGeneration = b.GetGeneration()
	}
}
