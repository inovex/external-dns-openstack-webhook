/*
Copyright 2017 The Kubernetes Authors.
Copyright 2024 inovex GmbH.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"time"

	"external-dns-openstack-webhook/internal/metrics"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/gophercloud/v2/pagination"
	log "github.com/sirupsen/logrus"
)

// interface between provider and OpenStack DNS API
type DesignateClientInterface interface {
	// ForEachZone calls handler for each zone managed by the Designate
	ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error

	// ForEachRecordSet calls handler for each recordset in the given DNS zone
	ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error

	// CreateRecordSet creates recordset in the given DNS zone
	CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error)

	// UpdateRecordSet updates recordset in the given DNS zone
	UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error

	// DeleteRecordSet deletes recordset in the given DNS zone
	DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error
}

// implementation of the DesignateClientInterface
type designateClient struct {
	serviceClient *gophercloud.ServiceClient
}

// factory function for the DesignateClientInterface
func NewDesignateClient() (DesignateClientInterface, error) {
	serviceClient, err := createDesignateServiceClient()
	if err != nil {
		return nil, err
	}
	return &designateClient{serviceClient}, nil
}

// authenticate in OpenStack and obtain Designate service endpoint
func createDesignateServiceClient() (*gophercloud.ServiceClient, error) {
	ctx := context.Background()

	authOptions, endpointOptions, tlsConfig, err := clouds.Parse()
	if err != nil {
		return nil, err
	}
	authOptions.AllowReauth = true

	providerClient, err := config.NewProviderClient(ctx, authOptions, config.WithTLSConfig(tlsConfig))
	if err != nil {
		return nil, err
	}
	log.Infof("Using OpenStack Keystone at %s", providerClient.IdentityEndpoint)

	client, err := openstack.NewDNSV2(providerClient, endpointOptions)
	if err != nil {
		return nil, err
	}
	log.Infof("Found OpenStack Designate (DNS) service at %s", client.Endpoint)
	return client, nil
}

// ForEachZone calls handler for each zone managed by the Designate
func (c designateClient) ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error {
	startTime := time.Now()

	pager := zones.List(c.serviceClient, zones.ListOpts{})
	var pageCount int
	var zoneCount int

	err := pager.EachPage(ctx,
		func(ctx context.Context, page pagination.Page) (bool, error) {
			// Jede Seite entspricht einem separaten API-Aufruf
			pageCount++
			metrics.TotalApiCalls.Inc()

			list, err := zones.ExtractZones(page)
			if err != nil {
				return false, err
			}

			zoneCount += len(list)

			for _, zone := range list {
				err := handler(&zone)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("ForEachZone").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("ForEachZone failed after %v: %v", duration, err)
	} else {
		log.Debugf("✓ ForEachZone completed: %d zones across %d pages in %v", zoneCount, pageCount, duration)
	}

	return err
}

// ForEachRecordSet calls handler for each recordset in the given DNS zone
func (c designateClient) ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	startTime := time.Now()

	pager := recordsets.ListByZone(c.serviceClient, zoneID, recordsets.ListOpts{})
	var pageCount int
	var recordCount int

	err := pager.EachPage(ctx,
		func(ctx context.Context, page pagination.Page) (bool, error) {
			// Jede Seite entspricht einem separaten API-Aufruf
			pageCount++
			metrics.TotalApiCalls.Inc()

			list, err := recordsets.ExtractRecordSets(page)
			if err != nil {
				return false, err
			}

			recordCount += len(list)

			for _, recordSet := range list {
				err := handler(&recordSet)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("ForEachRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("ForEachRecordSet failed for zone %s after %v: %v", zoneID, duration, err)
	} else {
		log.Debugf("✓ ForEachRecordSet zone=%s: %d records across %d pages in %v", zoneID[:8]+"...", recordCount, pageCount, duration)
	}

	return err
}

// CreateRecordSet creates recordset in the given DNS zone
func (c designateClient) CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error) {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	log.Debugf("→ Creating recordset: %s (%s) with %d targets", opts.Name, opts.Type, len(opts.Records))

	r, err := recordsets.Create(ctx, c.serviceClient, zoneID, opts).Extract()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("CreateRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ CreateRecordSet failed for %s after %v: %v", opts.Name, duration, err)
		return "", err
	}

	log.Debugf("✓ CreateRecordSet successful: %s (ID: %s) in %v", opts.Name, r.ID[:8]+"...", duration)
	return r.ID, nil
}

// UpdateRecordSet updates recordset in the given DNS zone
func (c designateClient) UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	recordCount := 0
	if opts.Records != nil {
		recordCount = len(opts.Records)
	}
	log.Debugf("→ Updating recordset: %s with %d targets", recordSetID[:8]+"...", recordCount)

	_, err := recordsets.Update(ctx, c.serviceClient, zoneID, recordSetID, opts).Extract()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("UpdateRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ UpdateRecordSet failed for %s after %v: %v", recordSetID[:8]+"...", duration, err)
	} else {
		log.Debugf("✓ UpdateRecordSet successful: %s in %v", recordSetID[:8]+"...", duration)
	}

	return err
}

// DeleteRecordSet deletes recordset in the given DNS zone
func (c designateClient) DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	log.Debugf("→ Deleting recordset: %s", recordSetID[:8]+"...")

	err := recordsets.Delete(ctx, c.serviceClient, zoneID, recordSetID).ExtractErr()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("DeleteRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ DeleteRecordSet failed for %s after %v: %v", recordSetID[:8]+"...", duration, err)
	} else {
		log.Debugf("✓ DeleteRecordSet successful: %s in %v", recordSetID[:8]+"...", duration)
	}

	return err
}
