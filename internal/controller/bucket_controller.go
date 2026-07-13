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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

const bucketControllerName = "garage-storage-controller"

const (
	configMapKeyBucketName = "bucket-name"
	configMapKeyEndpoint   = "s3-endpoint"
	labelBucketName        = "garage.getclustered.net/bucket"
)

type BucketClient interface {
	Create(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Get(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Update(ctx context.Context, id string, quotas s3.Quotas) error
}

type OwnershipVerifier interface {
	GetPermissions(ctx context.Context, keyID, bucketID string) (s3.Permissions, error)
}

const maxRequeueInterval = 15 * time.Minute

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client        client.Client
	Scheme        *runtime.Scheme
	bucket        BucketClient
	s3APIEndpoint string
	ownership     OwnershipVerifier
	recorder      record.EventRecorder
	// baseRequeueInterval to override in tests. Base delay for the exponential backoff.
	baseRequeueInterval time.Duration
}

func NewBucketReconciler(
	apiClient client.Client,
	scheme *runtime.Scheme,
	s3Client BucketClient,
	s3APIEndpoint string,
	ownershipVerifier OwnershipVerifier,
	recorder record.EventRecorder,
) *BucketReconciler {
	return &BucketReconciler{
		client:              apiClient,
		Scheme:              scheme,
		bucket:              s3Client,
		s3APIEndpoint:       s3APIEndpoint,
		ownership:           ownershipVerifier,
		recorder:            recorder,
		baseRequeueInterval: 5 * time.Second,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.Bucket{}).
		Named("bucket").
		Complete(r)
}

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bucket := garagev1alpha1.Bucket{}

	err := r.client.Get(ctx, req.NamespacedName, &bucket)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	orig := bucket.Status.DeepCopy()
	initializeBucketConditions(&bucket)

	result, err := r.reconcileBucket(ctx, &bucket)
	updateBucketReadyCondition(&bucket)

	if !equality.Semantic.DeepEqual(*orig, bucket.Status) {
		patchErr := r.client.Status().Patch(ctx, &bucket, client.Merge, client.FieldOwner(bucketControllerName))

		if patchErr != nil {
			if apierrors.IsConflict(patchErr) {
				return ctrl.Result{}, nil
			}
			if err == nil {
				return ctrl.Result{}, patchErr
			}
		}
	}

	return result, err
}

func (r *BucketReconciler) reconcileBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) (ctrl.Result, error) {
	var (
		s3Bucket s3.Bucket
		alias    string
		err      error
	)
	if bucket.Spec.ExistingBucket != nil {
		s3Bucket, alias, err = r.resolveExistingBucket(ctx, bucket)
		if err != nil {
			// TODO: distinguish between:
			// - namespace Secret not found (base, exponential backoff)
			// - no ownership (max interval?)
			// - Garage key not found (key deleted, skip backoff)
			return ctrl.Result{
				RequeueAfter: ownerSecretBackoff(time.Since(bucket.CreationTimestamp.Time), r.baseRequeueInterval, maxRequeueInterval),
			}, nil
		}
	} else {
		if bucket.Spec.NameOverride != "" {
			s3Bucket, alias, err = r.resolveNewNameOverrideBucket(ctx, bucket)
		} else {
			s3Bucket, alias, err = r.resolveNewBucket(ctx, bucket)
		}
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if bucket.Status.BucketID == "" {
		bucket.Status.BucketID = s3Bucket.ID
	}
	if bucket.Status.BucketName == "" {
		bucket.Status.BucketName = alias
	}

	diff := compareSpec(s3Bucket, bucket.Spec)

	if diff {
		err := r.bucket.Update(ctx, s3Bucket.ID, s3.Quotas{
			MaxObjects: bucket.Spec.MaxObjects,
			MaxSize:    bucket.Spec.MaxSize.Value(),
		})

		if err != nil {
			markBucketNotReady(bucket,
				"Update Failed",
				"Failed to update bucket configuration: %v", err)
			return ctrl.Result{}, fmt.Errorf("updating external bucket to spec: %w", err)
		}
	}
	markBucketReady(bucket)

	err = r.ensureConfigMap(ctx, bucket, r.s3APIEndpoint)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to create ConfigMap for Bucket")

		if errors.Is(err, errNameConflict) {
			updateBucketCMCondition(bucket, metav1.ConditionFalse,
				ReasonConfigMapNameConflict,
				"Unable to use existing ConfigMap for bucket details: %v",
				err,
			)
			return ctrl.Result{}, nil
		}
		updateBucketCMCondition(bucket, metav1.ConditionFalse,
			"ConfigMapCreateError",
			"Unable to create ConfigMap: %v",
			err,
		)
		return ctrl.Result{}, err
	}
	updateBucketCMCondition(bucket, metav1.ConditionTrue,
		"ConfigMapReady", "ConfigMap with external bucket details is ready")

	return ctrl.Result{}, r.deleteStaleConfigMap(ctx, *bucket)
}

