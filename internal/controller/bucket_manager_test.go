// /*
// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/bmarinov/garage-storage-controller/internal/tests/fixture"
)

var _ = Describe("Bucket controller manager", Ordered, func() {
	var (
		mCtx              context.Context
		cancel            context.CancelFunc
		s3Fake            *s3APIFake
		permissionsClient *permissionClientFake
		namespace         string
	)

	BeforeAll(func() {
		namespace = fixture.RandAlpha(10)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())

		s3Fake = newS3APIFake()
		permissionsClient = newPermissionClientFake()
		mCtx, cancel = context.WithCancel(context.Background())

		skipValidation := true
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:                 clientgoscheme.Scheme,
			Metrics:                metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
			Controller:             config.Controller{SkipNameValidation: &skipValidation},
		})
		Expect(err).ToNot(HaveOccurred())

		r := NewBucketReconciler(
			mgr.GetClient(),
			mgr.GetScheme(),
			s3Fake,
			"http://s3.bar.com",
			permissionsClient,
			mgr.GetEventRecorderFor("garage-bucket-controller"),
		)
		r.baseRequeueInterval = time.Second
		Expect(r.SetupWithManager(mgr)).To(Succeed())

		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mCtx)).To(Succeed())
		}()
	})

	AfterAll(func() {
		cancel()
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("existing bucket eventually Ready after owner permissions are fixed", func() {
		existingBucketID := fixture.RandAlpha(12)
		ownerKeyID := fixture.RandAlpha(12)
		garageBucketAlias := fixture.RandAlpha(12)

		s3Fake.Seed(existingBucketID, s3.Bucket{
			ID:            existingBucketID,
			GlobalAliases: []string{garageBucketAlias},
		})

		By("creating a Secret with insufficient permissions")
		secretName := fixture.RandAlpha(8)
		Expect(permissionsClient.SetPermissions(ctx, ownerKeyID, existingBucketID,
			s3.Permissions{Owner: false, Read: true})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{
				SecretKeyAccessKeyID:     []byte(ownerKeyID),
				SecretKeySecretAccessKey: []byte(fixture.RandAlpha(12)),
			},
		})).To(Succeed())

		By("creating Bucket CR for an existing Garage bucket")
		bucket := garagev1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fixture.RandAlpha(8),
				Namespace: namespace,
			},
			Spec: garagev1alpha1.BucketSpec{
				ExistingBucket: &garagev1alpha1.ExistingBucketSpec{
					Name:           garageBucketAlias,
					OwnerKeySecret: secretName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

		By("Bucket is not Ready due to insufficient permissions")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
			g.Expect(checkCondition(bucket.Status.Conditions, BucketReady, metav1.ConditionFalse)).To(Succeed())
			g.Expect(meta.FindStatusCondition(bucket.Status.Conditions, BucketReady).Reason).
				To(Equal(ReasonOwnershipVerificationFailed))
		}).Should(Succeed())

		By("granting owner permissions")
		Expect(permissionsClient.SetPermissions(ctx, ownerKeyID, existingBucketID,
			s3.Permissions{Owner: true, Read: true, Write: true})).To(Succeed())

		By("Bucket eventually becomes Ready")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
			g.Expect(checkCondition(bucket.Status.Conditions, Ready, metav1.ConditionTrue)).To(Succeed())
		}, "20s").Should(Succeed())
	})

	It("existing bucket eventually Ready after missing owner key Secret is created", func() {
		existingBucketID := fixture.RandAlpha(12)
		ownerKeyID := fixture.RandAlpha(12)
		const garageBucketAlias = "garage-bucket-mgr-test"

		s3Fake.Seed(existingBucketID, s3.Bucket{
			ID:            existingBucketID,
			GlobalAliases: []string{garageBucketAlias},
		})

		By("creating Bucket CR for an existing Garage bucket")
		secretName := fixture.RandAlpha(8)
		bucket := garagev1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fixture.RandAlpha(8),
				Namespace: namespace,
			},
			Spec: garagev1alpha1.BucketSpec{
				ExistingBucket: &garagev1alpha1.ExistingBucketSpec{
					Name:           garageBucketAlias,
					OwnerKeySecret: secretName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &bucket)).To(Succeed())

		By("Bucket is not Ready due to missing Secret")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
			g.Expect(checkCondition(bucket.Status.Conditions, BucketReady, metav1.ConditionFalse)).To(Succeed())
			g.Expect(meta.FindStatusCondition(bucket.Status.Conditions, BucketReady).Reason).
				To(Equal(ReasonOwnerKeySecretNotFound))
		}).Should(Succeed())

		By("creating the Secret")
		Expect(permissionsClient.SetPermissions(ctx, ownerKeyID, existingBucketID,
			s3.Permissions{Owner: true, Read: true, Write: true})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Data: map[string][]byte{
				SecretKeyAccessKeyID:     []byte(ownerKeyID),
				SecretKeySecretAccessKey: []byte(fixture.RandAlpha(12)),
			},
		})).To(Succeed())

		// TODO: reduce wait duration, configurable retry-after period:
		By("Bucket eventually becomes Ready")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName(bucket.ObjectMeta), &bucket)).To(Succeed())
			g.Expect(checkCondition(bucket.Status.Conditions, Ready, metav1.ConditionTrue)).To(Succeed())
		}, "40s").Should(Succeed())
	})
})
