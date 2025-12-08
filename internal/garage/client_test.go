package garage

import (
	"context"
	"net/url"
	"testing"

	"github.com/bmarinov/garage-storage-controller/internal/garage/integrationtests"
)

var garageEnv integrationtests.Environment

func TestMain(m *testing.M) {
	garageEnv = integrationtests.NewGarageEnv()
	defer garageEnv.Terminate(context.Background())

	m.Run()
}

func TestBucketClient(t *testing.T) {
	apiPath, _ := url.JoinPath(garageEnv.AdminAPIAddr, "/v2")
	token := garageEnv.APIToken

	t.Run("Create", func(t *testing.T) {
		apiclient := NewClient(apiPath, token)
		sut := apiclient.BucketClient

		t.Run("new bucket", func(t *testing.T) {
			bucket, err := sut.Create(t.Context(), "hello")
			if err != nil {
				t.Error(err)
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
				t.Errorf("expected err for duplicate bucket, result: %v", duplicate)
			}
			if duplicate.ID != "" {
				t.Errorf("second result should not have valid values: %v", duplicate)
			}
		})
	})
}
