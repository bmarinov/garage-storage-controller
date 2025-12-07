package garage

type Bucket struct {
	ID string `json:"id"`

	GlobalAliases []string `json:"globalAliases"`

	Quotas Quotas `json:"quotas"`
}

type Quotas struct {
	MaxObjects int64 `json:"maxObjects"`
	MaxSize    int64 `json:"maxSize"`
}

type AccessKey struct {
	ID     string
	Secret string
	Name   string
}
