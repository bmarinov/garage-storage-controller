package garage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

type AdminClient struct {
	*BucketClient
	*AccessKeyClient
	*PermissionClient
}

func NewClient(apiAddr string, token string) *AdminClient {
	baseClient := adminAPIHttpClient{
		httpClient: &http.Client{},
		token:      token,
		baseURL:    apiAddr,
	}

	keyClient := &AccessKeyClient{
		&baseClient,
	}
	return &AdminClient{
		BucketClient: &BucketClient{
			&baseClient,
		},
		AccessKeyClient: keyClient,
		PermissionClient: &PermissionClient{
			adminAPIHttpClient: &baseClient,
			accessKeys:         keyClient,
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
	*adminAPIHttpClient
}

func (a *AccessKeyClient) Create(ctx context.Context, keyName string) (s3.AccessKey, error) {
	// TODO: expose in client api?
	neverExpires := true

	request := CreateKeyRequest{
		Name:         keyName,
		NeverExpires: neverExpires,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return s3.AccessKey{}, fmt.Errorf("marshal request: %w", err)
	}

	const path = "/v2/CreateKey"
	response, err := a.doRequest(ctx, http.MethodPost, path, nil, &buf)

	if err != nil {
		return s3.AccessKey{}, fmt.Errorf("create key: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return s3.AccessKey{}, fmt.Errorf("unexpected status code %d", response.StatusCode)
	}

	result, err := unmarshalBody[AccessKeyResponse](response.Body)
	if err != nil {
		return s3.AccessKey{}, err
	}

	return s3.AccessKey{
		ID:     result.AccessKeyID,
		Name:   result.Name,
		Secret: result.SecretAccessKey,
	}, nil
}

func (a *AccessKeyClient) Get(ctx context.Context, id string) (s3.AccessKey, error) {
	result, err := a.get(ctx, id, "", true)
	if err != nil {
		return s3.AccessKey{}, err
	}

	return s3.AccessKey{
		ID:     result.AccessKeyID,
		Secret: result.SecretAccessKey,
		Name:   result.Name,
	}, nil
}

func (a *AccessKeyClient) Lookup(ctx context.Context, search string) (s3.AccessKey, error) {
	result, err := a.get(ctx, "", search, true)
	if err != nil {
		return s3.AccessKey{}, err
	}

	return s3.AccessKey{
		ID:     result.AccessKeyID,
		Secret: result.SecretAccessKey,
		Name:   result.Name,
	}, nil
}

func (a *AccessKeyClient) get(ctx context.Context, id string, search string, retrieveSecret bool) (AccessKeyResponse, error) {
	params := url.Values{}
	if id != "" {
		params.Add("id", id)
	}
	if search != "" {
		params.Add("search", search)
	}
	params.Add("showSecretKey", strconv.FormatBool(retrieveSecret))

	const path = "/v2/GetKeyInfo"

	response, err := a.doRequest(ctx, http.MethodGet, path, &params, nil)
	if err != nil {
		return AccessKeyResponse{}, fmt.Errorf("get key: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return AccessKeyResponse{}, fmt.Errorf("%w: id '%s'; search '%s'", s3.ErrKeyNotFound, id, search)
		}
		// TODO: inspect server side code, why bad request? workaround:
		if response.StatusCode == http.StatusBadRequest &&
			search != "" && id == "" {
			return AccessKeyResponse{}, fmt.Errorf("bad request looking for key with search term %s: %w", search, s3.ErrKeyNotFound)
		}
		return AccessKeyResponse{}, fmt.Errorf("unexpected status code %d", response.StatusCode)
	}

	return unmarshalBody[AccessKeyResponse](response.Body)
}

type BucketClient struct {
	*adminAPIHttpClient
}

func (b *BucketClient) Create(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	request := map[string]string{"globalAlias": globalAlias}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("marshal request: %w", err)
	}

	const path = "/v2/CreateBucket"
	resp, err := b.doRequest(ctx, http.MethodPost, path, nil, &buf)
	if err != nil {
		return s3.Bucket{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			return s3.Bucket{}, fmt.Errorf("%w: %w", s3.ErrBucketExists, err)
		}
		return s3.Bucket{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	result, err := unmarshalBody[BucketResponse](resp.Body)

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

	const path = "/v2/GetBucketInfo"

	resp, err := b.doRequest(ctx, http.MethodGet, path, &params, nil)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("retrieve bucket '%s': %w", globalAlias, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return s3.Bucket{}, fmt.Errorf("%w: with alias %s", s3.ErrBucketNotFound, globalAlias)
		}
		return s3.Bucket{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	result, err := unmarshalBody[BucketResponse](resp.Body)
	if err != nil {
		return s3.Bucket{}, fmt.Errorf("reading %s response: %w", path, err)
	}

	return s3.Bucket{
		ID:            result.ID,
		GlobalAliases: result.GlobalAliases,
		Quotas:        s3.Quotas(result.Quotas),
	}, err
}

func (b *BucketClient) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	request := BucketUpdateRequest{
		Quotas: Quotas(quotas),
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	const path = "/v2/UpdateBucket"
	params := url.Values{}
	params.Add("id", id)

	response, err := b.doRequest(ctx, http.MethodPost, path, &params, &buf)
	if err != nil {
		return fmt.Errorf("update bucket request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return fmt.Errorf("update bucket: %w for id '%s'", s3.ErrBucketNotFound, id)
		}
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("update bucket: unexpected status code %d: %s", response.StatusCode, string(body))
	}

	return nil
}

type PermissionClient struct {
	*adminAPIHttpClient

	accessKeys *AccessKeyClient
}

func (p *PermissionClient) SetPermissions(ctx context.Context,
	keyID string,
	bucketID string,
	permissions s3.Permissions) error {

	denyPermissions := s3.Permissions{
		Owner: !permissions.Owner,
		Read:  !permissions.Read,
		Write: !permissions.Write,
	}
	err := p.denyBucketKey(ctx, keyID, bucketID, denyPermissions)
	if err != nil {
		return err
	}

	err = p.allowBucketKey(ctx, keyID, bucketID, permissions)
	if err != nil {
		return err
	}

	return nil
}

func (p *PermissionClient) GetPermissions(ctx context.Context,
	keyID, bucketID string) (s3.Permissions, error) {
	key, err := p.accessKeys.get(ctx, keyID, "", false)
	if err != nil {
		return s3.Permissions{}, err
	}

	for i := 0; i < len(key.Buckets); i++ {
		if key.Buckets[i].ID == bucketID {

			return s3.Permissions(key.Buckets[i].Permissions), nil
		}
	}

	// key has no permissions on bucket
	return s3.Permissions{}, nil
}

func (p *PermissionClient) allowBucketKey(ctx context.Context,
	keyID, bucketID string,
	permissions s3.Permissions) error {

	request := AllowBucketKeyRequest{
		AccessKeyID: keyID,
		BucketID:    bucketID,
		Permissions: BucketKeyPerm(permissions),
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return fmt.Errorf("marshal AllowBucketKey request: %w", err)
	}

	const path = "/v2/AllowBucketKey"

	response, err := p.doRequest(ctx, http.MethodPost, path, nil, &buf)
	if err != nil {
		return fmt.Errorf("allow bucket key request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("update bucket: unexpected status code %d: %s", response.StatusCode, string(body))
	}

	return nil
}

func (p *PermissionClient) denyBucketKey(ctx context.Context,
	keyID, bucketID string,
	denyPermissions s3.Permissions) error {

	request := AllowBucketKeyRequest{
		AccessKeyID: keyID,
		BucketID:    bucketID,
		Permissions: BucketKeyPerm(denyPermissions),
	}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	if err != nil {
		return fmt.Errorf("marshal DenyBucketKey request: %w", err)
	}

	const path = "/v2/DenyBucketKey"

	response, err := p.doRequest(ctx, http.MethodPost, path, nil, &buf)
	if err != nil {
		return fmt.Errorf("deny bucket key request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("update bucket: unexpected status code %d: %s", response.StatusCode, string(body))
	}

	return nil
}

func unmarshalBody[T any](body io.ReadCloser) (T, error) {
	var result T
	err := json.NewDecoder(body).Decode(&result)
	return result, err
}
