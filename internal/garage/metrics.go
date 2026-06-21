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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the Garage admin API metrics for a single client.
// Construct with NewMetrics and register Collectors() with the Prometheus registry.
//
// Pass instance to NewClient via WithMetrics to start recording.
type Metrics struct {
	requests *prometheus.CounterVec
	duration prometheus.Histogram
	up       prometheus.Gauge
}

const (
	metricsNamespace = "garage_controller"
	metricsSubsystem = "admin_api"
)

func NewMetrics() *Metrics {
	return &Metrics{
		requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "requests_total",
				Help:      "Total Garage admin API requests by response status class (2xx/4xx/5xx or error).",
			},
			[]string{"status_class"}),
		duration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "request_duration_seconds",
				Help:      "Duration of Garage admin API requests in seconds.",
				Buckets:   prometheus.DefBuckets,
			}),
		up: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "up",
				Help:      "Gauge for the Garage admin API: 1 reachable, 0 not.",
			}),
	}
}

// Collectors returns the metrics to register with a Prometheus registry.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.requests, m.duration, m.up}
}

// SetAPIUp records whether the Garage admin API is currently reachable.
func (m *Metrics) SetAPIUp(isUp bool) {
	if isUp {
		m.up.Set(1)
		return
	}
	m.up.Set(0)
}

func (m *Metrics) record(code string, d time.Duration) {
	m.duration.Observe(d.Seconds())
	m.requests.WithLabelValues(code).Inc()
}

// Values for the "status_class" label on requests
const (
	codeHTTP2xx = "2xx"
	codeHTTP3xx = "3xx"
	codeHTTP4xx = "4xx"
	codeHTTP5xx = "5xx"
	codeError   = "error"
)

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
