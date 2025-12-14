/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
)

// AccessPolicyReconciler reconciles a AccessPolicy object
type AccessPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesspolicies/finalizers,verbs=update

func (r *AccessPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessPolicy{}).
		Named("accesspolicy").
		Complete(r)
}
