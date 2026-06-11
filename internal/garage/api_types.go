// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package garage

import "github.com/bmarinov/garage-storage-controller/internal/s3"

type BucketUpdateRequest struct {
	Quotas Quotas `json:"quotas"`
}

type BucketResponse struct {
	ID            string   `json:"id"`
	GlobalAliases []string `json:"globalAliases"`
	Quotas        Quotas   `json:"quotas"`
}

type Quotas struct {
	MaxObjects *int64 `json:"maxObjects"`
	MaxSize    *int64 `json:"maxSize,omitempty"`
}

func quotasFromS3(q s3.Quotas) Quotas {
	var g Quotas
	if q.MaxObjects > 0 {
		g.MaxObjects = &q.MaxObjects
	}
	if q.MaxSize > 0 {
		g.MaxSize = &q.MaxSize
	}
	return g
}

func quotasToS3(q Quotas) s3.Quotas {
	var out s3.Quotas
	if q.MaxObjects != nil {
		out.MaxObjects = *q.MaxObjects
	}
	if q.MaxSize != nil {
		out.MaxSize = *q.MaxSize
	}
	return out
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
	ID            string        `json:"id"`
	GlobalAliases []string      `json:"globalAliases"`
	Permissions   BucketKeyPerm `json:"permissions"`
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
