package s3

import "errors"

var (
	ErrBucketExists = errors.New("bucket with alias already exists")

	ErrResourceNotFound = errors.New("resource not found")
)

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

type Permissions struct {
	Owner bool
	Read  bool
	Write bool
}
