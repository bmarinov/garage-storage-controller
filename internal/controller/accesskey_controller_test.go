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
	"slices"
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
	"github.com/bmarinov/garage-storage-controller/internal/tests/fixture"
)

var _ = Describe("AccessKey Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var namespace string

		BeforeEach(func() {
			By("setting up namespace")
			namespace = fixture.RandAlpha(10)
			testNs := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, &testNs)).To(Succeed())

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
			ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Delete(ctx, &ns)).To(Succeed())

			resource := &garagev1alpha1.AccessKey{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			var secret corev1.Secret
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: resource.Namespace, Name: resource.Spec.SecretName}, &secret)
			if err == nil && secret.UID != "" {
				Expect(k8sClient.Delete(ctx, &secret)).To(Succeed())
			}

			By("Cleanup the specific resource instance AccessKey")
			if len(resource.Finalizers) > 0 {
				resource.Finalizers = []string{}
				_ = k8sClient.Update(ctx, resource)
			}

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

			By("storing key ID and secret name in status")
			Eventually(func(g Gomega) {
				var accessKey garagev1alpha1.AccessKey
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &accessKey)).To(Succeed())

				expectedKey := extAPI.keys[0]
				g.Expect(accessKey.Status.AccessKeyID).To(Equal(expectedKey.ID))
				g.Expect(accessKey.Status.SecretName).To(Equal(accessKey.Spec.SecretName))

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

				g.Expect(string(secretRes.Data["accessKeyId"])).To(Equal(accessKey.Status.AccessKeyID))
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
		It("should generate different key names when recreating AccessKey resources", func() {
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
			oldKeyID := oldKeyRes.Status.AccessKeyID
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
			_, _ = sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: commonName,
			})

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
			Expect(newKey.Status.AccessKeyID).To(Not(Equal(oldKeyID)))
		})
		It("should reconcile after transient error on create", func() {
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
			Expect(k8sClient.Create(ctx, &accessKey)).To(Succeed())
			resourceName := types.NamespacedName{
				Namespace: accessKey.Namespace,
				Name:      accessKey.Name,
			}

			By("external key created")
			expectedName := namespacedResourceName(accessKey.ObjectMeta)
			externalKey, err := externalAPI.Create(ctx, expectedName)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling API resource")

			_, err = sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("retrieving API resource")
			var retrievedKey garagev1alpha1.AccessKey
			Expect(k8sClient.Get(ctx, resourceName, &retrievedKey)).To(Succeed())

			By("storing existing external key ID in status")
			Expect(retrievedKey.Status.AccessKeyID).To(Equal(externalKey.ID),
				"should not create new key")
		})
		It("should replace secret on spec change", func() {
			sut, externalAPI := setup()
			By("reconcile with original spec")
			_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).ToNot(HaveOccurred())
			var accessKey garagev1alpha1.AccessKey
			_ = k8sClient.Get(ctx, typeNamespacedName, &accessKey)

			By("snapshot existing key and secret")
			oldSecretID := types.NamespacedName{Name: accessKey.Spec.SecretName, Namespace: accessKey.Namespace}
			var oldSecret corev1.Secret
			Expect(k8sClient.Get(ctx, oldSecretID, &oldSecret)).To(Succeed())
			existingExtKey, _ := externalAPI.Get(ctx, accessKey.Status.AccessKeyID)

			By("change secret name in spec")
			accessKey.Spec.SecretName = "changed-name-workload-foo123"
			Expect(k8sClient.Update(ctx, &accessKey)).To(Succeed())
			_, err = sut.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).ToNot(HaveOccurred())

			By("new secret with existing external key created")
			var newSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accessKey.Spec.SecretName, Namespace: accessKey.Namespace}, &newSecret)).
				To(Succeed())
			_ = k8sClient.Get(ctx, typeNamespacedName, &accessKey)
			Expect(accessKey.Status.AccessKeyID).To(Equal(existingExtKey.ID), "key ID should not change")

			Expect(newSecret.Data["accessKeyId"]).To(Equal(oldSecret.Data["accessKeyId"]))
			Expect(newSecret.Data["secretAccessKey"]).To(Equal(oldSecret.Data["secretAccessKey"]))

			By("old secret deleted")
			Expect(k8sClient.Get(ctx, oldSecretID, &oldSecret)).To(Not(Succeed()), "old secret should no longer exist")
		})
		It("should reconcile from stale ready when external key missing", func() {
			sut, externalAPI := setup()
			accessKey := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "key-went-poof",
					Namespace: "default",
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: "5zbrapp",
				},
			}
			Expect(k8sClient.Create(ctx, &accessKey)).To(Succeed())

			resourceName := types.NamespacedName{
				Namespace: accessKey.Namespace,
				Name:      accessKey.Name,
			}
			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("deleting external key out of band")
			// TODO: can use API client here:
			externalAPI.keys = []s3.AccessKey{}

			By("changing spec and reconciling")
			Expect(k8sClient.Get(ctx, resourceName, &accessKey)).To(Succeed())
			newSecretName := "changed-secret-321"
			accessKey.Spec.SecretName = newSecretName
			Expect(k8sClient.Update(ctx, &accessKey)).To(Succeed())
			_, err = sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("new access key ID stored in status")
			_ = k8sClient.Get(ctx, resourceName, &accessKey)
			newKey, err := externalAPI.Get(ctx, accessKey.Status.AccessKeyID)
			Expect(err).ToNot(HaveOccurred())

			Expect(accessKey.Status.AccessKeyID).To(Equal(newKey.ID))

			By("access key in namespace secret")
			var newSecret corev1.Secret
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: accessKey.Namespace, Name: accessKey.Spec.SecretName},
				&newSecret)).
				To(Succeed())

			Expect(string(newSecret.Data["accessKeyId"])).To(Equal(newKey.ID))
			Expect(string(newSecret.Data["secretAccessKey"])).To(Equal(newKey.Secret))
		})
		It("removes external key when resource is deleted", func() {
			sut, externalAPI := setup()

			accessKey := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(10),
					Namespace: namespace,
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: fixture.RandAlpha(10),
				},
			}
			Expect(k8sClient.Create(ctx, &accessKey)).To(Succeed())
			objID := types.NamespacedName{Name: accessKey.Name, Namespace: accessKey.Namespace}

			_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() []string {
				_ = k8sClient.Get(ctx, objID, &accessKey)
				finalizers := accessKey.GetFinalizers()
				return finalizers
			}).Should(ContainElement(finalizerName), "should register finalizer")

			By("deleting AccessKey resource")
			Expect(k8sClient.Delete(ctx, &accessKey)).To(Succeed())

			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, objID, &accessKey)).To(Succeed())
				hasDeletionTS := !accessKey.DeletionTimestamp.IsZero()
				return hasDeletionTS
			}).Should(BeTrue(), "resource should exist with deletion timestamp")

			By("reconciling AccessKey")
			_, err = sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ToNot(HaveOccurred())

			By("external key and resource removed")
			Expect(externalAPI.keys).To(BeEmpty())
			err = k8sClient.Get(ctx, objID, &accessKey)
			Expect(err).To(Satisfy(apierrors.IsNotFound))
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

// Delete implements AccessKeyManager.
func (a *accessMgrFake) Delete(ctx context.Context, id string) error {
	for idx, key := range a.keys {
		if key.ID == id {
			a.keys = slices.Delete(a.keys, idx, idx+1)
			return nil
		}
	}
	return s3.ErrKeyNotFound
}

var _ AccessKeyManager = &accessMgrFake{}
