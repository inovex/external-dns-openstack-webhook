package main

import (
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"

	"external-dns-openstack-webhook/internal/designate/provider"
	"external-dns-openstack-webhook/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

const (
	webhookServerAddr = "127.0.0.1:8888"
	statusServerAddr  = "0.0.0.0:8080"
)

func main() {
	log.SetLevel(log.DebugLevel)

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

	epf := endpoint.NewDomainFilter([]string{})
	dp, err := provider.NewDesignateProvider(*epf, false)
	if err != nil {
		log.Fatalf("NewDesignateProvider: %v", err)
		metrics.OpenstackConnectionMetric.Set(0)
	}
	metrics.OpenstackConnectionMetric.Set(1)
	log.Debugf("Connected to OpenStack API")

	log.Debugf("Starting webhook server on %s", webhookServerAddr)
	api.StartHTTPApi(dp, startedChan, 0, 0, webhookServerAddr)
}
