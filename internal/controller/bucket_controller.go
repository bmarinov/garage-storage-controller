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
	"crypto/sha256"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

const (
	bucketControllerName = "garage-storage-controller"
)

const (
	ConfigMapKeyBucketName = "bucket-name"
	ConfigMapKeyEndpoint   = "s3-endpoint"
)

type BucketClient interface {
	Create(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Get(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Update(ctx context.Context, id string, quotas s3.Quotas) error
}

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	bucket        BucketClient
	s3APIEndpoint string
}

func NewBucketReconciler(
	apiClient client.Client,
	scheme *runtime.Scheme,
	s3Client BucketClient,
	s3APIEndpoint string,
) *BucketReconciler {
	return &BucketReconciler{
		Client:        apiClient,
		Scheme:        scheme,
		bucket:        s3Client,
		s3APIEndpoint: s3APIEndpoint,
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

func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bucket := garagev1alpha1.Bucket{}

	err := r.Get(ctx, req.NamespacedName, &bucket)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	orig := bucket.Status.DeepCopy()
	initializeBucketConditions(&bucket)

	err = r.reconcileBucket(ctx, &bucket)

	if err != nil {
		return ctrl.Result{}, err
	}

	updateBucketReadyCondition(&bucket)

	if !equality.Semantic.DeepEqual(*orig, bucket.Status) {
		err = r.Status().Patch(ctx, &bucket, client.Merge, client.FieldOwner(bucketControllerName))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *BucketReconciler) reconcileBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) error {
	alias := suffixedResourceName(bucket.ObjectMeta)
	s3Bucket, err := r.bucket.Get(ctx, alias)
	if err != nil {
		if errors.Is(err, s3.ErrResourceNotFound) {
			s3Bucket, err = r.bucket.Create(ctx, alias)
			if err != nil {
				markBucketNotReady(
					bucket,
					"CreateFailed",
					"Failed to create bucket '%s': %v", alias, err)
				return fmt.Errorf("create new bucket: %w", err)
			}
		} else {
			markBucketNotReady(
				bucket,
				"UnknownState",
				"S3 API error: %v", err)
			return err
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
			return fmt.Errorf("updating external bucket to spec: %w", err)
		}
	}

	err = r.createBucketCM(ctx, bucket, r.s3APIEndpoint)
	if err != nil {
		return fmt.Errorf("create configmap for bucket '%s': %w", alias, err)
	}
	markBucketReady(bucket)
	return nil
}

// createBucketCM creates a new configmap or updates the values in an existing one.
func (r *BucketReconciler) createBucketCM(ctx context.Context, bucket *garagev1alpha1.Bucket, endpoint string) error {
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bucket.Name,
			Namespace: bucket.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &cm, func() error {
		err := controllerutil.SetControllerReference(bucket, &cm, r.Scheme)
		if err != nil {
			return err
		}
		cm.Data = map[string]string{
			ConfigMapKeyBucketName: bucket.Status.BucketName,
			ConfigMapKeyEndpoint:   endpoint,
		}
		return nil
	})

	return err
}

func compareSpec(bucket s3.Bucket, spec garagev1alpha1.BucketSpec) bool {
	if bucket.Quotas.MaxObjects != spec.MaxObjects ||
		bucket.Quotas.MaxSize != spec.MaxSize.Value() {
		return true
	}

	return false
}

// suffixedResourceName adds a suffix based on the resource UID.
func suffixedResourceName(meta metav1.ObjectMeta) string {
	hash := sha256.Sum256([]byte(meta.UID))
	return fmt.Sprintf("%s-%x", meta.Name, hash[:8])
}
