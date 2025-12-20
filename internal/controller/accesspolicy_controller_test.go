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

			sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), newPermissionClientFake())

			Eventually(func(g Gomega) {
				_, err := sut.Reconcile(ctx,
					reconcile.Request{NamespacedName: objID})
				g.Expect(err).ToNot(HaveOccurred(), "should reschedule and not err")
				var policy garagev1alpha1.AccessPolicy
				_ = k8sClient.Get(ctx, objID, &policy)

				readyCond := meta.FindStatusCondition(policy.Status.Conditions, Ready)
				g.Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse), "Ready condition has unexpected status")
				Expect(readyCond.Reason).To(Equal(expectedReason), "Specific reason should propagate")
			}).Should(Succeed())
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

			By("creating policy")
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

			By("reconciling reaches ready status")
			Eventually(func(g Gomega) {
				sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), newPermissionClientFake())
				_, err := sut.Reconcile(ctx,
					reconcile.Request{NamespacedName: objID})
				g.Expect(err).ToNot(HaveOccurred())

				var reconciled garagev1alpha1.AccessPolicy
				_ = k8sClient.Get(ctx, objID, &reconciled)

				readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
				g.Expect(readyCond).ToNot(BeNil())
				g.Expect(readyCond.Status).To(Equal(expectedStatus))
			}).Should(Succeed())
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
			// set status fields to fake readiness
			bucketRes.Status.BucketID = fixture.RandAlpha(12)
			Expect(k8sClient.Status().Patch(ctx, &bucketRes, client.Merge, client.FieldOwner(bucketControllerName))).To(Succeed())

			keyRes := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
				Spec:       garagev1alpha1.AccessKeySpec{SecretName: "zzz-ns-secret"},
			}
			Expect(k8sClient.Create(ctx, &keyRes)).To(Succeed())

			markAccessKeyReady(&keyRes)
			markSecretReady(&keyRes)
			// set status fields to fake readiness
			keyRes.Status.AccessKeyID = fixture.RandAlpha(12)
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

			sut := NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), newPermissionClientFake())
			objID := types.NamespacedName{
				Namespace: policy.Namespace,
				Name:      policy.Name,
			}

			By("reconciling sets policy and top-level Ready status")
			var reconciled garagev1alpha1.AccessPolicy
			Eventually(func(g Gomega) {
				_, err := sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})
				g.Expect(err).ToNot(HaveOccurred())

				_ = k8sClient.Get(ctx, objID, &reconciled)

				policyCond := meta.FindStatusCondition(reconciled.Status.Conditions, PolicyAssignmentReady)
				g.Expect(policyCond).ToNot(BeNil())
				g.Expect(policyCond.Status).To(Equal(metav1.ConditionTrue))

				readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
				g.Expect(readyCond).ToNot(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCond.ObservedGeneration).To(Equal(reconciled.GetGeneration()),
					"status and object generation should be equal")
			}).Should(Succeed())

			By("storing external resource IDs in status")
			Expect(reconciled.Status.BucketID).ToNot(BeEmpty())
			Expect(reconciled.Status.AccessKeyID).ToNot(BeEmpty())

			By("storing name of accesskey resource in policy label")
			Expect(reconciled.Labels[accesskeyLabel]).To(Equal(reconciled.Spec.AccessKey))
		})

		It("should remove access grant on deletion", func() {
			bucketName := fixture.RandAlpha(12)
			accessKeyName := fixture.RandAlpha(12)
			b := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: bucketName, Namespace: namespace},
				Spec:       garagev1alpha1.BucketSpec{Name: "blap-bucket3132"},
			}
			_ = k8sClient.Create(ctx, &b)

			markBucketReady(&b)
			updateBucketReadyCondition(&b)
			_ = k8sClient.Status().Patch(ctx, &b, client.Merge, client.FieldOwner(bucketControllerName))
			k := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
				Spec:       garagev1alpha1.AccessKeySpec{SecretName: "zzz-ns-secret"},
			}
			_ = k8sClient.Create(ctx, &k)

			markAccessKeyReady(&k)
			markSecretReady(&k)
			updateAccessKeyCondition(&k)
			_ = k8sClient.Status().Patch(ctx, &k, client.Merge, client.FieldOwner(bucketControllerName))

			By("creating access policy for bucket")
			policy := garagev1alpha1.AccessPolicy{
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
						Owner: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &policy)).To(Succeed())

			sut, apiClient := setupPolicyTest()
			objID := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}

			By("reconciling and setting external permissions")
			Eventually(func(g Gomega) {
				_, _ = sut.Reconcile(ctx,
					reconcile.Request{NamespacedName: objID})

				g.Expect(apiClient.assignedPermissions).To(HaveLen(1))
			}).Should(Succeed())

			By("deleting AccessPolicy")
			Expect(k8sClient.Delete(ctx, &policy)).To(Succeed())
			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, objID, &policy)).To(Succeed())
				return !policy.DeletionTimestamp.IsZero()
			}).Should(BeTrue(), "should have deletion timestamp")

			By("reconciling removes external access")
			_, _ = sut.Reconcile(ctx, reconcile.Request{NamespacedName: objID})

			Expect(apiClient.assignedPermissions[fmt.Sprintf("%s:%s", k.Status.AccessKeyID, b.Status.BucketID)]).
				To(Equal(s3.Permissions{Owner: false, Read: false, Write: false}))
		})
		It("updates policy status on dependency deletion", func() {
			By("creating dependencies")
			bucketName := fixture.RandAlpha(12)
			accessKeyName := fixture.RandAlpha(12)
			b := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: bucketName, Namespace: namespace},
				Spec:       garagev1alpha1.BucketSpec{Name: fixture.RandAlpha(8)},
			}
			_ = k8sClient.Create(ctx, &b)

			markBucketReady(&b)
			updateBucketReadyCondition(&b)
			_ = k8sClient.Status().Patch(ctx, &b, client.Merge, client.FieldOwner(bucketControllerName))
			key := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{Name: accessKeyName, Namespace: namespace},
				Spec:       garagev1alpha1.AccessKeySpec{SecretName: fixture.RandAlpha(8)},
			}
			_ = k8sClient.Create(ctx, &key)

			markAccessKeyReady(&key)
			markSecretReady(&key)
			updateAccessKeyCondition(&key)
			_ = k8sClient.Status().Patch(ctx, &key, client.Merge, client.FieldOwner(bucketControllerName))

			By("creating policy")
			policy := garagev1alpha1.AccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.RandAlpha(8),
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

			sut, _ := setupPolicyTest()
			objID := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}

			By("reconciling")
			Eventually(func(g Gomega) {
				_, err := sut.Reconcile(ctx,
					reconcile.Request{NamespacedName: objID})
				g.Expect(err).ToNot(HaveOccurred())

				var reconciled garagev1alpha1.AccessPolicy
				_ = k8sClient.Get(ctx, objID, &reconciled)

				readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
				g.Expect(readyCond).ToNot(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}).Should(Succeed())

			By("deleting AccessKey")

			Expect(k8sClient.Delete(ctx, &key)).To(Succeed())

			_, _ = sut.Reconcile(ctx,
				reconcile.Request{NamespacedName: objID})

			_ = k8sClient.Get(ctx, objID, &policy)
			topReady := meta.FindStatusCondition(policy.Status.Conditions, Ready)
			Expect(topReady.Status).To(Equal(metav1.ConditionFalse), "should detect AccessKey change")

			// todo: ensure access revocation on Garage?
		})
	})
})

func setupPolicyTest() (*AccessPolicyReconciler, *permissionClientFake) {
	apiClient := newPermissionClientFake()
	return NewAccessPolicyReconciler(k8sClient, k8sClient.Scheme(), apiClient), apiClient
}

type permissionClientFake struct {
	assignedPermissions map[string]s3.Permissions
}

func newPermissionClientFake() *permissionClientFake {
	return &permissionClientFake{
		assignedPermissions: make(map[string]s3.Permissions),
	}
}

// SetPermissions implements PermissionClient.
func (p *permissionClientFake) SetPermissions(ctx context.Context, keyID string, bucketID string, permissions s3.Permissions) error {
	key := fmt.Sprintf("%s:%s", keyID, bucketID)
	p.assignedPermissions[key] = permissions
	return nil
}

var _ PermissionClient = &permissionClientFake{}
