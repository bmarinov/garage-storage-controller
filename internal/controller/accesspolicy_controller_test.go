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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/bmarinov/garage-storage-controller/internal/tests/fixture"
)

var _ = Describe("AccessPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		// the namespace for the test
		var namespace string

		BeforeEach(func() {
			By("setting up namespace")
			namespace = fixture.RandAlpha(10)

			testNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, &testNamespace)).To(Succeed())
		})

		AfterEach(func() {
			ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Delete(ctx, &ns)).To(Succeed())
		})

		DescribeTable("missing resource dependencies", func(
			bucketExists, keyExists bool,
			expectedReason string,
		) {
			bucketName := "bucket-a"
			accessKeyName := "key-b"

			if bucketExists {
				b := garagev1alpha1.Bucket{
					ObjectMeta: metav1.ObjectMeta{Name: bucketName, Namespace: namespace},
					Spec:       garagev1alpha1.BucketSpec{Name: "blap-bucket3132"},
				}
				Expect(k8sClient.Create(ctx, &b)).To(Succeed())
				markBucketReady(&b)
				updateBucketReadyCondition(&b)
				Expect(k8sClient.Status().Patch(ctx, &b, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())
			}
			if keyExists {
				k := garagev1alpha1.AccessKey{
					ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
					Spec:       garagev1alpha1.AccessKeySpec{SecretName: "zzz-ns-secret"},
				}
				Expect(k8sClient.Create(ctx, &k)).To(Succeed())

				markAccessKeyReady(&k)
				markSecretReady(&k)
				updateAccessKeyCondition(&k)
				Expect(k8sClient.Status().Patch(ctx, &k, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())

			}

			p := garagev1alpha1.AccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testpolicy",
					Namespace: namespace,
				},
				Spec: garagev1alpha1.AccessPolicySpec{
					AccessKey: accessKeyName,
					Bucket:    bucketName,
					Permissions: garagev1alpha1.Permissions{
						Read: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &p)).To(Succeed())
			objID := types.NamespacedName{Namespace: namespace, Name: p.Name}

			sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), &permissionClientStub{})
			_, err := sut.Reconcile(ctx,
				reconcile.Request{NamespacedName: objID})
			Expect(err).ToNot(HaveOccurred(), "should reschedule and not err")

			var policy garagev1alpha1.AccessPolicy
			_ = k8sClient.Get(ctx, objID, &policy)

			readyCond := meta.FindStatusCondition(policy.Status.Conditions, Ready)
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse), "Ready condition has unexpected status")
			Expect(readyCond.Reason).To(Equal(expectedReason), "Specific reason should propagate")
		},
			Entry("bucket missing", false, true, ReasonBucketMissing),
			Entry("key missing", true, false, ReasonAccessKeyMissing),
			Entry("bucket and key missing", false, false, ReasonBucketMissing),
		)
		DescribeTable("Ready state in dependencies", func(
			bucketReady, keyReady bool,
			expectedStatus metav1.ConditionStatus,
		) {
			bucketName := "foo-bucket-123"
			accessKeyName := "read-write-key-123"

			By("creating upstream resources")
			b := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: bucketName, Namespace: namespace},
				Spec:       garagev1alpha1.BucketSpec{Name: "blap-bucket3132"},
			}
			Expect(k8sClient.Create(ctx, &b)).To(Succeed())

			if bucketReady {
				markBucketReady(&b)
			} else {
				markBucketNotReady(&b, "FooUnknown", "Not ready in test")
			}
			updateBucketReadyCondition(&b)
			Expect(k8sClient.Status().Patch(ctx, &b, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())

			k := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
				Spec:       garagev1alpha1.AccessKeySpec{SecretName: "zzz-ns-secret"},
			}
			Expect(k8sClient.Create(ctx, &k)).To(Succeed())

			if keyReady {
				markAccessKeyReady(&k)
				markSecretReady(&k)
			} else {
				markAccessKeyNotReady(&k, "Unknown", "Foo bar test")
			}
			updateAccessKeyCondition(&k)
			Expect(k8sClient.Status().Patch(ctx, &k, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())

			By("create and reconcile policy")

			p := garagev1alpha1.AccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testpolicy",
					Namespace: namespace,
				},
				Spec: garagev1alpha1.AccessPolicySpec{
					AccessKey: accessKeyName,
					Bucket:    bucketName,
					Permissions: garagev1alpha1.Permissions{
						Read:  true,
						Write: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &p)).To(Succeed())
			objID := types.NamespacedName{Name: p.Name,
				Namespace: p.Namespace}

			sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), &permissionClientStub{})
			_, err := sut.Reconcile(ctx,
				reconcile.Request{NamespacedName: objID})
			Expect(err).ToNot(HaveOccurred())

			var reconciled garagev1alpha1.AccessPolicy
			_ = k8sClient.Get(ctx, objID, &reconciled)

			readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
			Expect(readyCond.Status).To(Equal(expectedStatus))
		},
			Entry("bucket not ready", false, true, metav1.ConditionFalse),
			Entry("access key not ready", true, false, metav1.ConditionFalse),
			Entry("bucket and access key ready", true, true, metav1.ConditionTrue),
		)

		It("should reconcile with dependencies ready", func() {
			By("creating dependencies")
			bucketName := "bucket-foo"
			accessKeyName := "key-foo-bar"

			bucketRes := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: bucketName, Namespace: namespace},
				Spec:       garagev1alpha1.BucketSpec{Name: "blap-bucket3132"},
			}
			Expect(k8sClient.Create(ctx, &bucketRes)).To(Succeed())
			markBucketReady(&bucketRes)
			updateBucketReadyCondition(&bucketRes)
			Expect(k8sClient.Status().Patch(ctx, &bucketRes, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())
			keyRes := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
				Spec:       garagev1alpha1.AccessKeySpec{SecretName: "zzz-ns-secret"},
			}
			Expect(k8sClient.Create(ctx, &keyRes)).To(Succeed())

			markAccessKeyReady(&keyRes)
			markSecretReady(&keyRes)
			updateAccessKeyCondition(&keyRes)
			Expect(k8sClient.Status().Patch(ctx, &keyRes, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())

			By("creating a referencing policy")
			policy := garagev1alpha1.AccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testpolicy",
					Namespace: namespace,
				},
				Spec: garagev1alpha1.AccessPolicySpec{
					AccessKey: accessKeyName,
					Bucket:    bucketName,
					Permissions: garagev1alpha1.Permissions{
						Read: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &policy)).To(Succeed())

			By("reconciling policy")
			sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), &permissionClientStub{})
			objID := types.NamespacedName{
				Namespace: policy.Namespace,
				Name:      policy.Name,
			}

			_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
			Expect(err).ToNot(HaveOccurred())

			By("setting access policy and top-level Ready status")
			var reconciled garagev1alpha1.AccessPolicy
			_ = k8sClient.Get(ctx, objID, &reconciled)

			policyCond := meta.FindStatusCondition(reconciled.Status.Conditions, PolicyAssignmentReady)
			Expect(policyCond.Status).To(Equal(metav1.ConditionTrue))
			readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.ObservedGeneration).To(Equal(reconciled.GetGeneration()),
				"status and object generation should be equal")
		})
	})
})

type permissionClientStub struct {
}

// SetPermissions implements PermissionClient.
func (p *permissionClientStub) SetPermissions(ctx context.Context, keyID string, bucketID string, permissions s3.Permissions) error {

	return nil
}

var _ PermissionClient = &permissionClientStub{}
