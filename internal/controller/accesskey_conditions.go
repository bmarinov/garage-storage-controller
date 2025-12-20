package controller

import (
	"fmt"
	"slices"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccessKey struct {
	Object *garagev1alpha1.AccessKey
}

const (
	AccessKeyReady           string = "AccessKeyReady"
	KeySecretReady           string = "KeySecretReady"
	ReasonSecretNameConflict string = "SecretNameConflict"
)

func (k *AccessKey) InitializeConditions() {
	conditions := []string{Ready, AccessKeyReady, KeySecretReady}
	initResourceConditions(conditions, &k.Object.Status.Conditions)
}

func markAccessKeyReady(k *garagev1alpha1.AccessKey) {
	cond := metav1.Condition{
		Type:               AccessKeyReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: k.GetGeneration(),
		Reason:             "AccessKeyReady",
		Message:            "External access key is ready",
	}
	meta.SetStatusCondition(&k.Status.Conditions, cond)
}

func markAccessKeyNotReady(k *garagev1alpha1.AccessKey,
	reason,
	message string,
	args ...any,
) {
	markNotReady(
		k,
		&k.Status.Conditions,
		AccessKeyReady,
		reason,
		message,
		args...)
}

func (k *AccessKey) markNotReady(condType string,
	reason,
	message string,
	args ...any) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            fmt.Sprintf(message, args...),
		ObservedGeneration: k.Object.Generation,
	}

	meta.SetStatusCondition(&k.Object.Status.Conditions, cond)
}

func markSecretReady(k *garagev1alpha1.AccessKey) {
	cond := metav1.Condition{
		Type:               KeySecretReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: k.GetGeneration(),
		Reason:             "SecretReady",
		Message:            "Secret for external access key is ready",
	}
	meta.SetStatusCondition(&k.Status.Conditions, cond)
}

// externalKeyReadyStat returns the status of the AccessKeyReady condition.
func (k *AccessKey) externalKeyReadyStat() metav1.ConditionStatus {
	cond := meta.FindStatusCondition(k.Object.Status.Conditions, AccessKeyReady)
	return cond.Status
}

func updateAccessKeyCondition(k *garagev1alpha1.AccessKey) {
	readyStat := metav1.ConditionFalse
	readyReason := "WaitingForResources"
	readyMessage := "Waiting for conditions " + AccessKeyReady + " and " + KeySecretReady

	accessKeyCond := meta.FindStatusCondition(k.Status.Conditions, AccessKeyReady)
	secretCond := meta.FindStatusCondition(k.Status.Conditions, KeySecretReady)
	if accessKeyCond.Status == metav1.ConditionTrue &&
		secretCond.Status == metav1.ConditionTrue {
		readyStat = metav1.ConditionTrue
		readyReason = defaultReadyReason
		readyMessage = defaultReadyMessage
	} else {
		switch {
		case accessKeyCond.Status == metav1.ConditionFalse:
			readyReason = accessKeyCond.Reason
			readyMessage = accessKeyCond.Message
		case secretCond.Status == metav1.ConditionFalse:
			readyReason = secretCond.Reason
			readyMessage = secretCond.Message
		}
	}

	readyCondition := metav1.Condition{
		Type:               Ready,
		Status:             readyStat,
		Reason:             readyReason,
		Message:            readyMessage,
		ObservedGeneration: k.GetGeneration(),
	}
	meta.SetStatusCondition(&k.Status.Conditions, readyCondition)

	if readyCondition.Status == metav1.ConditionTrue || isPermanentFailure(readyCondition.Reason) {
		k.Status.ObservedGeneration = k.GetGeneration()
	}
}

var failureReasons []string = []string{
	ReasonSecretNameConflict,
}

func isPermanentFailure(reason string) bool {
	return slices.Contains(failureReasons, reason)
}
