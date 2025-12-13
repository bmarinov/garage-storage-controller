package garage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/bmarinov/garage-storage-controller/internal/garage/integrationtests"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/google/uuid"
)

var garageEnv integrationtests.Environment

func TestMain(m *testing.M) {
	garageEnv = integrationtests.NewGarageEnv()
	defer garageEnv.Terminate(context.Background())

	m.Run()
}

func TestBucketClient(t *testing.T) {
	apiclient := NewClient(garageEnv.AdminAPIAddr, garageEnv.APIToken)
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
		t.Run("existing bucket info", func(t *testing.T) {
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
	t.Run("Update", func(t *testing.T) {
		t.Run("new quota", func(t *testing.T) {
			bucket, _ := sut.Create(t.Context(), "update-quotas")

			newQuotas := s3.Quotas{
				MaxObjects: 9500,
				MaxSize:    350,
			}

			err := sut.Update(t.Context(), bucket.ID, newQuotas)
			if err != nil {
				t.Fatal(err)
			}

			retrieved, err := sut.Get(t.Context(), bucket.GlobalAliases[0])
			if err != nil {
				t.Fatal(err)
			}
			if retrieved.Quotas.MaxSize != newQuotas.MaxSize ||
				retrieved.Quotas.MaxObjects != newQuotas.MaxObjects {
				t.Errorf("quotas dont match: expected %v got %v", newQuotas, retrieved.Quotas)
			}
		})
		t.Run("unknown bucket ID", func(t *testing.T) {
			unknownID := integrationtests.GenerateRandomString(hex.EncodeToString)
			err := sut.Update(t.Context(), unknownID, s3.Quotas{MaxObjects: 3})
			if err == nil || !errors.Is(err, s3.ErrBucketNotFound) {
				t.Fatalf("expected error %v got %v", s3.ErrBucketNotFound, err)
			}
		})
	})
}

func TestAccessKeyClient(t *testing.T) {
	apiclient := NewClient(garageEnv.AdminAPIAddr, garageEnv.APIToken)
	sut := apiclient.AccessKeyClient

	t.Run("Create", func(t *testing.T) {
		t.Run("new key", func(t *testing.T) {
			key, err := sut.Create(t.Context(), "somename")
			if err != nil {
				t.Fatal(err)
			}
			if key.ID == "" || key.Name != "somename" || key.Secret == "" {
				t.Errorf("unexpected value in key fields: %v", key)
			}
		})
		t.Run("two keys with the same name", func(t *testing.T) {
			keyName := "testfoo312"

			first, err := sut.Create(t.Context(), keyName)
			if err != nil {
				t.Fatal(err)
			}

			second, err := sut.Create(t.Context(), keyName)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID == second.ID || first.Secret == second.Secret {
				t.Error("keys should not share ID or secret")
			}
		})
		t.Run("with naming convention", func(t *testing.T) {
			uid := uuid.New()
			hash := sha256.Sum256(uid[:])
			suffix := hash[:4]
			keyName := fmt.Sprintf("namespacefoo-keybar-%x", suffix)
			key, err := sut.Create(t.Context(), keyName)
			if err != nil {
				t.Fatal(err)
			}
			if key.Name != keyName {
				t.Errorf("expected %s got %s", keyName, key.Name)
			}
		})
	})
	t.Run("Get", func(t *testing.T) {
		name := "foo-canretrieve-key-with-id"
		created, err := sut.Create(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("retrieve by id", func(t *testing.T) {
			retrieved, err := sut.Get(t.Context(), created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if created.ID != retrieved.ID {
				t.Errorf("expected id %s got %s", created.ID, retrieved.ID)
			}
			if retrieved.Secret == "" {
				t.Error("should always return secret")
			}
		})

		t.Run("ID not found", func(t *testing.T) {
			_, err := sut.Get(t.Context(), "foo123-unknown-key")
			if err == nil || !errors.Is(err, s3.ErrKeyNotFound) {
				t.Errorf("expected error %v got %v", s3.ErrKeyNotFound, err)
			}
		})
	})
	t.Run("Lookup", func(t *testing.T) {
		name := "bar-canretrieve-key-bazz-by-name"
		created, _ := sut.Create(t.Context(), name)
		t.Run("retrieve existing by name match", func(t *testing.T) {
			retrieved, err := sut.Lookup(t.Context(), name)
			if err != nil {
				t.Fatal(err)
			}
			if created.ID != retrieved.ID {
				t.Errorf("expected ID %s got %s", created.ID, retrieved.ID)
			}
			if retrieved.Secret == "" {
				t.Error("should always return secret")
			}
		})
	})
}