// ensureConfigMap creates a new configmap for the bucket or updates the values in an existing one.
func (r *BucketReconciler) ensureConfigMap(ctx context.Context,
	bucket *garagev1alpha1.Bucket,
	endpoint string,
) error {
	var cmName string
	if bucket.Spec.ConfigMapName == "" {
		cmName = bucket.Name
	} else {
		cmName = bucket.Spec.ConfigMapName
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: bucket.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.client, &cm, func() error {
		if cm.UID != "" {
			// configmap already exists, check owner:
			if !metav1.IsControlledBy(&cm, bucket) {
				return fmt.Errorf(
					"conflict for ConfigMap %s/%s: resource already exists and is not owned by Bucket %s: %w",
					cm.Namespace, cm.Name, bucket.Name, errNameConflict)
			}
		}

		err := controllerutil.SetControllerReference(bucket, &cm, r.Scheme)
		if err != nil {
			return err
		}
		if cm.Labels == nil {
			cm.Labels = make(map[string]string)
		}
		cm.Labels[labelBucketName] = bucket.Name
		cm.Data = map[string]string{
			configMapKeyBucketName: bucket.Status.BucketName,
			configMapKeyEndpoint:   endpoint,
		}
		return nil
	})

	if opResult == controllerutil.OperationResultCreated {
		log.FromContext(ctx).Info("ConfigMap for Bucket created",
			"namespace", bucket.Namespace, "bucket", bucket.Name, "name", cm.Name)
	}

	return err
}

func (r *BucketReconciler) deleteStaleConfigMap(ctx context.Context, bucket garagev1alpha1.Bucket) error {
	var configMaps corev1.ConfigMapList
	err := r.client.List(ctx, &configMaps,
		client.InNamespace(bucket.Namespace),
		client.MatchingLabels{
			labelBucketName: bucket.Name,
		})
	if err != nil {
		return fmt.Errorf("retrieving ConfigMaps by Bucket label: %w", err)
	}

	var desiredName string
	if bucket.Spec.ConfigMapName != "" {
		desiredName = bucket.Spec.ConfigMapName
	} else {
		desiredName = bucket.Name
	}
	for _, cm := range configMaps.Items {
		if cm.Name != desiredName && metav1.IsControlledBy(&cm, &bucket) {
			err = r.client.Delete(ctx, &cm)
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting stale configmap %s: %w", cm.Name, err)
			}
		}
	}
	return nil
}

func compareSpec(bucket s3.Bucket, spec garagev1alpha1.BucketSpec) bool {
	if bucket.Quotas.MaxObjects != spec.MaxObjects ||
		bucket.Quotas.MaxSize != spec.MaxSize.Value() {
		return true
	}

	return false
}

func (r *BucketReconciler) resolveNewBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) (s3.Bucket, string, error) {
	alias := suffixedResourceName(bucket.Spec.Name, bucket.ObjectMeta)
	s3Bucket, err := r.bucket.Get(ctx, alias)
	if err != nil {
		if errors.Is(err, s3.ErrResourceNotFound) {
			s3Bucket, err = r.bucket.Create(ctx, alias)
			if err != nil {
				markBucketNotReady(bucket, "CreateFailed", "Failed to create bucket '%s': %v", alias, err)
				r.recorder.Eventf(bucket, corev1.EventTypeWarning, ReasonBucketCreateFailed, "Failed to create Garage bucket %q: %v", alias, err)
				return s3.Bucket{}, "", fmt.Errorf("create new bucket: %w", err)
			}
			r.recorder.Eventf(bucket, corev1.EventTypeNormal, ReasonBucketCreated, "Created Garage bucket %q", alias)
		} else {
			markBucketNotReady(bucket, "UnknownState", "S3 API error: %v", err)
			return s3.Bucket{}, "", fmt.Errorf("retrieving existing bucket: %w", err)
		}
	}
	return s3Bucket, alias, nil
}

