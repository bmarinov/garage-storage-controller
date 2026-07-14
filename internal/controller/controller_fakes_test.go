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

package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

type permissionClientFake struct {
	mu                  sync.RWMutex
	assignedPermissions map[string]s3.Permissions
}

func newPermissionClientFake() *permissionClientFake {
	return &permissionClientFake{
		assignedPermissions: make(map[string]s3.Permissions),
	}
}

func (p *permissionClientFake) SetPermissions(_ context.Context, keyID, bucketID string, permissions s3.Permissions) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.assignedPermissions[fmt.Sprintf("%s:%s", keyID, bucketID)] = permissions
	return nil
}

func (p *permissionClientFake) GetPermissions(_ context.Context, keyID, bucketID string) (s3.Permissions, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.assignedPermissions[fmt.Sprintf("%s:%s", keyID, bucketID)], nil
}

var _ PermissionClient = &permissionClientFake{}
var _ OwnershipVerifier = &permissionClientFake{}

var errRBACDenied = errors.New("RBAC: access denied in fake")

type forbiddenResource string

const (
	forbidConfigMaps forbiddenResource = "configmaps"
	forbidSecrets    forbiddenResource = "secrets"
)

func (r forbiddenResource) matches(obj client.Object) bool {
	switch obj.(type) {
	case *corev1.ConfigMap:
		return r == forbidConfigMaps
	case *corev1.Secret:
		return r == forbidSecrets
	default:
		return false
	}
}

// forbidClientFake denies Get on one configured resource.
type forbidClientFake struct {
	client.Client
	resource forbiddenResource
}

func (c forbidClientFake) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	if c.resource.matches(obj) {
		return apierrors.NewForbidden(corev1.Resource(string(c.resource)), key.Name, errRBACDenied)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// failGetClientFake fails Get on one resource type with a custom error.
type failGetClientFake struct {
	client.Client
	resource forbiddenResource
	err      error
}

func (c failGetClientFake) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	if c.resource.matches(obj) {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var (
	_ client.Client = forbidClientFake{}
	_ client.Client = failGetClientFake{}
)
