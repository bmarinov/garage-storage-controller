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
	"crypto/sha256"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

// AccessKeyReconciler reconciles an AccessKey object
type AccessKeyReconciler struct {
	client    client.Client
	scheme    *runtime.Scheme
	accessKey AccessKeyManager
}

func NewAccessKeyReconciler(c client.Client, s *runtime.Scheme, keyMgr AccessKeyManager) *AccessKeyReconciler {
	return &AccessKeyReconciler{
		client:    c,
		scheme:    s,
		accessKey: keyMgr,
	}
}

type AccessKeyManager interface {
	Create(ctx context.Context, keyName string) (s3.AccessKey, error)
	Get(ctx context.Context, id string) (s3.AccessKey, error)
	Lookup(ctx context.Context, search string) (s3.AccessKey, error)
}

// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=garage.getclustered.net,resources=accesskeys/finalizers,verbs=update

func (r *AccessKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var accessKey garagev1alpha1.AccessKey
	err := r.client.Get(ctx, req.NamespacedName, &accessKey)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO: finalizer

	orig := accessKey.Status.DeepCopy()

	key := AccessKey{Object: &accessKey}
	key.InitializeConditions()

	err = r.reconcileAccessKey(ctx, &key)
	if err != nil {
		return ctrl.Result{}, err
	}

	key.updateStatus()

	if !equality.Semantic.DeepEqual(*orig, accessKey.Status) {
		err = r.client.Status().Patch(ctx, &accessKey, client.Merge, client.FieldOwner(bucketControllerName))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AccessKeyReconciler) reconcileAccessKey(ctx context.Context, key *AccessKey) error {
	if key.Object.Status.ObservedGeneration == key.Object.Generation &&
		key.AccessKeyCondition().Status == metav1.ConditionTrue &&
		key.Object.Status.ID != "" {
		// object exists
		return nil
	}

	externalKey, err := r.ensureExternalKey(ctx, *key.Object)
	if err != nil {
		markAccessKeyNotReady(
			key.Object,
			"AccessKeyUnknown",
			"Failed to verify external access key: %v", err)

		return err
	}

	key.Object.Status.ID = externalKey.ID
	markAccessKeyReady(key.Object)

	err = r.ensureSecret(ctx, *key.Object, externalKey.Secret)
	if err != nil {
		key.MarkNotReady(KeySecretReady, "SecretSetupFailed", "Failed to set up secret for credentials: %v", err)
		return err
	}

	if key.Object.Status.SecretName != "" && key.Object.Status.SecretName != key.Object.Spec.SecretName {
		err = r.cleanupOldSecret(ctx, key.Object.Status.SecretName, key.Object.Namespace)
		if err != nil {
			logf.FromContext(ctx).Error(err, "cleaning up old secret on spec change", "accessKeyUID", key.Object.UID)
			key.MarkNotReady(KeySecretReady, "SecretCleanupFailed",
				"New secret created but failed to delete old secret %q: %v",
				key.Object.Status.SecretName, err)

			return err
		}
	}

	key.Object.Status.SecretName = key.Object.Spec.SecretName
	key.MarkSecretReady()

	return nil
}

func (r *AccessKeyReconciler) cleanupOldSecret(ctx context.Context, secretName, namespace string) error {
	oldSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	err := r.client.Delete(ctx, &oldSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting secret '%s/%s': %w", namespace, secretName, err)
	}
	return nil
}

func (r *AccessKeyReconciler) ensureExternalKey(ctx context.Context, resource garagev1alpha1.AccessKey) (s3.AccessKey, error) {
	logger := logf.FromContext(ctx)
	externalKeyName := namespacedResourceName(resource.ObjectMeta)

	if resource.Status.ID != "" {
		existing, err := r.accessKey.Get(ctx, resource.Status.ID)
		if err != nil {
			if errors.Is(err, s3.ErrKeyNotFound) {
				logger.Info("ensuring external key: none found with stale Status.ID, recreating", "keyID", resource.Status.ID, "err", err)
				return r.accessKey.Create(ctx, externalKeyName)
			} else {
				return s3.AccessKey{}, fmt.Errorf("verifying existing key: %w", err)
			}
		}

		return existing, err
	}
	existingKey, err := r.accessKey.Lookup(ctx, externalKeyName)
	if err != nil {
		if errors.Is(err, s3.ErrKeyNotFound) {
			newKey, err := r.accessKey.Create(ctx, externalKeyName)
			return newKey, err
		} else {
			return s3.AccessKey{}, fmt.Errorf("unable to check for existing remote key: %w", err)
		}
	}

	// key exists
	logger.Info("found existing key matching name, reconciling state", "externalKeyName", externalKeyName, "resourceUID", resource.UID)
	return existingKey, nil
}

func (r *AccessKeyReconciler) ensureSecret(ctx context.Context,
	parent garagev1alpha1.AccessKey,
	secret string) error {
	secretRes := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      parent.Spec.SecretName,
			Namespace: parent.Namespace,
			Labels: map[string]string{
				"garage.getclustered.net/owned-by": parent.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}

	err := controllerutil.SetControllerReference(&parent, &secretRes, r.scheme)
	if err != nil {
		return fmt.Errorf("setting owner reference on secret: %w", err)
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.client, &secretRes, func() error {
		secretRes.Data = map[string][]byte{
			"accessKeyId":     []byte(parent.Status.ID),
			"secretAccessKey": []byte(secret),
		}
		return nil
	})

	return err
}

// namespacedResourceName returns a key name including the namespace and a suffix from the UID hash.
func namespacedResourceName(meta metav1.ObjectMeta) string {
	hash := sha256.Sum256([]byte(meta.UID))
	return fmt.Sprintf("%s-%s-%x", meta.Namespace, meta.Name, hash[:8])
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccessKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&garagev1alpha1.AccessKey{}).
		Named("accesskey").
		Complete(r)
}