func (r *BucketReconciler) resolveNewNameOverrideBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) (s3.Bucket, string, error) {
	alias := bucket.Spec.NameOverride
	s3Bucket, err := r.bucket.Get(ctx, alias)
	if err != nil {
		if !errors.Is(err, s3.ErrResourceNotFound) {
			markBucketNotReady(bucket, "UnknownState", "S3 API error: %v", err)
			return s3.Bucket{}, "", fmt.Errorf("retrieving existing bucket: %w", err)
		}

		if bucket.Status.BucketID != "" {
			markBucketNotReady(bucket, "BucketMissing", "Bucket %q is missing: it has been deleted and will not be recreated", alias)
			r.recorder.Eventf(bucket, corev1.EventTypeWarning, ReasonBucketMissing, "Bucket %q is missing: it has been deleted and will not be recreated", alias)
			return s3.Bucket{}, "", fmt.Errorf("bucket %q is missing: it has been deleted and will not be recreated", alias)
		}

		s3Bucket, err = r.bucket.Create(ctx, alias)
		if err != nil {
			markBucketNotReady(bucket, "CreateFailed", "failed to create bucket '%s': %v", alias, err)
			r.recorder.Eventf(bucket, corev1.EventTypeWarning, ReasonBucketCreateFailed, "failed to create Garage bucket %q: %v", alias, err)
			return s3.Bucket{}, "", fmt.Errorf("create new bucket: %w", err)
		}

		r.recorder.Eventf(bucket, corev1.EventTypeNormal, ReasonBucketCreated, "Created Garage bucket %q", alias)
		return s3Bucket, alias, nil
	}

	if bucket.Status.BucketID == "" {
		markBucketNotReady(bucket, "CreateFailed", "Failed to create bucket %q: bucket already exists. Use spec.existingBucket to adopt it", alias)
		r.recorder.Eventf(bucket, corev1.EventTypeWarning, ReasonBucketCreateFailed, "Failed to create bucket %q: bucket already exists. Use spec.existingBucket to adopt it", alias)
		return s3.Bucket{}, "", fmt.Errorf("failed to create bucket %q: bucket already exists. Use spec.existingBucket to adopt it", alias)
	}

	if bucket.Status.BucketID != s3Bucket.ID {
		markBucketNotReady(bucket, "BucketMissing", "Bucket %q is missing: its bucket ID changed and it will not be adopted", alias)
		r.recorder.Eventf(bucket, corev1.EventTypeWarning, ReasonBucketMissing, "Bucket %q is missing: its bucket ID changed and it will not be adopted", alias)
		return s3.Bucket{}, "", fmt.Errorf("bucket %q is missing: its bucket ID changed and it will not be adopted", alias)
	}

	return s3Bucket, alias, nil
}

func (r *BucketReconciler) resolveExistingBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) (s3.Bucket, string, error) {
	spec := bucket.Spec.ExistingBucket

	var secret corev1.Secret
	err := r.client.Get(ctx, client.ObjectKey{Namespace: bucket.Namespace, Name: spec.OwnerKeySecret}, &secret)
	if err != nil {
		markBucketNotReady(bucket, ReasonOwnerKeySecretNotFound, "Secret %q not found: %v", spec.OwnerKeySecret, err)
		return s3.Bucket{}, "", fmt.Errorf("get owner key secret: %w", err)
	}
	keyID := string(secret.Data[SecretKeyAccessKeyID])

	s3Bucket, err := r.bucket.Get(ctx, spec.Name)
	if err != nil {
		markBucketNotReady(bucket, "BucketNotFound", "Bucket %q not found in Garage: %v", spec.Name, err)
		return s3.Bucket{}, "", fmt.Errorf("get existing bucket: %w", err)
	}

	perms, err := r.ownership.GetPermissions(ctx, keyID, s3Bucket.ID)
	if err != nil {
		markBucketNotReady(bucket, ReasonOwnershipVerificationFailed, "Failed to verify ownership: %v", err)
		return s3.Bucket{}, "", fmt.Errorf("verify ownership: %w", err)
	}
	if !perms.Owner {
		markBucketNotReady(
			bucket,
			ReasonOwnershipVerificationFailed,
			"Key %q does not have owner permission on bucket %q", keyID, spec.Name,
		)
		return s3.Bucket{}, "", fmt.Errorf("ownership check failed: key does not have owner permission")
	}

	return s3Bucket, spec.Name, nil
}

// suffixedResourceName extends a name with a suffix based on the resource UID.
func suffixedResourceName(name string, meta metav1.ObjectMeta) string {
	hash := sha256.Sum256([]byte(meta.UID))
	return fmt.Sprintf("%s-%x", name, hash[:8])
}
