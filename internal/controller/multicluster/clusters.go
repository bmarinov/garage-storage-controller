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

package multicluster

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/cluster"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	"github.com/bmarinov/garage-storage-controller/internal/controller"
)

// reconcilers holds the reconcilers for one cluster. They share one client, as the
// reconcilers in cmd/main.go share mgr.GetClient().
type reconcilers struct {
	bucket *controller.BucketReconciler
}

// clusters creates the reconcilers for a cluster on first use and keeps them.
type clusters struct {
	mgr    mcmanager.Manager
	garage Garage

	mu     sync.Mutex
	byName map[multicluster.ClusterName]*reconcilers
}

func newClusters(mgr mcmanager.Manager, garage Garage) *clusters {
	return &clusters{
		mgr:    mgr,
		garage: garage,
		byName: map[multicluster.ClusterName]*reconcilers{},
	}
}

// get returns the reconcilers for the named cluster, creating them on first use.
func (c *clusters) get(ctx context.Context, name multicluster.ClusterName) (*reconcilers, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r, ok := c.byName[name]; ok {
		return r, nil
	}

	cl, err := c.mgr.GetCluster(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting cluster %q: %w", name, err)
	}
	r, err := c.newReconcilers(cl)
	if err != nil {
		return nil, fmt.Errorf("creating reconcilers for cluster %q: %w", name, err)
	}
	c.byName[name] = r
	return r, nil
}

func (c *clusters) newReconcilers(cl cluster.Cluster) (*reconcilers, error) {
	apiClient, err := newClusterClient(cl)
	if err != nil {
		return nil, err
	}

	return &reconcilers{
		bucket: controller.NewBucketReconciler(
			apiClient,
			cl.GetScheme(),
			c.garage.Buckets,
			c.garage.S3Endpoint,
			c.garage.Ownership,
			cl.GetEventRecorderFor("garage-bucket-controller"),
		),
	}, nil
}
