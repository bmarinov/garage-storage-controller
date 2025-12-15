/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

type PermissionClient interface {
	// SetPermissions ensures that an access key has the exact r/w/owner permissions on a bucket
	SetPermissions(ctx context.Context, keyID, bucketID string, permissions s3.Permissions) error
}

// AccessPolicyReconciler reconciles a AccessPolicy object
type AccessPolicyReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewAccessPolicyReconciler(c client.Client, scheme *runtime.Scheme) *AccessPolicyReconciler {
	return &AccessPolicyReconciler{
		client: c,
		scheme: scheme,
	}
}

// errDependencyNotReady should resolve itself given enough time and recon can be retried.
var errDependencyNotReady = errors.New("resource dependency is not ready")

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/finalizers,verbs=update

func (r *AccessPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var policy garagev1alpha1.AccessPolicy
	err := r.client.Get(ctx, req.NamespacedName, &policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Reconciling AccessPolicy", "name", req.NamespacedName)

	orig := policy.Status.DeepCopy()
	initializePolicyConditions(&policy)

	err = r.reconcilePolicy(ctx, &policy)

	updateAccessPolicyCondition(&policy)

	var result ctrl.Result
	var resultErr error

	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			result = ctrl.Result{RequeueAfter: 15 * time.Second}
		case errors.Is(err, errDependencyNotReady):
			result = ctrl.Result{RequeueAfter: 10 * time.Second}
		default:
			resultErr = err
		}
	}

	// TODO: patch status, merge vs apply?
	if !equality.Semantic.DeepEqual(*orig, policy.Status) {
		err = r.client.Status().Patch(ctx, &policy, client.Merge, client.FieldOwner(bucketControllerName))
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return result, resultErr
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessPolicy{}).
		Named("accesspolicy").
		Complete(r)
}

func (r *AccessPolicyReconciler) reconcilePolicy(ctx context.Context, policy *garagev1alpha1.AccessPolicy) error {
	var bucket garagev1alpha1.Bucket
	err := r.client.Get(ctx,
		types.NamespacedName{Namespace: policy.Namespace, Name: policy.Spec.Bucket},
		&bucket)

	if err != nil {
		if apierrors.IsNotFound(err) {
			markNotReady(
				policy,
				&policy.Status.Conditions,
				PolicyBucketReady,
				ReasonBucketMissing,
				"Bucket not found in namespace: %v", err)
		}
		return fmt.Errorf("retrieve bucket %s: %w", policy.Spec.Bucket, err)
	}

	bucketCond := meta.FindStatusCondition(bucket.Status.Conditions, Ready)
	if bucketCond.Status != metav1.ConditionTrue {
		markPolicyConditionNotReady(policy, PolicyBucketReady, ReasonDependencyNotReady, "%s", bucketCond.Message)
		return errDependencyNotReady
	}

	var accessKey garagev1alpha1.AccessKey
	err = r.client.Get(ctx,
		types.NamespacedName{Namespace: policy.Namespace, Name: policy.Spec.AccessKey},
		&accessKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			markNotReady(policy,
				&policy.Status.Conditions,
				PolicyAccessKeyReady,
				ReasonAccessKeyMissing,
				"Access key not found in namespace: %v", err)
		}
		return fmt.Errorf("retrieve access key %s: %w", policy.Spec.AccessKey, err)
	}
	accessKeyCond := meta.FindStatusCondition(accessKey.Status.Conditions, Ready)
	if accessKeyCond.Status != metav1.ConditionTrue {
		markPolicyConditionNotReady(policy, PolicyAccessKeyReady, ReasonDependencyNotReady, "%s", accessKeyCond.Message)
		return errDependencyNotReady
		// 	policy.NotReady("reason key not ready")
	}

	// Happy path from here:

	return nil
}
