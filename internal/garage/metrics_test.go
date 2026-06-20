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
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
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
				apiRequests.Reset()

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
				}))
				defer srv.Close()

				err := NewClient(srv.URL, "token").Health(t.Context())
				if tc.wantErr && err == nil {
					t.Fatal("expected error, got nil")
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				got := testutil.ToFloat64(apiRequests.WithLabelValues(tc.statusCodeBucket))
				if got != 1 {
					t.Errorf("requests{code=%s} = %v, expected 1", tc.statusCodeBucket, got)
				}
			})
		}
	})

	t.Run("counts transport failures as error", func(t *testing.T) {
		apiRequests.Reset()

		// :1 is reserved and causes a transport error:
		err := NewClient("http://127.0.0.1:1", "token").Health(t.Context())
		if err == nil {
			t.Fatal("expected transport error, got nil")
		}

		got := testutil.ToFloat64(apiRequests.WithLabelValues(codeError))
		if got != 1 {
			t.Errorf("requests{code=error} = %v, expected 1", got)
		}
	})

	t.Run("records request duration", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		before := histogramSampleCount(t, apiRequestDuration)
		if err := NewClient(srv.URL, "token").Health(t.Context()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if after := histogramSampleCount(t, apiRequestDuration); after <= before {
			t.Errorf("duration sample count = %d, expected > %d", after, before)
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

func TestRegisterMetrics(t *testing.T) {
	RegisterMetrics()

	apiRequestDuration.Observe(0.01)
	// no series exposed until a label is observed:
	apiRequests.WithLabelValues(codeHTTP2xx).Inc()

	for _, name := range []string{
		"garage_admin_api_requests_total",
		"garage_admin_api_request_duration_seconds",
	} {
		count, err := testutil.GatherAndCount(crmetrics.Registry, name)
		if err != nil {
			t.Fatalf("gathering %s: %v", name, err)
		}
		if count == 0 {
			t.Errorf("%s not registered on the controller-runtime registry", name)
		}
	}
}
