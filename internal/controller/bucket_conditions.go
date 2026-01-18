package controller

import (
	"fmt"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BucketReady                 string = "BucketReady"
	BucketConfigMapReady        string = "BucketConfigMapReady"
	ReasonConfigMapNameConflict string = "ConfigMapNameConflict"
)

func initializeBucketConditions(b *garagev1alpha1.Bucket) {
	conditions := []string{Ready, BucketReady, BucketConfigMapReady}
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

func updateBucketCMCondition(b *garagev1alpha1.Bucket,
	stat metav1.ConditionStatus,
	reason, msg string,
	args ...any,
) {
	cond := metav1.Condition{
		Type:               BucketConfigMapReady,
		Status:             stat,
		Reason:             reason,
		Message:            fmt.Sprintf(msg, args...),
		ObservedGeneration: b.GetGeneration(),
	}
	meta.SetStatusCondition(&b.Status.Conditions, cond)
}

func updateBucketReadyCondition(b *garagev1alpha1.Bucket) {
	readyStat := metav1.ConditionFalse
	readyReason := "WaitingForBucket"
	readyMessage := "Waiting for " + BucketReady

	bucketCond := meta.FindStatusCondition(b.Status.Conditions, BucketReady)
	cmCond := meta.FindStatusCondition(b.Status.Conditions, BucketConfigMapReady)

	if bucketCond != nil && bucketCond.Status == metav1.ConditionTrue &&
		cmCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = defaultReadyReason
		readyMessage = defaultReadyMessage
	} else {
		switch {
		case bucketCond.Status == metav1.ConditionFalse:
			// TODO: any bucket states other than waiting / ready?
		case cmCond.Status == metav1.ConditionFalse:
			readyReason = cmCond.Reason
			readyMessage = cmCond.Message
		}
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
