package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPClientMetrics holds the metrics collected by instrumenting the
// transport of an http.Client, one series set per upstream service.
type HTTPClientMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight *prometheus.GaugeVec
	dns      *prometheus.HistogramVec
	tls      *prometheus.HistogramVec
}

// HTTPClient is the instance registered with the default registry.
var HTTPClient = NewHTTPClientMetrics(prometheus.DefaultRegisterer)

// NewHTTPClientMetrics creates the HTTP client metrics and registers them with reg.
func NewHTTPClientMetrics(reg prometheus.Registerer) *HTTPClientMetrics {
	const prefix = "external_dns_webhook_http_client_"
	m := &HTTPClientMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: prefix + "requests_total",
			Help: "Total number of completed HTTP requests to OpenStack services, by service, status code and method",
		}, []string{"service", "code", "method"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    prefix + "request_duration_seconds",
			Help:    "Duration of HTTP requests to OpenStack services, by service, status code and method",
			Buckets: prometheus.DefBuckets,
		}, []string{"service", "code", "method"}),
		inFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: prefix + "in_flight_requests",
			Help: "Number of HTTP requests to OpenStack services currently in flight",
		}, []string{"service"}),
		dns: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    prefix + "dns_duration_seconds",
			Help:    "Duration of DNS lookups made for HTTP requests to OpenStack services",
			Buckets: prometheus.DefBuckets,
		}, []string{"service"}),
		tls: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    prefix + "tls_duration_seconds",
			Help:    "Duration of TLS handshakes made for HTTP requests to OpenStack services",
			Buckets: prometheus.DefBuckets,
		}, []string{"service"}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight, m.dns, m.tls)
	return m
}

// InstrumentTransport wraps next so that every request is recorded. classify
// maps a request to the value of the service label.
func (m *HTTPClientMetrics) InstrumentTransport(next http.RoundTripper, classify func(*http.Request) string) http.RoundTripper {
	var chains sync.Map // service -> http.RoundTripper

	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		service := classify(r)
		rt, ok := chains.Load(service)
		if !ok {
			rt, _ = chains.LoadOrStore(service, m.chain(next, service))
		}
		return rt.(http.RoundTripper).RoundTrip(r)
	})
}

// chain builds the promhttp middleware stack for a single service.
func (m *HTTPClientMetrics) chain(next http.RoundTripper, service string) http.RoundTripper {
	trace := &promhttp.InstrumentTrace{
		DNSDone:          m.dns.WithLabelValues(service).Observe,
		TLSHandshakeDone: m.tls.WithLabelValues(service).Observe,
	}

	return promhttp.InstrumentRoundTripperInFlight(m.inFlight.WithLabelValues(service),
		promhttp.InstrumentRoundTripperCounter(m.requests.MustCurryWith(prometheus.Labels{"service": service}),
			promhttp.InstrumentRoundTripperTrace(trace,
				promhttp.InstrumentRoundTripperDuration(m.duration.MustCurryWith(prometheus.Labels{"service": service}), next))))
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
