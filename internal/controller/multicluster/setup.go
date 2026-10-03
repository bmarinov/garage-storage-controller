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

// Package multicluster runs the reconcilers from its parent package, internal/controller,
// through multicluster-runtime, with one set of reconcilers per cluster.
// internal/controller must not import this package, so the single-cluster binary does
// not depend on multicluster-runtime.
package multicluster

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/controller"
)

// Garage holds the Garage clients and settings shared by all clusters.
type Garage struct {
	Buckets    controller.BucketClient
	Ownership  controller.OwnershipVerifier
	S3Endpoint string
}

// Setup registers the controllers with mgr.
func Setup(mgr mcmanager.Manager, garage Garage) error {
	c := newClusters(mgr, garage)

	if err := mcbuilder.ControllerManagedBy(mgr).
		For(&garagev1alpha1.Bucket{}).
		Named("bucket").
		Complete(bucketController{clusters: c}); err != nil {
		return fmt.Errorf("setting up bucket controller: %w", err)
	}
	return nil
}

// bucketController passes each request to the BucketReconciler of the request's cluster.
type bucketController struct {
	clusters *clusters
}

func (b bucketController) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	r, err := b.clusters.get(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.bucket.Reconcile(ctx, req.Request)
}
