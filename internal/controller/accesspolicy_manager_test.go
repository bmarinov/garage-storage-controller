package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
	"github.com/bmarinov/garage-storage-controller/internal/s3"
	"github.com/bmarinov/garage-storage-controller/internal/tests/fixture"
)

var _ = Describe("AccessPolicy controller manager", Ordered, func() {
	var (
		mCtx      context.Context // context for the controller manager
		cancel    context.CancelFunc
		apiClient *permissionClientFake
		namespace string
	)

	BeforeAll(func() {
		namespace = fixture.RandAlpha(10)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())

		apiClient = newPermissionClientFake()
		mCtx, cancel = context.WithCancel(context.Background())

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:                 clientgoscheme.Scheme,
			Metrics:                metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		// sync with manager setup in main:
		Expect(NewBucketReconciler(mgr.GetClient(), mgr.GetScheme(), newS3APIFake(), "http://s3.test.foo").
			SetupWithManager(mgr)).To(Succeed())
		Expect(NewAccessKeyReconciler(mgr.GetClient(), mgr.GetScheme(), newAccessMgrFake()).
			SetupWithManager(mgr)).To(Succeed())
		Expect(NewAccessPolicyReconciler(mgr.GetClient(), mgr.GetScheme(), apiClient).
			SetupWithManager(mgr)).To(Succeed())

		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mCtx)).To(Succeed())
		}()
	})

	AfterAll(func() {
		cancel()

		// strip finalizers to fix namespace del hanging:
		var policies garagev1alpha1.AccessPolicyList
		_ = k8sClient.List(ctx, &policies, client.InNamespace(namespace))
		for i := range policies.Items {
			p := &policies.Items[i]
			p.Finalizers = nil
			_ = k8sClient.Update(ctx, p)
		}
		var keys garagev1alpha1.AccessKeyList
		_ = k8sClient.List(ctx, &keys, client.InNamespace(namespace))
		for i := range keys.Items {
			k := &keys.Items[i]
			k.Finalizers = nil
			_ = k8sClient.Update(ctx, k)
		}
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("all resources eventually Ready", func() {
		By("creating dependencies")
		b := newBucket(namespace)
		Expect(k8sClient.Create(ctx, &b)).To(Succeed())

		key := garagev1alpha1.AccessKey{
			ObjectMeta: metav1.ObjectMeta{Name: fixture.RandAlpha(8), Namespace: namespace},
			Spec:       garagev1alpha1.AccessKeySpec{SecretName: fixture.RandAlpha(8)},
		}
		Expect(k8sClient.Create(ctx, &key)).To(Succeed())

		By("creating policy")
		policy := garagev1alpha1.AccessPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: fixture.RandAlpha(8), Namespace: namespace},
			Spec: garagev1alpha1.AccessPolicySpec{
				AccessKey:   key.Name,
				Bucket:      b.Name,
				Permissions: garagev1alpha1.Permissions{Read: true},
			},
		}
		Expect(k8sClient.Create(ctx, &policy)).To(Succeed())

		By("waiting for all resources to reach Ready cond")
		Eventually(func(g Gomega) {
			var reconciled garagev1alpha1.AccessPolicy
			g.Expect(k8sClient.Get(ctx, namespacedName(policy.ObjectMeta), &reconciled)).To(Succeed())
			readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		}, "30s").Should(Succeed())
	})

	It("revokes permissions and marks not ready when bucket is deleted", func() {
		By("creating dependencies")
		b := newBucket(namespace)
		Expect(k8sClient.Create(ctx, &b)).To(Succeed())

		key := garagev1alpha1.AccessKey{
			ObjectMeta: metav1.ObjectMeta{Name: fixture.RandAlpha(8), Namespace: namespace},
			Spec:       garagev1alpha1.AccessKeySpec{SecretName: fixture.RandAlpha(8)},
		}
		Expect(k8sClient.Create(ctx, &key)).To(Succeed())

		By("creating policy")
		policy := garagev1alpha1.AccessPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: fixture.RandAlpha(8), Namespace: namespace},
			Spec: garagev1alpha1.AccessPolicySpec{
				AccessKey:   key.Name,
				Bucket:      b.Name,
				Permissions: garagev1alpha1.Permissions{Read: true, Write: true},
			},
		}
		Expect(k8sClient.Create(ctx, &policy)).To(Succeed())

		By("waiting for policy to reach Ready")
		var readyPolicy garagev1alpha1.AccessPolicy
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, namespacedName(policy.ObjectMeta), &readyPolicy)).To(Succeed())
			readyCond := meta.FindStatusCondition(readyPolicy.Status.Conditions, Ready)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		}, "30s").Should(Succeed())

		By("deleting the Bucket resource")
		Expect(k8sClient.Delete(ctx, &b)).To(Succeed())

		By("policy transitions to not ready")
		Eventually(func(g Gomega) {
			var reconciled garagev1alpha1.AccessPolicy
			g.Expect(k8sClient.Get(ctx, namespacedName(policy.ObjectMeta), &reconciled)).To(Succeed())
			readyCond := meta.FindStatusCondition(reconciled.Status.Conditions, Ready)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(readyCond.Reason).To(Equal(ReasonBucketMissing))
		}, "15s").Should(Succeed())

		By("permissions revoked on Garage")
		permKey := fmt.Sprintf("%s:%s", readyPolicy.Status.AccessKeyID, readyPolicy.Status.BucketID)
		Expect(apiClient.assignedPermissions[permKey]).To(Equal(s3.Permissions{}))
	})
})
