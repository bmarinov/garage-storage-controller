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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
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
			if err != nil && errors.IsNotFound(err) {
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
			conventionalName := fmt.Sprintf("%s-%s", typeNamespacedName.Namespace, typeNamespacedName.Name)
			Expect(externalKey.Name).To(Equal(conventionalName))
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

			// By("setting secret ref in AccessKey status")

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
		})

		// It("should reconcile with existing external access key", func() {
		// })
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
func (a *accessMgrFake) Get(ctx context.Context, id string, search string) (s3.AccessKey, error) {
	panic("unimplemented")
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
