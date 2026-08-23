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

package controller

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	garagev1alpha1 "github.com/bmarinov/garage-storage-controller/api/v1alpha1"
)

var _ = Describe("Printer columns", func() {
	DescribeTable("the header row",
		func(plural, wantPlain, wantWide string) {
			gotPlain, gotWide := servedHeaders(plural)

			Expect(gotPlain).To(Equal(wantPlain))
			Expect(gotWide).To(Equal(wantWide))
		},
		Entry("kubectl get buckets", "buckets",
			"NAME READY ALIAS AGE",
			"NAME READY ALIAS AGE REASON ID MAX-SIZE MAX-OBJECTS"),
		Entry("kubectl get accesskeys", "accesskeys",
			"NAME READY KEY-ID AGE",
			"NAME READY KEY-ID AGE REASON SECRET"),
		Entry("kubectl get accesspolicies", "accesspolicies",
			"NAME READY BUCKET ACCESSKEY AGE",
			"NAME READY BUCKET ACCESSKEY AGE REASON READ WRITE OWNER"),
	)
})

// servedHeaders retrieves the named resource schema and renders the header row.
func servedHeaders(plural string) (plain, wide string) {
	GinkgoHelper()

	// envtest installs the manifests from config/crd/bases.
	//
	// see suite_test.go:
	// CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},

	tableCfg := rest.CopyConfig(cfg)
	tableCfg.GroupVersion = &garagev1alpha1.GroupVersion
	tableCfg.APIPath = "/apis"
	tableCfg.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme).WithoutConversion()

	restClient, err := rest.RESTClientFor(tableCfg)
	Expect(err).NotTo(HaveOccurred())

	raw, err := restClient.Get().
		Resource(plural).
		Namespace("default").
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io").
		Do(ctx).
		Raw()
	Expect(err).NotTo(HaveOccurred())

	var table metav1.Table
	Expect(json.Unmarshal(raw, &table)).To(Succeed())

	plainNames := make([]string, 0, len(table.ColumnDefinitions))
	wideNames := make([]string, 0, len(table.ColumnDefinitions))
	for _, c := range table.ColumnDefinitions {
		wideNames = append(wideNames, strings.ToUpper(c.Name))
		if c.Priority == 0 {
			plainNames = append(plainNames, strings.ToUpper(c.Name))
		}
	}
	return strings.Join(plainNames, " "), strings.Join(wideNames, " ")
}
