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

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BucketReady                       string = "BucketReady"
	BucketConfigMapReady              string = "BucketConfigMapReady"
	ReasonConfigMapNameConflict       string = "ConfigMapNameConflict"
	ReasonOwnerKeySecretNotFound      string = "OwnerKeySecretNotFound"
	ReasonOwnershipVerificationFailed string = "OwnershipVerificationFailed"
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

	if bucketCond == nil || cmCond == nil {
		return
	}

	if bucketCond.Status == metav1.ConditionTrue &&
		cmCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = defaultReadyReason
		readyMessage = defaultReadyMessage
	} else {
		switch {
		case bucketCond.Status == metav1.ConditionFalse:
			readyReason = bucketCond.Reason
			readyMessage = bucketCond.Message
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
