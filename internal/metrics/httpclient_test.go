package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testClient(t *testing.T, next http.RoundTripper) (*http.Client, *HTTPClientMetrics) {
	t.Helper()
	m := NewHTTPClientMetrics(prometheus.NewRegistry())
	classify := func(r *http.Request) string {
		if r.URL.Path == "/v3/auth/tokens" {
			return "keystone"
		}
		return "designate"
	}
	return &http.Client{Transport: m.InstrumentTransport(next, classify)}, m
}

func TestInstrumentTransportCountsRequestsPerServiceMethodAndCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c, m := testClient(t, http.DefaultTransport)

	for _, req := range []struct{ method, path string }{
		{http.MethodPost, "/v3/auth/tokens"},
		{http.MethodGet, "/v2/zones"},
		{http.MethodGet, "/v2/zones"},
		{http.MethodDelete, "/v2/zones/x"},
	} {
		r, _ := http.NewRequest(req.method, srv.URL+req.path, nil)
		resp, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	for _, tc := range []struct {
		service, method, code string
		want                  float64
	}{
		{"keystone", "post", "200", 1},
		{"designate", "get", "200", 2},
		{"designate", "delete", "500", 1},
	} {
		got := testutil.ToFloat64(m.requests.WithLabelValues(tc.service, tc.code, tc.method))
		if got != tc.want {
			t.Errorf("requests{service=%s,method=%s,code=%s} = %v, want %v", tc.service, tc.method, tc.code, got, tc.want)
		}
	}

	if n := testutil.CollectAndCount(m.duration); n != 3 {
		t.Errorf("duration series = %d, want 3", n)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("boom")
}

func TestInstrumentTransportTransportErrorDoesNotPanic(t *testing.T) {
	c, m := testClient(t, failingTransport{})

	if _, err := c.Get("http://designate.invalid/v2/zones"); err == nil {
		t.Fatal("expected error")
	}

	if n := testutil.CollectAndCount(m.requests); n != 0 {
		t.Errorf("requests series after transport error = %d, want 0", n)
	}
	if got := testutil.ToFloat64(m.inFlight.WithLabelValues("designate")); got != 0 {
		t.Errorf("in-flight = %v, want 0 after completion", got)
	}
}
