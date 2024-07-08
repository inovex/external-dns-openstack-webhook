package main

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

import "external-dns-openstack-webhook/internal/designate/provider"

func main() {
	epf := endpoint.NewDomainFilter([]string{})
	dp, err := provider.NewDesignateProvider(epf, false)
	if err != nil {
		log.Fatalf("NewDesignateProvider: %v", err)
	}

	log.SetLevel(log.DebugLevel)
	log.Printf("Starting server")
	api.StartHTTPApi(dp, nil, 0, 0, "127.0.0.1:8888")
}
