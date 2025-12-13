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
	"fmt"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

var _ = Describe("AccessKey Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AccessKey")
			accesskey := &garagev1alpha1.AccessKey{}
			err := k8sClient.Get(ctx, typeNamespacedName, accesskey)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &garagev1alpha1.AccessKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: garagev1alpha1.AccessKeySpec{
						SecretName:   "some-ns-secret",
						NeverExpires: true,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			} else if err != nil {
				panic(err)
			}

		})

		AfterEach(func() {
			resource := &garagev1alpha1.AccessKey{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			var secret corev1.Secret
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: resource.Namespace, Name: resource.Spec.SecretName}, &secret)
			if err == nil && secret.UID != "" {
				Expect(k8sClient.Delete(ctx, &secret)).To(Succeed())
			}

			By("Cleanup the specific resource instance AccessKey")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			sut, extAPI := setup()

			By("reconciling the created resource")
			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			By("creating external access key")
			Expect(extAPI.keys).To(HaveLen(1))

			By("naming external key according to convention")
			externalKey := extAPI.keys[0]
			nsName := fmt.Sprintf("%s-%s", typeNamespacedName.Namespace, typeNamespacedName.Name)

			Expect(strings.HasPrefix(externalKey.Name, nsName)).To(BeTrue())
		})
		It("sets AccessKey status and condition", func() {
			sut, extAPI := setup()
			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("setting AccessKey status with generation")
			Eventually(func(g Gomega) {
				var accessKey garagev1alpha1.AccessKey
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &accessKey)).To(Succeed())

				var keyCondition metav1.Condition
				_ = g.Expect((accessKey.Status.Conditions)).To(ContainElement(SatisfyAll(
					WithTransform(
						func(c metav1.Condition) string { return c.Type },
						Equal(AccessKeyReady),
					),
					WithTransform(
						func(c metav1.Condition) metav1.ConditionStatus { return c.Status },
						Equal(metav1.ConditionTrue),
					),
				), &keyCondition))

				g.Expect(keyCondition.ObservedGeneration).To(Equal(accessKey.Generation))
			}).Should(Succeed())

			By("storing key ID in status")
			Eventually(func(g Gomega) {
				var accessKey garagev1alpha1.AccessKey
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &accessKey)).To(Succeed())

				expected := extAPI.keys[0].ID
				g.Expect(accessKey.Status.ID).To(Equal(expected))
			}).Should(Succeed())
		})
		It("creates kubernetes secret with data from external access key", func() {
			sut, extAPI := setup()
			_, _ = sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			By("creating a kubernetes secret resource in the same namespace")
			Eventually(func(g Gomega) {
				var accessKey garagev1alpha1.AccessKey
				err := k8sClient.Get(ctx, typeNamespacedName, &accessKey)
				Expect(err).ToNot(HaveOccurred())

				var secretRes corev1.Secret
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: accessKey.Namespace, Name: accessKey.Spec.SecretName}, &secretRes)
				Expect(err).ToNot(HaveOccurred())
			}).Should(Succeed())

			By("storing access credentials in kubernetes secret")
			Eventually(func(g Gomega) {
				var accessKey garagev1alpha1.AccessKey
				_ = k8sClient.Get(ctx, typeNamespacedName, &accessKey)

				var secretRes corev1.Secret
				_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: accessKey.Namespace, Name: accessKey.Spec.SecretName}, &secretRes)

				g.Expect(string(secretRes.Data["accessKeyId"])).To(Equal(accessKey.Status.ID))
				g.Expect(string(secretRes.Data["secretAccessKey"])).To(Equal(extAPI.keys[0].Secret))
			}).Should(Succeed())
			By("setting top-level Ready when access key and secret are ready")
			var accessKey garagev1alpha1.AccessKey
			_ = k8sClient.Get(ctx, typeNamespacedName, &accessKey)
			var secretRes corev1.Secret
			_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: accessKey.Namespace, Name: accessKey.Spec.SecretName}, &secretRes)
			topReady := meta.FindStatusCondition(accessKey.Status.Conditions, Ready)
			accessKeyReady := meta.FindStatusCondition(accessKey.Status.Conditions, AccessKeyReady)
			secretReady := meta.FindStatusCondition(accessKey.Status.Conditions, KeySecretReady)
			Expect(accessKeyReady.Status).To(Equal(metav1.ConditionTrue))
			Expect(secretReady.Status).To(Equal(metav1.ConditionTrue))
			Expect(topReady.Status).To(Equal(metav1.ConditionTrue))

			By("setting owner refs")
			expectedRef := metav1.OwnerReference{
				APIVersion:         "garage.getclustered.net/v1alpha1",
				Kind:               "AccessKey",
				Name:               accessKey.Name,
				UID:                accessKey.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}
			Expect(secretRes.OwnerReferences).To(HaveLen(1))
			Expect(secretRes.OwnerReferences[0]).To(Equal(expectedRef))
		})
		It("should create different names when recreating AccessKey resources", func() {
			sut, externalAPI := setup()

			By("creating the old resource")
			commonSecretName := "recreate-secret-foo"
			oldKeyRes := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recreate-resource-foo",
					Namespace: "default",
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: commonSecretName,
				},
			}
			commonName := types.NamespacedName{
				Namespace: oldKeyRes.Namespace,
				Name:      oldKeyRes.Name,
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &oldKeyRes)
			})
			Expect(k8sClient.Create(ctx, &oldKeyRes)).To(Succeed())

			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: commonName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("fetching remote key")
			_ = k8sClient.Get(ctx, commonName, &oldKeyRes)
			oldKeyID := oldKeyRes.Status.ID
			Expect(oldKeyID).ToNot(BeEmpty())
			oldRemoteKey, err := externalAPI.Get(ctx, oldKeyID)

			Expect(err).ToNot(HaveOccurred())
			Expect(oldRemoteKey.ID).To(Equal(oldKeyID))

			By("deleting old API resources")
			oldSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      oldKeyRes.Spec.SecretName,
					Namespace: oldKeyRes.Namespace,
				},
			}
			Expect(k8sClient.Delete(ctx, &oldKeyRes)).To(Succeed())

			By("manually deleting secret in envtest with no GC")
			Expect(k8sClient.Delete(ctx, &oldSecret)).To(Succeed())

			var secretRes corev1.Secret
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: oldKeyRes.Namespace, Name: oldKeyRes.Spec.SecretName}, &secretRes)
			Expect(err).To(Satisfy(apierrors.IsNotFound))

			By("recreating new key with same name")
			newKey := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      commonName.Name,
					Namespace: commonName.Namespace,
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: commonSecretName,
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &newKey)
			})
			Expect(k8sClient.Create(ctx, &newKey)).To(Succeed())

			_, err = sut.Reconcile(ctx, reconcile.Request{NamespacedName: commonName})
			Expect(err).To(Succeed())

			By("comparing old and new IDs")
			Expect(newKey.Status.ID).To(Not(Equal(oldKeyID)))
		})
		FIt("should reconcile after intermittent error on create", func() {
			sut, externalAPI := setup()
			accessKey := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "reconcile-after-create",
					Namespace: "default",
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: "f61xg",
				},
			}

			By("external key created")
			expectedName := namespacedResourceName(accessKey.ObjectMeta)
			externalKey, err := externalAPI.Create(ctx, expectedName)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling API resource")
			Expect(k8sClient.Create(ctx, &accessKey)).To(Succeed())
			resourceName := types.NamespacedName{
				Namespace: accessKey.Namespace,
				Name:      accessKey.Name}

			_, err = sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("retrieving API resource")
			var retrievedKey garagev1alpha1.AccessKey
			Expect(k8sClient.Get(ctx, resourceName, &retrievedKey)).To(Succeed())

			By("storing existing external key ID in status")
			Expect(retrievedKey.Status.ID).To(Equal(externalKey.ID),
				"should not create new key")

			// error during reconcile
			// key exists but no Status.ID
			// setup test double or recreate state after AC error?
		})
		It("should reconcile with stale status and existing k8s secret", func() {
			// create secret
			// fail on patch
			// restart recon
			// should not create duplicate secrets
		})
		// more tests:
		// - naming conflict with existing corev1 secret
	})
})

