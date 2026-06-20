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

package garage

import (
	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	apiRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "garage_admin_api_requests_total",
		Help: "Total Garage admin API requests by response status class (2xx/4xx/5xx or error).",
	}, []string{"code"})

	apiRequestDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "garage_admin_api_request_duration_seconds",
		Help:    "Duration of Garage admin API requests in seconds.",
		Buckets: prometheus.DefBuckets,
	})
)

// Values for the "code" label on apiRequests
const (
	codeHTTP2xx = "2xx"
	codeHTTP3xx = "3xx"
	codeHTTP4xx = "4xx"
	codeHTTP5xx = "5xx"
	codeError   = "error"
)

// collectors lists every metric exposed by this package.
var collectors = []prometheus.Collector{apiRequests, apiRequestDuration}

// RegisterMetrics registers the Garage admin API metrics with the
// controller-runtime registry.
// Call exactly once during startup or MustRegister will panic.
func RegisterMetrics() {
	// TODO: sync once?;
	// TODO: garage client package is now tied to controller-runtime, improve.
	crmetrics.Registry.MustRegister(collectors...)
}

// statusClass maps an HTTP status code to a low cardinality label.
func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return codeHTTP2xx
	case code >= 300 && code < 400:
		return codeHTTP3xx
	case code >= 400 && code < 500:
		return codeHTTP4xx
	default:
		return codeHTTP5xx
	}
}
