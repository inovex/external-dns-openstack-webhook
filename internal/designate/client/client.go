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
	pager := zones.List(c.serviceClient, zones.ListOpts{})
	return pager.EachPage(ctx,
		func(ctx context.Context, page pagination.Page) (bool, error) {
			list, err := zones.ExtractZones(page)
			if err != nil {
				return false, err
			}
			for _, zone := range list {
				err := handler(&zone)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)
}

// ForEachRecordSet calls handler for each recordset in the given DNS zone
func (c designateClient) ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	pager := recordsets.ListByZone(c.serviceClient, zoneID, recordsets.ListOpts{})
	return pager.EachPage(ctx,
		func(ctx context.Context, page pagination.Page) (bool, error) {
			list, err := recordsets.ExtractRecordSets(page)
			if err != nil {
				return false, err
			}
			for _, recordSet := range list {
				err := handler(&recordSet)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)
}

// CreateRecordSet creates recordset in the given DNS zone
func (c designateClient) CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error) {
	r, err := recordsets.Create(ctx, c.serviceClient, zoneID, opts).Extract()
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

// UpdateRecordSet updates recordset in the given DNS zone
func (c designateClient) UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	_, err := recordsets.Update(ctx, c.serviceClient, zoneID, recordSetID, opts).Extract()
	return err
}

// DeleteRecordSet deletes recordset in the given DNS zone
func (c designateClient) DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error {
	return recordsets.Delete(ctx, c.serviceClient, zoneID, recordSetID).ExtractErr()
}
