package s3

import "errors"

var ErrBucketNotFound = errors.New("bucket not found")

type Bucket struct {
	ID string

	// Quotas
}
