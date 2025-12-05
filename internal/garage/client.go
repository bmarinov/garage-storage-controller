package garage

import (
	"context"

	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

type AdminClient struct {
	BucketClient
}

type BucketClient struct {
}

func (a *BucketClient) Create(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	panic("unimplemented")
}

func (a *BucketClient) Get(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	panic("unimplemented")
}

func (a *BucketClient) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	panic("unimplemented")
}
