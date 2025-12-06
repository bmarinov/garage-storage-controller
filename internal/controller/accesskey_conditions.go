package controller

import (
	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccessKey struct {
	Object *garagev1alpha1.AccessKey
}

const (
	AccessKeyReady string = "AccessKeyReady"
	KeySecretReady string = "KeySecretReady"
)

func (k *AccessKey) InitializeConditions() {
	conditions := []string{Ready, AccessKeyReady, KeySecretReady}
	initResourceConditions(conditions, &k.Object.Status.Conditions)
}

func (k *AccessKey) MarkAccessKeyReady() {
	cond := metav1.Condition{
		Type:               AccessKeyReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: k.Object.GetGeneration(),
		Reason:             "AccessKeyReady",
		Message:            "External access key is ready",
	}
	meta.SetStatusCondition(&k.Object.Status.Conditions, cond)
}

func (k *AccessKey) MarkSecretReady() {
	cond := metav1.Condition{
		Type:               KeySecretReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: k.Object.GetGeneration(),
		Reason:             "SecretReady",
		Message:            "Secret for external access key is ready",
	}
	meta.SetStatusCondition(&k.Object.Status.Conditions, cond)
}

func (k *AccessKey) AccessKeyCondition() metav1.Condition {
	cond := meta.FindStatusCondition(k.Object.Status.Conditions, AccessKeyReady)
	return *cond
}

func (k *AccessKey) updateStatus() {
	// accessKeyCond := meta.FindStatusCondition(k.Object.Status.Conditions, AccessKeyReady)

	k.Object.Status.ObservedGeneration = k.Object.GetGeneration()
}
