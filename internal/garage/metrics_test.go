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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestDoRequestMetrics(t *testing.T) {
	t.Run("counts requests by status class", func(t *testing.T) {
		testCases := []struct {
			statusCodeBucket string
			status           int
			wantErr          bool
		}{
			{codeHTTP2xx, http.StatusOK, false},
			{codeHTTP4xx, http.StatusNotFound, true},
			{codeHTTP5xx, http.StatusInternalServerError, true},
		}
		for _, tc := range testCases {
			t.Run(tc.statusCodeBucket, func(t *testing.T) {
				m := NewMetrics()

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
				}))
				defer srv.Close()

				err := NewClient(srv.URL, "token", WithMetrics(m)).Health(t.Context())
				if tc.wantErr && err == nil {
					t.Fatal("expected error, got nil")
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				got := testutil.ToFloat64(m.requests.WithLabelValues(tc.statusCodeBucket))
				if got != 1 {
					t.Errorf("requests{code=%s} = %v, expected 1", tc.statusCodeBucket, got)
				}
			})
		}
	})

	t.Run("counts transport failures as error", func(t *testing.T) {
		m := NewMetrics()

		// :1 is reserved and causes a transport error:
		err := NewClient("http://127.0.0.1:1", "token", WithMetrics(m)).Health(t.Context())
		if err == nil {
			t.Fatal("expected transport error, got nil")
		}

		got := testutil.ToFloat64(m.requests.WithLabelValues(codeError))
		if got != 1 {
			t.Errorf("requests{code=error} = %v, expected 1", got)
		}
	})

	t.Run("records request duration", func(t *testing.T) {
		m := NewMetrics()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		if err := NewClient(srv.URL, "token", WithMetrics(m)).Health(t.Context()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := histogramSampleCount(t, m.duration); got != 1 {
			t.Errorf("duration sample count = %d, expected 1", got)
		}
	})

	t.Run("a client without metrics records nothing and does not panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		if err := NewClient(srv.URL, "token").Health(t.Context()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func histogramSampleCount(t *testing.T, h prometheus.Histogram) uint64 {
	t.Helper()
	var m dto.Metric
	if err := h.Write(&m); err != nil {
		t.Fatalf("writing histogram metric: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestMetricsCollectors(t *testing.T) {
	m := NewMetrics()

	m.duration.Observe(0.01)
	// no series exposed until a label is observed:
	m.requests.WithLabelValues(codeHTTP2xx).Inc()

	reg := prometheus.NewRegistry()
	reg.MustRegister(m.Collectors()...)

	for _, name := range []string{
		"garage_admin_api_requests_total",
		"garage_admin_api_request_duration_seconds",
	} {
		count, err := testutil.GatherAndCount(reg, name)
		if err != nil {
			t.Fatalf("gathering %s: %v", name, err)
		}
		if count == 0 {
			t.Errorf("%s not registered", name)
		}
	}
}
