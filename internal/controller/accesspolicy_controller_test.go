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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
)

var _ = Describe("AccessPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		accessKeyObjID := types.NamespacedName{
			Name:      "foo-key",
			Namespace: "default",
		}
		bucketObjID := types.NamespacedName{
			Name:      "foo-bucket",
			Namespace: "default",
		}

		policyObjID := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		accesspolicy := &garagev1alpha1.AccessPolicy{}

		BeforeEach(func() {
			By("creating the base custom resources")

			key := garagev1alpha1.AccessKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      accessKeyObjID.Name,
					Namespace: accessKeyObjID.Namespace,
				},
				Spec: garagev1alpha1.AccessKeySpec{
					SecretName: "blap-secret",
				},
			}
			bucket := garagev1alpha1.Bucket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bucketObjID.Name,
					Namespace: bucketObjID.Namespace,
				},
				Spec: garagev1alpha1.BucketSpec{
					MaxSize: 353,
					Name:    "bucketname-docs",
				},
			}
			Expect(k8sClient.Create(ctx, &key)).To(Succeed())
			Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

			err := k8sClient.Get(ctx, policyObjID, accesspolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &garagev1alpha1.AccessPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: garagev1alpha1.AccessPolicySpec{
						AccessKey: key.Name,
						Bucket:    bucket.Name,
						Permissions: garagev1alpha1.Permissions{
							Read:  true,
							Write: true,
							Owner: false,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &garagev1alpha1.AccessPolicy{}
			err := k8sClient.Get(ctx, policyObjID, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instances")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// TODO: clean up:
			var bucket garagev1alpha1.Bucket
			Expect(k8sClient.Get(ctx, bucketObjID, &bucket)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, &bucket)).To(Succeed())

			var accessKey garagev1alpha1.AccessKey
			Expect(k8sClient.Get(ctx, accessKeyObjID, &accessKey)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, &accessKey)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Bucket and key status Ready")
			var bucket garagev1alpha1.Bucket
			Expect(k8sClient.Get(ctx, bucketObjID, &bucket)).To(Succeed())

			b := Bucket{Object: &bucket}
			b.MarkBucketReady()

			var accessKey garagev1alpha1.AccessKey
			Expect(k8sClient.Get(ctx, accessKeyObjID, &accessKey)).To(Succeed())
			k := AccessKey{Object: &accessKey}
			k.MarkAccessKeyReady()

			By("Reconciling the policy resource")
			controllerReconciler := &AccessPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: policyObjID,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Policy is Ready")
			var policy garagev1alpha1.AccessPolicy
			Expect(k8sClient.Get(ctx, policyObjID, &policy)).To(Succeed())
			policyReady := meta.FindStatusCondition(policy.Status.Conditions, Ready)
			Expect(policyReady.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
