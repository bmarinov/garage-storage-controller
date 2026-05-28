package controller

import (
	"context"
	"fmt"
	"sync"

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
