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
	"log/slog"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	bucketControllerName = "garage-storage-controller"
)

type S3Client interface {
	Create(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Get(ctx context.Context, globalAlias string) (s3.Bucket, error)
	Update(ctx context.Context, id string, quotas s3.Quotas) error
}

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	s3     S3Client
}

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=buckets/finalizers,verbs=update

func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	bucket := garagev1alpha1.Bucket{}

	err := r.Get(ctx, req.NamespacedName, &bucket)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		slog.Error("get Bucket", "err", err)
		return ctrl.Result{}, err
	}

	orig := bucket.Status.DeepCopy()

	bucketMgr := Bucket{Object: &bucket}
	bucketMgr.InitializeConditions()

	err = r.reconcileBucket(ctx, &bucketMgr)

	if err != nil {
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepEqual(*orig, bucket.Status) {
		err = r.Status().Patch(ctx, &bucket, client.Merge, client.FieldOwner(bucketControllerName))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.Bucket{}).
		Named("bucket").
		Complete(r)
}
