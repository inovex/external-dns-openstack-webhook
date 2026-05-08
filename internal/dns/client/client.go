/*
Copyright 2017 The Kubernetes Authors.
Copyright 2024 inovex GmbH.
Copyright 2026 T-Systems International GmbH.

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

	"external-dns-t-cloud-public-webhook/internal/metrics"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/recordsets"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/zones"
	"github.com/opentelekomcloud/gophertelekomcloud/pagination"
	log "github.com/sirupsen/logrus"
)

// interface between provider and DNS API
type DNSClientInterface interface {
	// ForEachZone calls handler for each managed zone
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

// implementation of the DNSClientInterface
type dnsClient struct {
	serviceClient *golangsdk.ServiceClient
	zoneType      string
}

// factory function for the DNSClientInterface
func NewDNSClient(zoneType string) (DNSClientInterface, error) {
	serviceClient, err := createDNSServiceClient()
	if err != nil {
		return nil, err
	}
	return &dnsClient{serviceClient: serviceClient, zoneType: zoneType}, nil
}

// authenticate in T-Cloud Public and obtain DNS service endpoint
func createDNSServiceClient() (*golangsdk.ServiceClient, error) {
	env := openstack.NewEnv("OS_")
	cloud, err := env.Cloud()
	if err != nil {
		return nil, err
	}

	providerClient, err := openstack.AuthenticatedClientFromCloud(cloud)
	if err != nil {
		return nil, err
	}
	log.Infof("Using T-Cloud Public IAM at %s", providerClient.IdentityEndpoint)

	endpointOptions := golangsdk.EndpointOpts{Region: cloud.RegionName}
	if availability := cloud.EndpointType; availability != "" {
		endpointOptions.Availability = golangsdk.Availability(availability)
	} else if cloud.Interface != "" {
		endpointOptions.Availability = golangsdk.Availability(cloud.Interface)
	}

	client, err := openstack.NewDNSV2(providerClient, endpointOptions)
	if err != nil {
		return nil, err
	}
	log.Infof("Found T-Cloud Public DNS service at %s", client.Endpoint)
	return client, nil
}

// ForEachZone calls handler for each managed zone
func (c dnsClient) ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error {
	startTime := time.Now()

	listOpts := zones.ListOpts{}
	if c.zoneType != "" {
		listOpts.Type = c.zoneType
	}
	pager := zones.List(c.serviceClient, listOpts)
	var pageCount int
	var zoneCount int

	err := pager.EachPage(
		func(page pagination.Page) (bool, error) {
			// Each page corresponds to a separate API call.
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
func (c dnsClient) ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	startTime := time.Now()

	pager := recordsets.ListByZone(c.serviceClient, zoneID, recordsets.ListOpts{})
	var pageCount int
	var recordCount int

	err := pager.EachPage(
		func(page pagination.Page) (bool, error) {
			// Each page corresponds to a separate API call.
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
		log.Debugf("✓ ForEachRecordSet zone=%s: %d records across %d pages in %v", zoneID, recordCount, pageCount, duration)
	}

	return err
}

// CreateRecordSet creates recordset in the given DNS zone
func (c dnsClient) CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error) {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	log.Debugf("→ Creating recordset: %s (%s) with %d targets", opts.Name, opts.Type, len(opts.Records))

	r, err := recordsets.Create(c.serviceClient, zoneID, opts).Extract()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("CreateRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ CreateRecordSet failed for %s after %v: %v", opts.Name, duration, err)
		return "", err
	}

	log.Debugf("✓ CreateRecordSet successful: %s (ID: %s) in %v", opts.Name, r.ID, duration)
	return r.ID, nil
}

// UpdateRecordSet updates recordset in the given DNS zone
func (c dnsClient) UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	recordCount := 0
	if opts.Records != nil {
		recordCount = len(opts.Records)
	}
	log.Debugf("→ Updating recordset: %s with %d targets", recordSetID, recordCount)

	_, err := recordsets.Update(c.serviceClient, zoneID, recordSetID, opts).Extract()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("UpdateRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ UpdateRecordSet failed for %s after %v: %v", recordSetID, duration, err)
	} else {
		log.Debugf("✓ UpdateRecordSet successful: %s in %v", recordSetID, duration)
	}

	return err
}

// DeleteRecordSet deletes recordset in the given DNS zone
func (c dnsClient) DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error {
	startTime := time.Now()
	metrics.TotalApiCalls.Inc()

	log.Debugf("→ Deleting recordset: %s", recordSetID)

	err := recordsets.Delete(c.serviceClient, zoneID, recordSetID).ExtractErr()

	duration := time.Since(startTime)
	metrics.ApiCallLatency.WithLabelValues("DeleteRecordSet").Observe(duration.Seconds())

	if err != nil {
		metrics.FailedApiCallsTotal.Inc()
		log.Errorf("✗ DeleteRecordSet failed for %s after %v: %v", recordSetID, duration, err)
	} else {
		log.Debugf("✓ DeleteRecordSet successful: %s in %v", recordSetID, duration)
	}

	return err
}
