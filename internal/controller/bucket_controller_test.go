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
	"slices"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/bmarinov/garage-storage-controller/internal/tests/fixture"
)

var _ = Describe("Bucket Controller", func() {
	var namespace string
	BeforeEach(func() {
		By("create namespace for test")
		namespace = fixture.RandAlpha(8)
		testNs := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &testNs)).To(Succeed())
	})
	Context("When creating a new bucket", func() {
		It("should create external S3 bucket matching spec", func() {
			By("creating a Bucket custom resource")
			bucketName := "foo-storage"
			objID := types.NamespacedName{Namespace: namespace, Name: bucketName}

			resource := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      objID.Name,
					Namespace: objID.Namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name:       bucketName,
					MaxSize:    resource.MustParse("365Mi"),
					MaxObjects: 9005,
				},
			}
			expectedSizeBytes := int64(365 * 1024 * 1024)
			Expect(k8sClient.Create(ctx, &resource)).To(Succeed())

			By("reconciling")
			var s3Fake = newS3APIFake()
			s3Endpoint := "https://foo.bar:3456/baz"
			controllerReconciler := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3Fake, s3Endpoint)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ShouldNot(HaveOccurred())

			By("waiting for external bucket to be provisioned")
			var bucket garagev1alpha1.Bucket
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, objID, &bucket)).To(Succeed())

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

			By("retrieving bucket with suffixed name")
			hash := sha256.Sum256([]byte(bucket.UID))
			expectedName := fmt.Sprintf("%s-%x", bucketName, hash[:8])
			created, err := s3Fake.Get(ctx, expectedName)
			Expect(err).ToNot(HaveOccurred(), "bucket should exist: %s", expectedName)

			By("comparing the bucket config with spec")
			Expect(created.Quotas.MaxObjects).To(Equal(resource.Spec.MaxObjects))
			Expect(created.Quotas.MaxSize).To(Equal(resource.Spec.MaxSize.Value()))
			Expect(created.Quotas.MaxSize).To(Equal(expectedSizeBytes))

			By("creating a ConfigMap with bucket details")
			expectedCMName := bucket.Name
			Eventually(func(g Gomega) {
				var configmap corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx,
					types.NamespacedName{Namespace: namespace, Name: expectedCMName},
					&configmap)).To(Succeed())

				g.Expect(configmap.Data[ConfigMapKeyBucketName]).To(Equal(bucket.Status.BucketName))
				g.Expect(configmap.Data[ConfigMapKeyEndpoint]).To(Equal(s3Endpoint))
			}).Should(Succeed())
		})

		It("should create ConfigMap with connection details", func() {
			By("creating the bucket")
			resource := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(12),
					Namespace: namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name: "crate-with-crabs",
				},
			}
			Expect(k8sClient.Create(ctx, &resource)).To(Succeed())
			var s3Fake = newS3APIFake()
			s3API := "https://s3.test.fooz:3909"
			sut := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3Fake, s3API)

			// Note: need to reconcile with Eventually once finalizer is added:
			_, err := sut.Reconcile(ctx,
				reconcile.Request{NamespacedName: namespacedName(resource.ObjectMeta)})
			Expect(err).ToNot(HaveOccurred())

			_ = k8sClient.Get(ctx, namespacedName(resource.ObjectMeta), &resource)

			By("storing Bucket details in ConfigMap")
			expectedCMName := resource.Name
			var configmap corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: resource.Namespace, Name: expectedCMName}, &configmap)).
				To(Succeed())

			Expect(configmap.Data).To(HaveKey(ConfigMapKeyEndpoint))
			Expect(configmap.Data).To(HaveKey(ConfigMapKeyBucketName))

			Expect(configmap.Data[ConfigMapKeyEndpoint]).To(Equal(s3API))
			Expect(configmap.Data[ConfigMapKeyBucketName]).To(Equal(resource.Status.BucketName))
		})

		// TODO:
		// It("sets status and reason on configmap name conflict")
	})

	Context("When reconciling existing buckets", func() {
		It("should successfully reconcile for existing external resource", func() {
			bucketName := fixture.RandAlpha(12)
			bucket := &garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(8),
					Namespace: namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name: bucketName,
				},
			}
			Expect(k8sClient.Create(ctx, bucket)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, bucket)
			})

			By("simulating reconcile error after external bucket creation")
			s3API := newS3APIFake()
			existing := s3.Bucket{
				ID:            "3f5z",
				GlobalAliases: []string{suffixedResourceName(bucket.ObjectMeta)},
			}
			s3API.buckets[existing.ID] = existing

			By("reconciling the created resource")
			sut := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3API, "https://abc")

			bucketObjID := namespacedName(bucket.ObjectMeta)
			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: bucketObjID,
			})
			Expect(err).NotTo(HaveOccurred())

			By("storing resource condition and generation in status")
			Eventually(func(g Gomega) {
				var bucket garagev1alpha1.Bucket
				g.Expect(k8sClient.Get(ctx, bucketObjID, &bucket)).To(Succeed())

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
				g.Expect(k8sClient.Get(ctx, bucketObjID, &bucket)).To(Succeed())

				g.Expect(bucket.Status.BucketID).To(Equal(existing.ID))
			}).Should(Succeed())
		})
		It("should apply modifications to existing bucket", func() {
			newBucket := &garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(12),
					Namespace: namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name: fixture.RandAlpha(12),
				},
			}

			Expect(k8sClient.Create(ctx, newBucket)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, newBucket)
			})

			s3API := newS3APIFake()
			controllerReconciler := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3API, "foo/bar")

			By("reconciling")
			objID := namespacedName(newBucket.ObjectMeta)
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: objID})

			By("setting bucket quota")
			var bucket garagev1alpha1.Bucket
			Expect(k8sClient.Get(ctx, objID, &bucket)).To(Succeed())

			bucket.Spec.MaxSize = resource.MustParse("9500k")
			bucket.Spec.MaxObjects = 900
			Expect(k8sClient.Update(ctx, &bucket)).To(Succeed())

			// act
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ShouldNot(HaveOccurred())

			// assert
			quota := s3API.buckets[bucket.Status.BucketID].Quotas
			Expect(quota.MaxSize).To(Equal(bucket.Spec.MaxSize.Value()))
			Expect(quota.MaxObjects).To(Equal(bucket.Spec.MaxObjects))
		})
		It("recreates missing ConfigMap on delete", func() {
			bucket := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(11),
					Namespace: namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					Name: fixture.RandAlpha(8),
				},
			}
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			By("reconciling initial create")
			s3API := newS3APIFake()
			sut := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3API, "https://foo.bar")
			Expect(sut.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName(bucket.ObjectMeta)})).
				Error().ToNot(HaveOccurred())

			By("fetching original ConfigMap")
			cmName := bucket.Name
			var bucketConfigMap corev1.ConfigMap
			configmapKey := types.NamespacedName{Name: cmName, Namespace: namespace}
			Expect(k8sClient.
				Get(ctx, configmapKey, &bucketConfigMap)).
				To(Succeed())
			originalUUID := bucketConfigMap.UID

			By("deleting CM out of band")
			Expect(k8sClient.Delete(ctx, &bucketConfigMap)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, configmapKey, &bucketConfigMap)
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())

			By("triggering reconcile manually")
			Expect(sut.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName(bucket.ObjectMeta)})).
				Error().ToNot(HaveOccurred())

			By("bucket ConfigMap recreated")
			Eventually(func(g Gomega) {
				var new corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, configmapKey, &new)).To(Succeed())
				Expect(new.UID).ToNot(Equal(originalUUID))
			}).Should(Succeed(), "should create new configmap resource")
		})
	})

	Context("When creating bucket API resources", func() {
		DescribeTable("validates bucket names",
			func(bucketName string, isValid bool) {
				resource := garagev1alpha1.Bucket{
					ObjectMeta: metav1.ObjectMeta{
						Name:      bucketName,
						Namespace: namespace,
					},
					Spec: garagev1alpha1.BucketSpec{
						Name: bucketName,
					},
				}

				err := k8sClient.Create(ctx, &resource)

				DeferCleanup(func() {
					_ = k8sClient.Delete(ctx, &resource)
				})

				if isValid {
					Expect(err).To(Succeed())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
			Entry("short name", "foo", true),
			Entry("valid max length", "pqcvgdh4wtcqnryt5bsvguakutbdhjzb01234567890xyz", true),
			Entry("too long", "pqcvgdh4wtcqnryt5bsvguakutbdhjzb01234567890xyz1", false),
			Entry("upper case name", "FooBar", false),
			Entry("too short", "ab", false),
			Entry("starts with hyphen", "-invalid", false),
		)
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
