package controller

import (
	"context"
	"errors"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

func (r *BucketReconciler) reconcileBucket(ctx context.Context, bucket *Bucket) error {
	alias := bucket.Object.Spec.Name
	s3Bucket, err := r.s3.Get(ctx, alias)
	if err != nil {
		if errors.Is(err, s3.ErrBucketNotFound) {
			// create
			bucket.MarkBucketNotReady("BucketNotFound", "Bucket with alias %s not found", alias)
		} else {
			bucket.MarkBucketNotReady("UnknownState", "S3 API error")
			return err
		}
	}

	diff := compare(s3Bucket, *bucket.Object)

	if diff.needUpdate {
		bucket.MarkBucketNotReady("BucketDrift", "Progressing..")

		// return foo.patch()
	} else {
		bucket.MarkBucketReady()
	}

	return nil
}

func compare(existing s3.Bucket, desired garagev1alpha1.Bucket) bucketDiff {

	return bucketDiff{}
}

// TBD
type bucketDiff struct {
	needUpdate bool
}
