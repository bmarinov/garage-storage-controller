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

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bmarinov/garage-storage-controller/internal/garage/integrationtests"
	"github.com/bmarinov/garage-storage-controller/internal/tests"
	"github.com/bmarinov/garage-storage-controller/test/e2e/utils"
)

var wellKnown = struct {
	accessKeyName    string
	secretName       string
	bucketName       string
	configMapName    string
	accessPolicyName string
}{
	// Keep in sync with "../../config/samples/"
	accessKeyName:    "accesskey-sample",
	secretName:       "foo-some-secret",
	bucketName:       "bucket-sample",
	configMapName:    "bucket-sample",
	accessPolicyName: "accesspolicy-sample",
}

// namespace where the project is deployed in
const namespace = "garage-storage-controller-system"

// serviceAccountName created for the project
const serviceAccountName = "garage-storage-controller-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "garage-storage-controller-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "garage-storage-controller-metrics-binding"

// garageAPISecret contains the token for the admin Garage API.
const garageAPISecret = "garage-api-token"

// garageRPCSecret contains the rpc_secret for Garage.
const garageRPCSecret = "garage-rpc-secret"

// garageSecretKey is the key in the secret pointing to the admin token.
const garageSecretKey = "api-token"

// garageNamespace contains the Garage deployment resources.
const garageNamespace = "garage-system"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	BeforeAll(func() {
		By("creating garage namespace")
		cmd := exec.Command("kubectl", "create", "ns", garageNamespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("creating manager namespace")
		cmd = exec.Command("kubectl", "create", "ns", namespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("creating admin API secret in controller and garage namespaces")
		garageAdminToken := integrationtests.GenerateRandomString(base64.StdEncoding.EncodeToString)

		for _, ns := range []string{garageNamespace, namespace} {
			cmd = exec.Command("kubectl", "create", "secret", "generic", garageAPISecret,
				fmt.Sprintf("--from-literal=%s=%s", garageSecretKey, garageAdminToken),
				"--namespace", ns,
			)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		}

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("creating Garage secret")
		cmd = exec.Command("kubectl", "create", "secret", "generic", garageRPCSecret,
			fmt.Sprintf("--from-literal=%s=%s", "secret", integrationtests.GenerateRandomString(hex.EncodeToString)),
			"--namespace", garageNamespace,
		)
		Expect(cmd.Run()).To(Succeed())

		By("deploying garage and controller")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the garage controller")

		By("waiting for Garage startup")
		garagePod := "garage-storage-controller-garage-0"
		Eventually(func() error {
			cmd := exec.Command("kubectl", "wait",
				"--for=condition=ready",
				fmt.Sprintf("pod/%s", garagePod),
				"-n", garageNamespace,
				"--timeout=3s")
			_, err := utils.Run(cmd)
			return err
		}, 25*time.Second, time.Second*1).Should(Succeed())

		By("initializing garage cluster")
		err = tests.InitGarageLayout(context.TODO(), NamespacePodExec(garageNamespace, garagePod))
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("removing finalizers")
		removeFinalizers("accesspolicies", "accesskeys", "buckets")

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()

		By("fetching controller logs")
		cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
		controllerLogs, err := utils.Run(cmd)

		if err == nil {
			if strings.Contains(controllerLogs, "\tERROR\t") {
				_, _ = fmt.Fprintf(GinkgoWriter, "ERROR found in controller logs:\n%s\n", controllerLogs)
				Fail("Controller logged ERROR during test")
			} else if specReport.Failed() {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			}
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
		}

		if specReport.Failed() {
			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=garage-storage-controller-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})
	})

	Context("Custom resources", Ordered, func() {

		It("should deploy external resources", func() {

			// TODO: use separate manifests for the e2e tests

			By("creating AccessKey API resource")
			cmd := exec.Command("kubectl", "apply", "-f", "config/samples/garage_v1alpha1_accesskey.yaml")
			_, err := utils.Run(cmd)
			Expect(err).To(Succeed())

			By("waiting for AccessKey Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl",
					"wait", "accesskey", wellKnown.accessKeyName, "--for=condition=Ready", "--timeout=1s")
				g.Expect(cmd.Run()).To(Succeed())
			}, 5*time.Second).Should(Succeed())

			By("namespace secret created")
			cmd = exec.Command("kubectl", "get", "secret", "-n", "default",
				wellKnown.secretName)
			Expect(cmd.Run()).Should(Succeed())

			By("creating Bucket API resource")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/garage_v1alpha1_bucket.yaml")
			_, err = utils.Run(cmd)
			Expect(err).To(Succeed())

			By("waiting for Bucket ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl",
					"wait", "bucket", wellKnown.bucketName, "--for=condition=Ready", "--timeout=1s")
				g.Expect(cmd.Run()).To(Succeed())
			}).Should(Succeed())

			By("configmap for bucket created")
			cmd = exec.Command("kubectl", "get", "cm", "-n", "default",
				wellKnown.configMapName)
			Expect(cmd.Run()).To(Succeed())

			By("creating AccessPolicy resource")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/garage_v1alpha1_accesspolicy.yaml")
			_, err = utils.Run(cmd)
			Expect(err).To(Succeed())

			By("waiting for AccessPolicy Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl",
					"wait", "accesspolicy", wellKnown.accessPolicyName, "--for=condition=Ready", "--timeout=1s")
				g.Expect(cmd.Run()).To(Succeed())
			}).Should(Succeed())
		})

		// test depends on previous step, overlay -> new resource?
		It("recreates missing cm for existing buckets", func() {
			By("storing original ConfigMap UID")
			cmd := exec.Command("kubectl", "get", "cm", wellKnown.configMapName,
				"-n", "default", "-o", "jsonpath={.metadata.uid}")
			originalUID, err := utils.Run(cmd)
			Expect(err).ToNot(HaveOccurred())

			By("deleting cm")
			cmd = exec.Command("kubectl", "delete", "cm", "-n", "default",
				wellKnown.configMapName)
			Expect(cmd.Run()).To(Succeed())

			By("waiting for new ConfigMap")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "cm", wellKnown.configMapName,
					"-n", "default", "-o", "jsonpath={.metadata.uid}")
				newID, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(newID).ToNot(Equal(originalUID), "expected new ConfigMap ID")
			}).Should(Succeed())
		})

		It("should successfully delete resources", func() {
			manifests := []string{
				"config/samples/garage_v1alpha1_accesspolicy.yaml",
				"config/samples/garage_v1alpha1_accesskey.yaml",
				"config/samples/garage_v1alpha1_bucket.yaml",
			}

			By("removing custom resources")
			for _, manifest := range manifests {
				cmd := exec.Command("kubectl", "delete", "-f", manifest,
					"--wait=true", "--timeout=30s")
				out, err := utils.Run(cmd)
				Expect(err).ToNot(HaveOccurred(), "deleting %s: %s", manifest, out)
			}

			By("secret and configmap deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", wellKnown.secretName,
					"-n", "default", "--ignore-not-found", "-o", "name")
				output, _ := utils.Run(cmd)
				g.Expect(strings.TrimSpace(output)).To(BeEmpty(), "expect no output when not found")
			}).Should(Succeed(), "should delete Secret after AccessKey")

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "cm", wellKnown.configMapName,
					"-n", "default", "--ignore-not-found", "-o", "name")
				output, _ := utils.Run(cmd)
				g.Expect(strings.TrimSpace(output)).To(BeEmpty(), "expect no output when not found")
			}).Should(Succeed(), "should delete ConfigMap after Bucket")
		})
	})

	// +kubebuilder:scaffold:e2e-webhooks-checks

	// TBD: webhooks or validation?
})

// removeFinalizers ensures that leftover resources can be deleted during teardown.
func removeFinalizers(crds ...string) {
	for _, crd := range crds {
		cmd := exec.Command(
			"kubectl", "get", crd, "-A", "-o", "jsonpath={range .items[*]}{.metadata.namespace}/{.metadata.name}{\"\\n\"}{end}")
		out, err := utils.Run(cmd)
		if err == nil && len(out) > 0 {
			resources := strings.Split(strings.TrimSpace(out), "\n")
			for _, resource := range resources {
				parts := strings.Split(resource, "/")
				if len(parts) == 2 {
					ns, name := parts[0], parts[1]
					cmd = exec.Command("kubectl", "patch", crd, name, "-n", ns,
						"--type=json", "-p", `[{"op":"remove","path":"/metadata/finalizers"}]`)
					_, _ = utils.Run(cmd)
				}
			}
		}
	}
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
