package garage

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/bmarinov/garage-storage-controller/internal/garage/integrationtests"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

var garageEnv integrationtests.Environment

func TestMain(m *testing.M) {
	garageEnv = integrationtests.NewGarageEnv()
	defer garageEnv.Terminate(context.Background())

	m.Run()
}

func TestBucketClient(t *testing.T) {
	apiV2, _ := url.JoinPath(garageEnv.AdminAPIAddr, "/v2")
	apiclient := NewClient(apiV2, garageEnv.APIToken)
	sut := apiclient.BucketClient

	t.Run("Create", func(t *testing.T) {
		t.Run("new bucket", func(t *testing.T) {
			bucketName := "hello"
			bucket, err := sut.Create(t.Context(), bucketName)
			if err != nil {
				t.Fatal(err)
			}
			if bucket.ID == "" {
				t.Error("expected bucket ID to be set")
			}
			if bucket.GlobalAliases[0] != "hello" {
				t.Errorf("unexpected alias: %v", bucket.GlobalAliases)
			}
		})
		t.Run("bucket exists", func(t *testing.T) {
			bucketName := "foobaz3134"
			_, err := sut.Create(t.Context(), bucketName)
			if err != nil {
				t.Fatal(err)
			}

			duplicate, err := sut.Create(t.Context(), bucketName)
			if err == nil {
				t.Fatalf("expected err for duplicate bucket, result: %v", duplicate)
			}
			if !errors.Is(err, s3.ErrBucketExists) {
				t.Errorf("expected %v got %v", s3.ErrBucketExists, err)
			}
			if duplicate.ID != "" {
				t.Errorf("second result should not have valid values: %v", duplicate)
			}
		})
	})
	t.Run("Get", func(t *testing.T) {
		t.Run("retrieve created bucket info", func(t *testing.T) {
			newBucket := "blap313"
			created, err := sut.Create(t.Context(), newBucket)
			if err != nil {
				t.Fatal(err)
			}

			retrieved, err := sut.Get(t.Context(), newBucket)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(created, retrieved) {
				t.Errorf("buckets not equal: created %v retrieved %v", created, retrieved)
			}
		})
		t.Run("bucket does not exist", func(t *testing.T) {
			_, err := sut.Get(t.Context(), "unknown-bucket-name")
			if err == nil {
				t.Fatal("expected error for not found")
			}
			if !errors.Is(err, s3.ErrBucketNotFound) {
				t.Errorf("expected error %v got %v", s3.ErrBucketNotFound, err)
			}
		})
	})
}
