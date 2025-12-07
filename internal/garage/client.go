package garage

import (
	"context"

	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/google/uuid"
)

type AdminClient struct {
	*BucketClient
	*AccessKeyClient
}

type AccessKeyClient struct {
}

func (a *AccessKeyClient) Create(ctx context.Context, keyName string) (s3.AccessKey, error) {
	return s3.AccessKey{
		ID:     uuid.NewString(),
		Secret: uuid.NewString(),
		Name:   keyName,
	}, nil
}

func (a *AccessKeyClient) Get(ctx context.Context, id string, search string) (s3.AccessKey, error) {
	panic("unimplemented")
}

type BucketClient struct {
}

func (a *BucketClient) Create(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	return s3.Bucket{
		ID:            uuid.NewString(),
		GlobalAliases: []string{globalAlias},
	}, nil
}

func (a *BucketClient) Get(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	return s3.Bucket{
		ID:            uuid.NewString(),
		GlobalAliases: []string{globalAlias},
	}, nil
}

func (a *BucketClient) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	return nil
}
