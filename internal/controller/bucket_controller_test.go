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
	"k8s.io/apimachinery/pkg/api/meta"
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
			bucket := newBucket(namespace)
			bucket.Spec.MaxSize = resource.MustParse("365Mi")
			bucket.Spec.MaxObjects = 9005
			expectedSizeBytes := int64(365 * 1024 * 1024)
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			By("reconciling")
			var s3Fake = newS3APIFake()
			s3Endpoint := "https://foo.bar:3456/baz"
			controllerReconciler := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3Fake, s3Endpoint)

			_, err := controllerReconciler.Reconcile(ctx,
				reconcile.Request{NamespacedName: namespacedName(bucket.ObjectMeta)})
			Expect(err).ShouldNot(HaveOccurred())

			By("waiting for external bucket to be provisioned")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
				g.Expect(checkCondition(bucket.Status.Conditions, Ready, metav1.ConditionTrue)).To(Succeed())
				bucketCond := meta.FindStatusCondition(bucket.Status.Conditions, Ready)
				g.Expect(bucketCond.ObservedGeneration).To(Equal(bucket.Generation))
			}).Should(Succeed())

			By("retrieving bucket with suffixed name")
			hash := sha256.Sum256([]byte(bucket.UID))
			expectedName := fmt.Sprintf("%s-%x", bucket.Spec.Name, hash[:8])
			created, err := s3Fake.Get(ctx, expectedName)
			Expect(err).ToNot(HaveOccurred(), "bucket should exist: %s", expectedName)

			By("comparing the bucket config with spec")
			Expect(created.Quotas.MaxObjects).To(Equal(bucket.Spec.MaxObjects))
			Expect(created.Quotas.MaxSize).To(Equal(bucket.Spec.MaxSize.Value()))
			Expect(created.Quotas.MaxSize).To(Equal(expectedSizeBytes))

			By("creating a ConfigMap with bucket details")
			expectedCMName := bucket.Name
			Eventually(func(g Gomega) {
				var configmap corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx,
					types.NamespacedName{Namespace: namespace, Name: expectedCMName},
					&configmap)).To(Succeed())

				g.Expect(configmap.Data[configMapKeyBucketName]).To(Equal(bucket.Status.BucketName))
				g.Expect(configmap.Data[configMapKeyEndpoint]).To(Equal(s3Endpoint))
			}).Should(Succeed())
		})

		It("should create ConfigMap with connection details", func() {
			By("creating the bucket")
			resource := newBucket(namespace)
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

			Expect(configmap.Data).To(HaveKey(configMapKeyEndpoint))
			Expect(configmap.Data).To(HaveKey(configMapKeyBucketName))

			Expect(configmap.Data[configMapKeyEndpoint]).To(Equal(s3API))
			Expect(configmap.Data[configMapKeyBucketName]).To(Equal(resource.Status.BucketName))
		})

		It("creates cm with name from spec", func() {
			By("creating bucket with configMapName set")
			resource := newBucket(namespace)
			resource.Spec.ConfigMapName = "bucket-config"
			Expect(k8sClient.Create(ctx, &resource)).To(Succeed())

			sut, _ := setupBucket()
			Expect(sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName(resource.ObjectMeta),
			})).Error().ToNot(HaveOccurred())

			By("verifying ConfigMap name equals spec")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Name: resource.Spec.ConfigMapName, Namespace: namespace}, &cm)).
				To(Succeed(), "should find bucket with configured name")
			Expect(cm.Name).To(Equal(resource.Spec.ConfigMapName))
		})

		It("handles configmap rename in Bucket spec", func() {
			bucket := newBucket(namespace)
			bucket.Spec.ConfigMapName = "blappers-config"
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			sut, _ := setupBucket()
			By("reconciling with old CM name")
			Expect(sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName(bucket.ObjectMeta),
			})).Error().ToNot(HaveOccurred())

			var oldCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace,
				Name: bucket.Spec.ConfigMapName}, &oldCM)).To(Succeed())

			By("changing spec and reconciling")
			_ = k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)
			newName := "foo-configuration"
			bucket.Spec.ConfigMapName = newName
			Expect(k8sClient.Update(ctx, &bucket)).To(Succeed())
			_, _ = sut.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName(bucket.ObjectMeta)})

			By("new bucket ConfigMap created")
			var newCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace,
				Name: newName}, &newCM)).Should(Succeed())
			Expect(newCM.OwnerReferences).To(
				ContainElement(HaveField("Name", bucket.Name)), "Bucket should own new CM")

			By("ensuring old ConfigMap is deleted")
			Expect(k8sClient.Get(ctx, namespacedName(oldCM.ObjectMeta), &oldCM)).Error().
				To(Satisfy(apierrors.IsNotFound), "should fail fetching old CM")
		})

		It("sets correct condition and reason on ConfigMap name conflict", func() {
			bucket := newBucket(namespace)
			cmName := bucket.Name

			By("ConfigMap with name already exists")
			existing := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: namespace,
				},
				Data: map[string]string{"unrelated": "other"},
			}
			Expect(k8sClient.Create(ctx, &existing)).To(Succeed())

			By("create Bucket with conflicting ConfigMap name")
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			sut, _ := setupBucket()
			_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName(bucket.ObjectMeta)})
			Expect(err).ToNot(HaveOccurred(), "no error in conflict state, should not retry")

			By("setting correct condition status")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
				g.Expect(checkCondition(bucket.Status.Conditions, BucketReady, metav1.ConditionTrue)).
					Should(Succeed())
				g.Expect(checkCondition(bucket.Status.Conditions, BucketConfigMapReady, metav1.ConditionFalse)).
					Should(Succeed())

				cmCondition := meta.FindStatusCondition(bucket.Status.Conditions, BucketConfigMapReady)
				Expect(cmCondition.Reason).To(Equal(ReasonConfigMapNameConflict))
			}).Should(Succeed())

			By("top-level Ready condition reflects ConfigMap issue")
			readyCondition := meta.FindStatusCondition(bucket.Status.Conditions, Ready)
			Expect(readyCondition).ToNot(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse), "top-level Ready must be false")
			Expect(readyCondition.Reason).To(Equal(ReasonConfigMapNameConflict), "should reflect underlying configuration conflict")
		})

		It("recovers from conflict once configMapName is set", func() {
			conflictingName := "foobar"
			originalData := map[string]string{"foo": "bar"}
			By("existing ConfigMap with name")
			existingCM := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      conflictingName,
					Namespace: namespace,
				},
				Data: originalData,
			}
			Expect(k8sClient.Create(ctx, &existingCM)).Should(Succeed())

			By("bucket sharing configmap resource name")
			bucket := newBucket(namespace)
			bucket.Name = conflictingName
			Expect(k8sClient.Create(ctx, &bucket)).Should(Succeed())

			sut, _ := setupBucket()
			shouldReconcile(sut, bucket.ObjectMeta)

			By("bucket is not ready")
			_ = k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)
			Expect(checkCondition(bucket.Status.Conditions, Ready, metav1.ConditionFalse)).Should(Succeed())

			By("change CM name in bucket spec")
			bucket.Spec.ConfigMapName = conflictingName + "-bucket-config"
			Expect(k8sClient.Update(ctx, &bucket)).Should(Succeed())
			shouldReconcile(sut, bucket.ObjectMeta)

			By("bucket recovers to Ready")
			_ = k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)
			Expect(checkCondition(bucket.Status.Conditions, Ready, metav1.ConditionTrue)).Should(Succeed())

			By("new ConfigMap for Bucket created")
			var bucketConfig corev1.ConfigMap
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: namespace, Name: bucket.Spec.ConfigMapName}, &bucketConfig)).
				Should(Succeed())

			By("original conflicting ConfigMap is not deleted")
			Expect(k8sClient.Get(ctx, namespacedName(existingCM.ObjectMeta), &existingCM)).
				Should(Succeed(), "should not delete unowned CM")
			Expect(existingCM.Data).To(Equal(originalData))
		})
	})

	Context("When reconciling existing buckets", func() {
		It("should successfully reconcile for existing external resource", func() {
			bucket := newBucket(namespace)
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &bucket)
			})

			sut, s3API := setupBucket()

			By("simulating reconcile error after external bucket creation")
			existing := s3.Bucket{
				ID:            "3f5z",
				GlobalAliases: []string{suffixedResourceName(bucket.Spec.Name, bucket.ObjectMeta)},
			}
			s3API.buckets[existing.ID] = existing

			By("reconciling the created resource")
			bucketObjID := namespacedName(bucket.ObjectMeta)
			_, err := sut.Reconcile(ctx, reconcile.Request{
				NamespacedName: bucketObjID,
			})
			Expect(err).NotTo(HaveOccurred())

			By("storing resource condition and generation in status")
			Eventually(func(g Gomega) {
				var bucket garagev1alpha1.Bucket
				g.Expect(k8sClient.Get(ctx, bucketObjID, &bucket)).To(Succeed())

				bucketCond := meta.FindStatusCondition(bucket.Status.Conditions, BucketReady)
				g.Expect(bucketCond).ToNot(BeNil())
				g.Expect(bucketCond.Status).To(Equal(metav1.ConditionTrue), "bucket should be ready")

				readyCond := meta.FindStatusCondition(bucket.Status.Conditions, Ready)
				g.Expect(readyCond).ToNot(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))

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
			initial := newBucket(namespace)

			Expect(k8sClient.Create(ctx, &initial)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &initial)
			})

			sut, s3API := setupBucket()

			By("reconciling")
			objID := namespacedName(initial.ObjectMeta)
			_, _ = sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})

			By("setting bucket quota")
			var bucket garagev1alpha1.Bucket
			Expect(k8sClient.Get(ctx, objID, &bucket)).To(Succeed())

			bucket.Spec.MaxSize = resource.MustParse("9500k")
			bucket.Spec.MaxObjects = 900
			Expect(k8sClient.Update(ctx, &bucket)).To(Succeed())

			// act
			_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ShouldNot(HaveOccurred())

			// assert
			quota := s3API.buckets[bucket.Status.BucketID].Quotas
			Expect(quota.MaxSize).To(Equal(bucket.Spec.MaxSize.Value()))
			Expect(quota.MaxObjects).To(Equal(bucket.Spec.MaxObjects))
		})
		It("recreates missing ConfigMap on reconcile", func() {
			bucket := newBucket(namespace)
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			By("reconciling initial create")
			sut, _ := setupBucket()
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
			// TODO: is resource name or spec.Name being validated here? Both?
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

func newBucket(namespace string) garagev1alpha1.Bucket {
	return garagev1alpha1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fixture.RandAlpha(8),
			Namespace: namespace,
		},
		Spec: garagev1alpha1.BucketSpec{
			Name: fixture.RandAlpha(8),
		},
	}
}

// shouldReconcile ensures that Reconcile() did not return an error.
func shouldReconcile(controller *BucketReconciler, obj metav1.ObjectMeta) {
	Expect(controller.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName(obj)})).
		Error().ToNot(HaveOccurred())
}

func setupBucket() (*BucketReconciler, *s3APIFake) {
	s3API := newS3APIFake()
	sut := NewBucketReconciler(k8sClient, k8sClient.Scheme(), s3API, "https://s3.foo/bar:123")
	return sut, s3API
}

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
