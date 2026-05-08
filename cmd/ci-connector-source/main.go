package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"sigs.k8s.io/external-dns/endpoint"
)

const endpointCount = 10

func main() {
	addr := getenv("CONNECTOR_SOURCE_SERVER", "127.0.0.1:18080")
	zoneName := mustGetenv("ZONE_NAME")

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fatalf("listen on %s: %v", addr, err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "close listener: %v\n", closeErr)
		}
	}()

	fmt.Printf("connector source listening on %s for zone %s\n", addr, zoneName)

	endpoints := buildEndpoints(zoneName)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fatalf("accept connection: %v", err)
		}

		if err := gob.NewEncoder(conn).Encode(endpoints); err != nil {
			_ = conn.Close()
			if isTransientDisconnect(err) {
				fmt.Fprintf(os.Stderr, "client disconnected before reading endpoints: %v\n", err)
				continue
			}
			fatalf("encode endpoints: %v", err)
		}

		if err := conn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close connection: %v\n", err)
		}
	}
}

func buildEndpoints(zoneName string) []*endpoint.Endpoint {
	endpoints := make([]*endpoint.Endpoint, 0, endpointCount)
	for i := 0; i < endpointCount; i++ {
		name := fmt.Sprintf("ci-%02d.%s", i+1, zoneName)
		target := fmt.Sprintf("192.0.2.%d", i+1)
		endpoints = append(endpoints, endpoint.NewEndpoint(name, endpoint.RecordTypeA, target))
	}
	return endpoints
}

func mustGetenv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		fatalf("%s is required", name)
	}
	return value
}

func getenv(name, fallback string) string {
	value := os.Getenv(name)
	if value != "" {
		return value
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func isTransientDisconnect(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET)
}
