package garage

type BucketUpdateRequest struct {
	Quotas Quotas `json:"quotas"`
}

type BucketResponse struct {
	ID            string   `json:"id"`
	GlobalAliases []string `json:"globalAliases"`
	Quotas        Quotas   `json:"quotas"`
}

type Quotas struct {
	MaxObjects int64 `json:"maxObjects"`
	MaxSize    int64 `json:"maxSize"`
}

type CreateKeyRequest struct {
	Name         string `json:"name"`
	NeverExpires bool   `json:"neverExpires"`
}

type AccessKeyResponse struct {
	AccessKeyID     string                  `json:"accessKeyId"`
	Buckets         []KeyInfoBucketResponse `json:"buckets"`
	SecretAccessKey string                  `json:"secretAccessKey"`
	Name            string                  `json:"name"`
}

type KeyInfoBucketResponse struct {
	ID          string        `json:"id"`
	Permissions BucketKeyPerm `json:"permissions"`
}

type AllowBucketKeyRequest struct {
	AccessKeyID string        `json:"accessKeyId"`
	BucketID    string        `json:"bucketId"`
	Permissions BucketKeyPerm `json:"permissions"`
}

type BucketKeyPerm struct {
	Owner bool `json:"owner"`
	Read  bool `json:"read"`
	Write bool `json:"write"`
}
