package controller

import (
	"context"
	"errors"
	"fmt"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

func (r *BucketReconciler) reconcileBucket(ctx context.Context, bucket *garagev1alpha1.Bucket) error {
	alias := bucket.Spec.Name
	s3Bucket, err := r.bucket.Get(ctx, alias)
	if err != nil {
		if errors.Is(err, s3.ErrBucketNotFound) {
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

	diff := compareSpec(s3Bucket, bucket.Spec)

	if diff {
		err := r.bucket.Update(ctx, s3Bucket.ID, s3.Quotas{
			MaxObjects: bucket.Spec.MaxObjects,
			MaxSize:    bucket.Spec.MaxSize,
		})

		if err != nil {
			markBucketNotReady(bucket,
				"Update Failed",
				"Failed to update bucket configuration: %v", err)
			return fmt.Errorf("updating external bucket to spec: %w", err)
		}
	}
	markBucketReady(bucket)
	return nil
}

func compareSpec(bucket s3.Bucket, spec garagev1alpha1.BucketSpec) bool {
	if bucket.Quotas.MaxObjects != spec.MaxObjects ||
		bucket.Quotas.MaxSize != spec.MaxSize {
		return true
	}

	return false
}