func setup() (*AccessKeyReconciler, *accessMgrFake) {
	externalAPI := newAccessMgrFake()

	return NewAccessKeyReconciler(k8sClient, k8sClient.Scheme(), externalAPI), externalAPI
}

type accessMgrFake struct {
	keys []s3.AccessKey
}

func newAccessMgrFake() *accessMgrFake {
	return &accessMgrFake{}
}

// Get implements AccessKeyManager.
func (a *accessMgrFake) Get(ctx context.Context, id string) (s3.AccessKey, error) {
	for _, v := range a.keys {
		if v.ID == id {
			return v, nil
		}
	}
	return s3.AccessKey{}, s3.ErrKeyNotFound
}

// Lookup implements AccessKeyManager.
func (a *accessMgrFake) Lookup(ctx context.Context, search string) (s3.AccessKey, error) {
	for _, v := range a.keys {
		if v.Name == search {
			return v, nil
		}
	}
	return s3.AccessKey{}, s3.ErrKeyNotFound
}

// Create implements AccessKeyManager.
func (a *accessMgrFake) Create(ctx context.Context, keyName string) (s3.AccessKey, error) {
	for _, v := range a.keys {
		if v.Name == keyName {
			return s3.AccessKey{}, fmt.Errorf("key %s exists", keyName)
		}
	}

	key := s3.AccessKey{
		ID:     uuid.NewString(),
		Secret: uuid.NewString(),
		Name:   keyName,
	}
	a.keys = append(a.keys, key)
	return key, nil
}

var _ AccessKeyManager = &accessMgrFake{}
