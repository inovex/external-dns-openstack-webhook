package main

import (
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"

	"external-dns-openstack-webhook/internal/designate/provider"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

func main() {
	epf := endpoint.NewDomainFilter([]string{})
	dp, err := provider.NewDesignateProvider(epf, false)
	if err != nil {
		log.Fatalf("NewDesignateProvider: %v", err)
	}

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

	go func() {
		log.Debug("Starting status server on :8080")
	s := &http.Server{
		Addr:         "0.0.0.0:8080",
		Handler:      m,
	}

	l, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatal(err)
	}
	err = s.Serve(l)
	if err != nil {
		log.Fatalf("health listener stopped : %s", err)
	}
	}()

	log.Printf("Starting server")
	api.StartHTTPApi(dp, nil, 0, 0, "127.0.0.1:8888")
}
