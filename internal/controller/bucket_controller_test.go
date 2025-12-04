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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
)

var _ = Describe("Bucket Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-bucket"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		bucketName := "foo-bucket-alias"

		BeforeEach(func() {
			By("creating the custom resource for the Kind Bucket")
			bucket := &garagev1alpha1.Bucket{}
			err := k8sClient.Get(ctx, typeNamespacedName, bucket)
			if err != nil && errors.IsNotFound(err) {
				resource := &garagev1alpha1.Bucket{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: garagev1alpha1.BucketSpec{
						Name: bucketName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &garagev1alpha1.Bucket{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Bucket")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile for existing external resource", func() {
			By("Reconciling the created resource")
			stub := newS3APIStub()
			controllerReconciler := &BucketReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				s3:     stub,
			}

			existing := s3.Bucket{
				ID:            "3f5z",
				GlobalAliases: []string{bucketName},
			}
			stub.buckets[existing.ID] = existing

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("updating resource condition and generation")
			Eventually(func(g Gomega) {
				var bucket garagev1alpha1.Bucket
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &bucket)).To(Succeed())

				var bucketCond metav1.Condition
				_ = g.Expect((bucket.Status.Conditions)).To(ContainElement(SatisfyAll(
					WithTransform(
						func(c metav1.Condition) string { return c.Type },
						Equal(string(ConditionBucketReady)),
					),
					WithTransform(
						func(c metav1.Condition) metav1.ConditionStatus { return c.Status },
						Equal(metav1.ConditionTrue),
					),
				), &bucketCond))

				var readyCond metav1.Condition
				_ = g.Expect((bucket.Status.Conditions)).To(ContainElement(SatisfyAll(
					WithTransform(
						func(c metav1.Condition) string { return c.Type },
						Equal(string(ConditionReady)),
					),
					WithTransform(
						func(c metav1.Condition) metav1.ConditionStatus { return c.Status },
						Equal(metav1.ConditionTrue),
					),
				), &readyCond))

				g.Expect(bucketCond.ObservedGeneration).To(Equal(bucket.Generation))
				g.Expect(readyCond.ObservedGeneration).To(Equal(bucket.Generation))
			}).Should(Succeed())

			By("setting bucket ID")
			Eventually(func(g Gomega) {
				var bucket garagev1alpha1.Bucket
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &bucket)).To(Succeed())

				g.Expect(bucket.Status.BucketID).To(Equal(existing.ID))
			}).Should(Succeed())
		})
		It("should apply modifications to existing bucket", func() {
			By("updating bucket quota")

			var bucket garagev1alpha1.Bucket

			Expect(k8sClient.Get(ctx, typeNamespacedName, &bucket)).To(Succeed())
			bucket.Spec.MaxSize = 9500
			bucket.Spec.MaxObjects = 900
			Expect(k8sClient.Update(ctx, &bucket)).To(Succeed())

			const bucketID = "abc333"

			stub := newS3APIStub()
			stub.buckets[bucketID] = s3.Bucket{
				ID:            bucketID,
				GlobalAliases: []string{bucket.Spec.Name},
				Quotas:        s3.Quotas{},
			}
			controllerReconciler := &BucketReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				s3:     stub,
			}

			// act
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).ShouldNot(HaveOccurred())

			// assert
			quota := stub.buckets[bucketID].Quotas
			Expect(quota.MaxSize).To(Equal(bucket.Spec.MaxSize))
			Expect(quota.MaxObjects).To(Equal(bucket.Spec.MaxObjects))
		})
	})
})

func newS3APIStub() *s3APIStub {
	return &s3APIStub{
		buckets: make(map[string]s3.Bucket),
	}
}

type s3APIStub struct {
	// buckets by id
	buckets map[string]s3.Bucket
}

// Update implements S3Client.
func (s *s3APIStub) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	b, got := s.buckets[id]
	if !got {
		return s3.ErrBucketNotFound
	}

	b.Quotas.MaxObjects = quotas.MaxObjects
	b.Quotas.MaxSize = quotas.MaxSize
	s.buckets[id] = b

	return nil
}

// Get implements S3Client.
func (s *s3APIStub) Get(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	for _, v := range s.buckets {
		if slices.Contains(v.GlobalAliases, globalAlias) {
			return v, nil
		}
	}
	return s3.Bucket{}, s3.ErrBucketNotFound
}

var _ S3Client = &s3APIStub{}
