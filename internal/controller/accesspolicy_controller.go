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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

type PermissionClient interface {
	// SetPermissions ensures that an access key has the target r/w/owner permissions on a bucket.
	SetPermissions(ctx context.Context, keyID, bucketID string, permissions s3.Permissions) error
}

// AccessPolicyReconciler reconciles a AccessPolicy object
type AccessPolicyReconciler struct {
	client      client.Client
	scheme      *runtime.Scheme
	adminClient PermissionClient
}

func NewAccessPolicyReconciler(c client.Client,
	scheme *runtime.Scheme,
	ac PermissionClient,
) *AccessPolicyReconciler {
	return &AccessPolicyReconciler{
		client:      c,
		scheme:      scheme,
		adminClient: ac,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessPolicy{}).
		Watches(&garagev1alpha1.AccessKey{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForAccessKey)).
		Named("accesspolicy").
		Complete(r)
}

// errDependencyNotReady should resolve itself given enough time and recon can be retried.
var errDependencyNotReady = errors.New("resource dependency is not ready")

// accesskeyLabel on the AccessPolicy resource.
const accesskeyLabel = "garage.getclustered.net/accesskey-name"

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/finalizers,verbs=update

func (r *AccessPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy garagev1alpha1.AccessPolicy
	err := r.client.Get(ctx, req.NamespacedName, &policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	metaChanged := r.ensureLabels(&policy)

	if policy.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&policy, finalizerName) {
			modified := controllerutil.AddFinalizer(&policy, finalizerName)
			if modified {
				metaChanged = true
			}
		}
	} else {
		if !controllerutil.ContainsFinalizer(&policy, finalizerName) {
			return ctrl.Result{}, nil
		}
		err = r.adminClient.SetPermissions(ctx,
			policy.Status.AccessKeyID,
			policy.Status.BucketID,
			s3.Permissions{})
		if err != nil {
			if errors.Is(err, s3.ErrResourceNotFound) {
				logger.Info("AccessPolicy finalizer: key or bucket already deleted")
			} else {
				log.FromContext(ctx).Error(err, "Removing permissions in finalizer failed")
				return ctrl.Result{}, err
			}
		}

		_ = controllerutil.RemoveFinalizer(&policy, finalizerName)
		err = r.client.Update(ctx, &policy)
		return ctrl.Result{}, err
	}

	if metaChanged {
		err = r.client.Update(ctx, &policy)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating object meta: %w", err)
		}
		// let the next reconciliation loop handle this update:
		return reconcile.Result{}, nil
	}

	logger.V(1).Info("Reconciling AccessPolicy", "name", req.NamespacedName)

	oldStatus := policy.Status.DeepCopy()
	initializePolicyConditions(&policy)
	err = r.reconcilePolicy(ctx, &policy)

	updateAccessPolicyCondition(&policy)

	var result ctrl.Result
	var resultErr error = nil

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

	if !equality.Semantic.DeepEqual(*oldStatus, policy.Status) {
		err = r.client.Status().Patch(ctx, &policy, client.Merge, client.FieldOwner(bucketControllerName))
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return result, resultErr
}

func (r *AccessPolicyReconciler) findPoliciesForAccessKey(ctx context.Context, obj client.Object) []reconcile.Request {
	accessKey := obj.(*garagev1alpha1.AccessKey)
	var policies garagev1alpha1.AccessPolicyList
	err := r.client.List(ctx, &policies,
		client.InNamespace(accessKey.Namespace),
		client.MatchingLabels{accesskeyLabel: accessKey.Name},
	)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(policies.Items))
	for i, policy := range policies.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      policy.Name,
				Namespace: policy.Namespace,
			},
		}
	}

	return requests
}

func (r *AccessPolicyReconciler) ensureLabels(policy *garagev1alpha1.AccessPolicy) bool {
	if policy.Labels == nil {
		policy.Labels = make(map[string]string)
	}
	if policy.Labels[accesskeyLabel] != policy.Spec.AccessKey {
		policy.Labels[accesskeyLabel] = policy.Spec.AccessKey
		return true
	}
	return false
}

func (r *AccessPolicyReconciler) reconcilePolicy(ctx context.Context, policy *garagev1alpha1.AccessPolicy) error {
	var errs []error

	bucket, err := r.checkBucket(ctx, policy)
	if err != nil {
		errs = append(errs, err)
	}

	accessKey, err := r.checkAccessKey(ctx, policy)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	err = r.adminClient.SetPermissions(ctx,
		accessKey.Status.AccessKeyID,
		bucket.Status.BucketID,
		s3.Permissions{
			Read:  policy.Spec.Permissions.Read,
			Write: policy.Spec.Permissions.Write,
			Owner: policy.Spec.Permissions.Owner,
		})
	if err != nil {
		return err
	}
	markPolicyAssignmentReady(policy)

	return nil
}

func (r *AccessPolicyReconciler) checkBucket(ctx context.Context,
	policy *garagev1alpha1.AccessPolicy) (*garagev1alpha1.Bucket, error) {
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
			return nil, fmt.Errorf("retrieve bucket %s: %w", policy.Spec.Bucket, err)
		} else {
			return nil, fmt.Errorf("unexpected error getting bucket: %w", err)
		}
	} else {
		policy.Status.BucketID = bucket.Status.BucketID
		bucketCond := meta.FindStatusCondition(bucket.Status.Conditions, Ready)
		if bucketCond == nil || bucketCond.Status != metav1.ConditionTrue {
			message := "Bucket condition not met"
			if bucketCond != nil {
				message = bucketCond.Message
			}
			markPolicyConditionNotReady(policy, PolicyBucketReady, ReasonDependenciesNotReady, "%s", message)
			return nil, fmt.Errorf("bucket not ready: %w", errDependencyNotReady)
		} else {
			markPolicyBucketReady(policy)
		}
	}

	return &bucket, nil
}

func (r *AccessPolicyReconciler) checkAccessKey(
	ctx context.Context,
	policy *garagev1alpha1.AccessPolicy,
) (*garagev1alpha1.AccessKey, error) {
	var accessKey garagev1alpha1.AccessKey
	err := r.client.Get(ctx,
		types.NamespacedName{Namespace: policy.Namespace, Name: policy.Spec.AccessKey},
		&accessKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			markNotReady(policy,
				&policy.Status.Conditions,
				PolicyAccessKeyReady,
				ReasonAccessKeyMissing,
				"Access key not found in namespace: %v", err)
			return nil, fmt.Errorf("retrieve access key %s: %w", policy.Spec.AccessKey, err)
		} else {
			return nil, fmt.Errorf("unexpected error gettig access key: %w", err)
		}
	} else {
		policy.Status.AccessKeyID = accessKey.Status.AccessKeyID
		accessKeyCond := meta.FindStatusCondition(accessKey.Status.Conditions, Ready)
		if accessKeyCond == nil || accessKeyCond.Status != metav1.ConditionTrue {
			message := "AccessKey condition not met"
			if accessKeyCond != nil {
				message = accessKeyCond.Message
			}
			markPolicyConditionNotReady(policy, PolicyAccessKeyReady, ReasonDependenciesNotReady, "%s", message)
			return nil, fmt.Errorf("access key not ready: %w", errDependencyNotReady)
		} else {
			markPolicyKeyReady(policy)
		}
	}
	return &accessKey, nil
}
