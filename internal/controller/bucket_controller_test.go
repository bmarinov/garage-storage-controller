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
	"errors"
	"slices"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			if err != nil && apierrors.IsNotFound(err) {
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
			s3API := newS3APIFake()
			controllerReconciler := &BucketReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				bucket: s3API,
			}

			existing := s3.Bucket{
				ID:            "3f5z",
				GlobalAliases: []string{bucketName},
			}
			s3API.buckets[existing.ID] = existing

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
						Equal(BucketReady),
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
						Equal(Ready),
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

			s3API := newS3APIFake()
			s3API.buckets[bucketID] = s3.Bucket{
				ID:            bucketID,
				GlobalAliases: []string{bucket.Spec.Name},
				Quotas:        s3.Quotas{},
			}
			controllerReconciler := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3API)

			// act
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).ShouldNot(HaveOccurred())

			// assert
			quota := s3API.buckets[bucketID].Quotas
			Expect(quota.MaxSize).To(Equal(bucket.Spec.MaxSize))
			Expect(quota.MaxObjects).To(Equal(bucket.Spec.MaxObjects))
		})
		It("should create external S3 bucket matching spec", func() {
			By("creating a Bucket custom resource")
			key := types.NamespacedName{Namespace: "default", Name: "bar-bucket"}
			bucketName := "foo-storage"

			resource := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name:       bucketName,
					MaxSize:    365,
					MaxObjects: 9005,
				},
			}
			Expect(k8sClient.Create(ctx, &resource)).To(Succeed())

			By("reconciling")
			var s3Fake = newS3APIFake()
			controllerReconciler := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3Fake)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).ShouldNot(HaveOccurred())

			By("waiting for external bucket to be provisioned")
			Eventually(func(g Gomega) {
				var bucket garagev1alpha1.Bucket
				g.Expect(k8sClient.Get(ctx, key, &bucket)).To(Succeed())

				g.Expect(bucket.Status.Conditions).To(ContainElement(
					SatisfyAll(
						WithTransform(
							func(c metav1.Condition) string { return c.Type },
							Equal(Ready),
						),
						WithTransform(
							func(c metav1.Condition) metav1.ConditionStatus { return c.Status },
							Equal(metav1.ConditionTrue),
						),
						WithTransform(
							func(c metav1.Condition) int64 { return c.ObservedGeneration },
							Equal(bucket.Generation),
						)),
				))
			}).Should(Succeed())

			By("comparing the bucket config with spec")
			created, _ := s3Fake.Get(ctx, bucketName)
			Expect(created.Quotas.MaxObjects).To(Equal(resource.Spec.MaxObjects))
			Expect(created.Quotas.MaxSize).To(Equal(resource.Spec.MaxSize))
		})
	})
})

func newS3APIFake() *s3APIFake {
	return &s3APIFake{
		buckets: make(map[string]s3.Bucket),
	}
}

type s3APIFake struct {
	// buckets by id
	buckets map[string]s3.Bucket
}

// Create implements S3Client.
func (s *s3APIFake) Create(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	if _, err := s.Get(ctx, globalAlias); err == nil {
		return s3.Bucket{}, errors.New("expected get error, likely already exists")
	}

	new := s3.Bucket{
		ID:            uuid.NewString(),
		GlobalAliases: []string{globalAlias},
	}
	s.buckets[new.ID] = new
	return new, nil
}

// Update implements S3Client.
func (s *s3APIFake) Update(ctx context.Context, id string, quotas s3.Quotas) error {
	b, got := s.buckets[id]
	if !got {
		return s3.ErrResourceNotFound
	}

	b.Quotas.MaxObjects = quotas.MaxObjects
	b.Quotas.MaxSize = quotas.MaxSize
	s.buckets[id] = b

	return nil
}

// Get implements S3Client.
func (s *s3APIFake) Get(ctx context.Context, globalAlias string) (s3.Bucket, error) {
	for _, v := range s.buckets {
		if slices.Contains(v.GlobalAliases, globalAlias) {
			return v, nil
		}
	}
	return s3.Bucket{}, s3.ErrResourceNotFound
}

var _ BucketClient = &s3APIFake{}
