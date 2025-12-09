package garage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/google/uuid"
)

type AdminClient struct {
	*BucketClient
	*AccessKeyClient
}

func NewClient(apiAddr string, token string) *AdminClient {
	baseClient := adminAPIHttpClient{
		httpClient: &http.Client{},
		token:      token,
		baseURL:    apiAddr,
	}

	return &AdminClient{
		BucketClient: &BucketClient{
			&baseClient,
		},
	}
}

type adminAPIHttpClient struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

func (c *adminAPIHttpClient) doRequest(ctx context.Context,
	method string,
	path string,
	queryParams *url.Values,
	body io.Reader,
) (*http.Response, error) {
	fullURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("constructing endpoint path: %w", err)
	}

	requestURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if queryParams != nil {
		query := requestURL.Query()
		for k, values := range *queryParams {
			for _, v := range values {
				query.Add(k, v)
			}
		}
		requestURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
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
	*adminAPIHttpClient
}

func (b *BucketClient) Create(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	request := map[string]string{"globalAlias": globalAlias}
	payload, err := json.Marshal(request)
	if err != nil {
		return s3.Bucket{}, err
	}

	resp, err := b.doRequest(ctx, http.MethodPost, "/CreateBucket", nil, bytes.NewBuffer(payload))
	if err != nil {
		return s3.Bucket{}, err
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			return s3.Bucket{}, fmt.Errorf("%w: %w", s3.ErrBucketExists, err)
		}
		return s3.Bucket{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("reading response: %w", err)
	}

	var result Bucket
	err = json.Unmarshal(responseBytes, &result)

	return s3.Bucket{
		ID:            result.ID,
		GlobalAliases: result.GlobalAliases,
		Quotas:        s3.Quotas(result.Quotas),
	}, err
}

func (b *BucketClient) Get(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	params := url.Values{}
	// params.Add("id", bucketID)
	params.Add("globalAlias", globalAlias)

	path := "GetBucketInfo"

	resp, err := b.doRequest(ctx, http.MethodGet, path, &params, nil)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("retrieve bucket '%s': %w", globalAlias, err)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return s3.Bucket{}, fmt.Errorf("%w: %w", s3.ErrBucketNotFound, err)
		}
		return s3.Bucket{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var result Bucket
	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("reading response: %w", err)
	}
	err = json.Unmarshal(responseBytes, &result)
	if err != nil {
		return s3.Bucket{}, nil
	}

	return s3.Bucket{
		ID:            result.ID,
		GlobalAliases: result.GlobalAliases,
		Quotas:        s3.Quotas(result.Quotas),
	}, err
}

func (b *BucketClient) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	return nil
}
