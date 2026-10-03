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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mchandler "sigs.k8s.io/multicluster-runtime/pkg/handler"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/controller"
)

// Garage holds the Garage clients and settings shared by all clusters.
type Garage struct {
	Buckets     controller.BucketClient
	Ownership   controller.OwnershipVerifier
	AccessKeys  controller.AccessKeyManager
	Permissions controller.PermissionClient
	S3Endpoint  string
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
	if err := mcbuilder.ControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessKey{}).
		Named("accesskey").
		Complete(accessKeyController{clusters: c}); err != nil {
		return fmt.Errorf("setting up accesskey controller: %w", err)
	}
	if err := mcbuilder.ControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessPolicy{}).
		Watches(&garagev1alpha1.AccessKey{},
			enqueuePolicies(c, (*controller.AccessPolicyReconciler).FindPoliciesForAccessKey)).
		Watches(&garagev1alpha1.Bucket{},
			enqueuePolicies(c, (*controller.AccessPolicyReconciler).FindPoliciesForBucket)).
		Named("accesspolicy").
		Complete(accessPolicyController{clusters: c}); err != nil {
		return fmt.Errorf("setting up accesspolicy controller: %w", err)
	}
	return nil
}

// enqueuePolicies queues the AccessPolicies that reference a changed AccessKey or Bucket.
// It finds and queues them in the cluster where the change happened.
func enqueuePolicies(
	c *clusters,
	find func(*controller.AccessPolicyReconciler, context.Context, client.Object) []reconcile.Request,
) mchandler.TypedEventHandlerFunc[client.Object, mcreconcile.Request] {
	return func(name multicluster.ClusterName, cl cluster.Cluster) handler.TypedEventHandler[client.Object, mcreconcile.Request] {
		return mchandler.TypedForCluster[client.Object](
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				r, err := c.forCluster(name, cl)
				if err != nil {
					ctrl.LoggerFrom(ctx).Error(err, "mapping event to AccessPolicies", "cluster", name)
					return nil
				}
				return find(r.accessPolicy, ctx, obj)
			}),
			name,
		)
	}
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

// accessKeyController passes each request to the AccessKeyReconciler of the request's cluster.
type accessKeyController struct {
	clusters *clusters
}

func (a accessKeyController) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	r, err := a.clusters.get(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.accessKey.Reconcile(ctx, req.Request)
}

// accessPolicyController passes each request to the AccessPolicyReconciler of the request's cluster.
type accessPolicyController struct {
	clusters *clusters
}

func (a accessPolicyController) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	r, err := a.clusters.get(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.accessPolicy.Reconcile(ctx, req.Request)
}
