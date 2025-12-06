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

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

// AccessKeyReconciler reconciles a AccessKey object
type AccessKeyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	accessKey AccessKeyManager
}

func NewAccessKeyReconciler(c client.Client, s *runtime.Scheme, keyMgr AccessKeyManager) *AccessKeyReconciler {
	return &AccessKeyReconciler{
		Client:    c,
		Scheme:    s,
		accessKey: keyMgr,
	}
}

type AccessKeyManager interface {
	Create(ctx context.Context, keyName string) (s3.AccessKey, error)
}

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys/finalizers,verbs=update

func (r *AccessKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	var ac garagev1alpha1.AccessKey
	err := r.Get(ctx, req.NamespacedName, &ac)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if ac.Status.ObservedGeneration == 0 {
		// new, create access key
		externalKeyName := namespacedKeyName(ac)
		_, err := r.accessKey.Create(ctx, externalKeyName)

		// TODO: handle consistency issues gracefully
		if err != nil {
			return ctrl.Result{}, err
		}

		// create secret
	}

	if ac.Status.ObservedGeneration != ac.GetGeneration() {
		// spec change

		panic("implement")
	}

	if ac.Status.ID == "" {
		// fetch key ID
	}

	// up to date
	return ctrl.Result{}, nil
}

func (r *AccessKeyReconciler) createNewKey(ctx context.Context, resource garagev1alpha1.AccessKey) error {
	externalKeyName := namespacedKeyName(resource)
	_, err := r.accessKey.Create(ctx, externalKeyName)

	if err != nil {
		return err
	}

	return nil
}

// namespacedKeyName returns a key name including the namespace to avoid conflicts.
func namespacedKeyName(ac garagev1alpha1.AccessKey) string {
	// consider a random suffix:
	return ac.ObjectMeta.Namespace + "-" + ac.ObjectMeta.Name
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessKey{}).
		Named("accesskey").
		Complete(r)
}
