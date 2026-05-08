package main

import (
	"net"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"external-dns-t-cloud-public-webhook/internal/dns/provider"
	"external-dns-t-cloud-public-webhook/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

const (
	webhookServerAddr = "0.0.0.0:8888"
	statusServerAddr  = "0.0.0.0:8080"
)

func main() {
	var domainFilters []string
	var zoneType string
	pflag.StringArrayVar(&domainFilters, "domain-filter", []string{}, "List of domains to work on (can be specified multiple times)")
	pflag.StringVar(&zoneType, "zone-type", "", "DNS zone visibility to manage: public or private; overrides ZONE_TYPE")
	pflag.Parse()

	log.SetLevel(log.DebugLevel)

	if zoneType == "" {
		zoneType = os.Getenv("ZONE_TYPE")
	}
	if zoneType == "" {
		zoneType = os.Getenv("OS_ZONE_TYPE")
	}
	if zoneType == "" {
		zoneType = provider.ZoneTypePublic
	}

	if !provider.IsSupportedZoneType(zoneType) {
		log.Fatalf("invalid zone type %q: expected %q or %q", zoneType, provider.ZoneTypePublic, provider.ZoneTypePrivate)
	}

	startedChan := make(chan struct{})
	httpApiStarted := false

	go func() {
		<-startedChan
		httpApiStarted = true
	}()

	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !httpApiStarted {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	m.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)

	go func() {
		log.Debugf("Starting status server on %s", statusServerAddr)
		s := &http.Server{
			Addr:    statusServerAddr,
			Handler: m,
		}

		l, err := net.Listen("tcp", statusServerAddr)
		if err != nil {
			log.Fatal(err)
		}
		err = s.Serve(l)
		if err != nil {
			log.Fatalf("status listener stopped : %s", err)
		}
	}()

	epf := endpoint.NewDomainFilter(domainFilters)
	dp, err := provider.NewDNSProvider(*epf, zoneType, false)
	if err != nil {
		log.Fatalf("NewDNSProvider: %v", err)
		metrics.TCloudPublicConnectionMetric.Set(0)
	}
	metrics.TCloudPublicConnectionMetric.Set(1)
	log.Debugf("Connected to T-Cloud Public API")

	log.Debugf("Starting webhook server on %s", webhookServerAddr)
	api.StartHTTPApi(dp, startedChan, 0, 0, webhookServerAddr)
}
