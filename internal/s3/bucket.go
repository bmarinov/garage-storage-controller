package s3

import "errors"

var ErrBucketNotFound = errors.New("bucket not found")

type Bucket struct {
	ID string

	GlobalAliases []string

	Quotas Quotas

	// Keys
}

type Quotas struct {
	MaxObjects int64
	MaxSize    int64
}

type AccessKey struct {
	ID     string
	Secret string
	Name   string
}
