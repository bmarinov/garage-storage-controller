package controller

import (
	"context"
	"errors"
	"fmt"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

func (r *BucketReconciler) reconcileBucket(ctx context.Context, bucket *Bucket) error {
	alias := bucket.Object.Spec.Name
	s3Bucket, err := r.bucket.Get(ctx, alias)
	if err != nil {
		if errors.Is(err, s3.ErrBucketNotFound) {
			s3Bucket, err = r.bucket.Create(ctx, alias)
			if err != nil {
				bucket.MarkBucketNotReady("CreateFailed", "Failed to create bucket '%s': %v", alias, err)
				return fmt.Errorf("create new bucket: %w", err)
			}
		} else {
			bucket.MarkBucketNotReady("UnknownState", "S3 API error: %v", err)
			return err
		}
	}

	if bucket.Object.Status.BucketID == "" {
		bucket.Object.Status.BucketID = s3Bucket.ID
	}

	diff := compareSpec(s3Bucket, bucket.Object.Spec)

	if diff {
		err := r.bucket.Update(ctx, s3Bucket.ID, s3.Quotas{
			MaxObjects: bucket.Object.Spec.MaxObjects,
			MaxSize:    bucket.Object.Spec.MaxSize,
		})

		if err != nil {
			bucket.MarkBucketNotReady("Update Failed", "Failed to update bucket configuration: %v", err)
			return fmt.Errorf("updating external bucket to spec: %w", err)
		}
	}

	bucket.MarkBucketReady()
	return nil
}

func compareSpec(bucket s3.Bucket, spec garagev1alpha1.BucketSpec) bool {
	if bucket.Quotas.MaxObjects != spec.MaxObjects ||
		bucket.Quotas.MaxSize != spec.MaxSize {
		return true
	}

	return false
}
