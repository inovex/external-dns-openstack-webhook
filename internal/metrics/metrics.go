package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	TCloudPublicConnectionMetric = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "external_dns_webhook_t_cloud_public_connection_initialized",
		Help: "Indicates if the webhook has been initialized with T-Cloud Public API credentials (1 for initialized, 0 for not initialized)",
	})
	FailedApiCallsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "external_dns_webhook_failed_api_calls_total",
		Help: "Total number of failed API calls",
	})
	TotalApiCalls = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "external_dns_webhook_total_api_calls",
		Help: "Total number of API calls",
	})
	ApiCallLatency = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "external_dns_webhook_api_call_latency_seconds",
		Help: "Latency of T-Cloud Public API calls",
	}, []string{"method"}) // method label to differentiate API calls
)

func init() {
	prometheus.MustRegister(TCloudPublicConnectionMetric, FailedApiCallsTotal, ApiCallLatency, TotalApiCalls)
}
