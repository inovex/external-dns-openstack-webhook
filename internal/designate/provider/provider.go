/*
gopyright 2017 The Kubernetes Authors.
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

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

import "external-dns-openstack-webhook/internal/designate/client"

const (
	// ID of the RecordSet from which endpoint was created
	designateRecordSetID = "designate-recordset-id"
	// Zone ID of the RecordSet
	designateZoneID = "designate-record-id"

	// Initial records values of the RecordSet. This label is required in order not to loose records that haven't
	// changed where there are several targets per domain and only some of them changed.
	// Values are joined by zero-byte to in order to get a single string
	designateOriginalRecords = "designate-original-records"
)

// designate provider type
type designateProvider struct {
	provider.BaseProvider
	client client.DesignateClientInterface

	// only consider hosted zones managing domains ending in this suffix
	domainFilter endpoint.DomainFilter
	dryRun       bool
}

// NewDesignateProvider is a factory function for OpenStack designate providers
func NewDesignateProvider(domainFilter endpoint.DomainFilter, dryRun bool) (provider.Provider, error) {
	client, err := client.NewDesignateClient()
	if err != nil {
		return nil, err
	}
	return &designateProvider{
		client:       client,
		domainFilter: domainFilter,
		dryRun:       dryRun,
	}, nil
}

// converts domain names to FQDN
func canonicalizeDomainNames(domains []string) []string {
	var cDomains []string
	for _, d := range domains {
		if !strings.HasSuffix(d, ".") {
			d += "."
			cDomains = append(cDomains, strings.ToLower(d))
		}
	}
	return cDomains
}

// converts domain name to FQDN
func canonicalizeDomainName(d string) string {
	if !strings.HasSuffix(d, ".") {
		d += "."
	}
	return strings.ToLower(d)
}

// returns ZoneID -> ZoneName mapping for zones that are managed by the Designate and match domain filter
func (p designateProvider) getZones(ctx context.Context) (map[string]string, error) {
	result := map[string]string{}

	err := p.client.ForEachZone(ctx,
		func(zone *zones.Zone) error {
			if zone.Type != "" && strings.ToUpper(zone.Type) != "PRIMARY" || zone.Status != "ACTIVE" {
				return nil
			}

			zoneName := canonicalizeDomainName(zone.Name)
			if !p.domainFilter.Match(zoneName) {
				return nil
			}
			result[zone.ID] = zoneName
			return nil
		},
	)

	return result, err
}

// finds best suitable DNS zone for the hostname
func (p designateProvider) getHostZoneID(hostname string, managedZones map[string]string) (string, error) {
	longestZoneLength := 0
	resultID := ""

	for zoneID, zoneName := range managedZones {
		if !strings.HasSuffix(hostname, zoneName) {
			continue
		}
		ln := len(zoneName)
		if ln > longestZoneLength {
			resultID = zoneID
			longestZoneLength = ln
		}
	}

	return resultID, nil
}

// Records returns the list of records.
func (p designateProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	var result []*endpoint.Endpoint
	managedZones, err := p.getZones(ctx)
	if err != nil {
		return nil, err
	}
	for zoneID := range managedZones {
		err = p.client.ForEachRecordSet(ctx, zoneID,
			func(recordSet *recordsets.RecordSet) error {
				if recordSet.Type != endpoint.RecordTypeA && recordSet.Type != endpoint.RecordTypeTXT && recordSet.Type != endpoint.RecordTypeCNAME {
					return nil
				}

				ep := endpoint.NewEndpointWithTTL(recordSet.Name, recordSet.Type, endpoint.TTL(recordSet.TTL), recordSet.Records...)
				ep.Labels[designateRecordSetID] = recordSet.ID
				ep.Labels[designateZoneID] = recordSet.ZoneID
				ep.Labels[designateOriginalRecords] = strings.Join(recordSet.Records, "\000")
				result = append(result, ep)

				return nil
			},
		)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// temporary structure to hold recordset parameters so that we could aggregate endpoints into recordsets
type recordSet struct {
	dnsName     string
	recordType  string
	zoneID      string
	recordSetID string
	ttl         int
	names       map[string]bool
}

// adds endpoint into recordset aggregation, loading original values from endpoint labels first
func addEndpoint(ep *endpoint.Endpoint, recordSets map[string]*recordSet, oldEndpoints []*endpoint.Endpoint, delete bool) {
	key := fmt.Sprintf("%s/%s", ep.DNSName, ep.RecordType)
	rs := recordSets[key]
	if rs == nil {
		rs = &recordSet{
			dnsName:    canonicalizeDomainName(ep.DNSName),
			recordType: ep.RecordType,
			names:      make(map[string]bool),
			ttl:        int(ep.RecordTTL),
		}
	}

	addDesignateIDLabelsFromExistingEndpoints(oldEndpoints, ep)

	if rs.zoneID == "" {
		rs.zoneID = ep.Labels[designateZoneID]
	}
	if rs.recordSetID == "" {
		rs.recordSetID = ep.Labels[designateRecordSetID]
	}
	for _, rec := range strings.Split(ep.Labels[designateOriginalRecords], "\000") {
		if _, ok := rs.names[rec]; !ok && rec != "" {
			rs.names[rec] = true
		}
	}
	targets := ep.Targets
	if ep.RecordType == endpoint.RecordTypeCNAME {
		targets = canonicalizeDomainNames(targets)
	}
	for _, t := range targets {
		rs.names[t] = !delete
	}
	recordSets[key] = rs
}

// addDesignateIDLabelsFromExistingEndpoints adds the labels identified by the constants designateZoneID and designateRecordSetID
// to an Endpoint. Therefore, it searches all given existing endpoints for an endpoint with the same record type and record
// value. If the given Endpoint already has the labels set, they are left untouched. This fixes an issue with the
// TXTRegistry which generates new TXT entries instead of updating the old ones.
func addDesignateIDLabelsFromExistingEndpoints(existingEndpoints []*endpoint.Endpoint, ep *endpoint.Endpoint) {
	_, hasZoneIDLabel := ep.Labels[designateZoneID]
	_, hasRecordSetIDLabel := ep.Labels[designateRecordSetID]
	if hasZoneIDLabel && hasRecordSetIDLabel {
		return
	}
	for _, oep := range existingEndpoints {
		if ep.RecordType == oep.RecordType && ep.DNSName == oep.DNSName {
			if !hasZoneIDLabel {
				ep.Labels[designateZoneID] = oep.Labels[designateZoneID]
			}
			if !hasRecordSetIDLabel {
				ep.Labels[designateRecordSetID] = oep.Labels[designateRecordSetID]
			}
			return
		}
	}
}

// ApplyChanges applies a given set of changes in a given zone.
func (p designateProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	managedZones, err := p.getZones(ctx)
	if err != nil {
		return err
	}

	endpoints, err := p.Records(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch active records: %w", err)
	}

	recordSets := map[string]*recordSet{}
	for _, ep := range changes.Create {
		addEndpoint(ep, recordSets, endpoints, false)
	}
	for _, ep := range changes.UpdateOld {
		addEndpoint(ep, recordSets, endpoints, true)
	}
	for _, ep := range changes.UpdateNew {
		addEndpoint(ep, recordSets, endpoints, false)
	}
	for _, ep := range changes.Delete {
		addEndpoint(ep, recordSets, endpoints, true)
	}

	for _, rs := range recordSets {
		if err2 := p.upsertRecordSet(ctx, rs, managedZones); err == nil {
			err = err2
		}
	}
	return err
}

// apply recordset changes by inserting/updating/deleting recordsets
func (p designateProvider) upsertRecordSet(ctx context.Context, rs *recordSet, managedZones map[string]string) error {
	if rs.zoneID == "" {
		var err error
		rs.zoneID, err = p.getHostZoneID(rs.dnsName, managedZones)
		if err != nil {
			return err
		}
		if rs.zoneID == "" {
			log.Debugf("Skipping record %s because no hosted zone matching record DNS Name was detected", rs.dnsName)
			return nil
		}
	}
	var records []string
	for rec, v := range rs.names {
		if v {
			records = append(records, rec)
		}
	}
	if rs.recordSetID == "" && records == nil {
		return nil
	}
	if rs.recordSetID == "" {
		opts := recordsets.CreateOpts{
			Name:    rs.dnsName,
			Type:    rs.recordType,
			Records: records,
			TTL:     rs.ttl,
		}
		log.Infof("Creating records: %s/%s: %s", rs.dnsName, rs.recordType, strings.Join(records, ","))
		if p.dryRun {
			return nil
		}
		_, err := p.client.CreateRecordSet(ctx, rs.zoneID, opts)
		return err
	} else if len(records) == 0 {
		log.Infof("Deleting records for %s/%s", rs.dnsName, rs.recordType)
		if p.dryRun {
			return nil
		}
		return p.client.DeleteRecordSet(ctx, rs.zoneID, rs.recordSetID)
	} else {
		opts := recordsets.UpdateOpts{
			Records: records,
			TTL:     &rs.ttl,
		}
		log.Infof("Updating records: %s/%s: %s", rs.dnsName, rs.recordType, strings.Join(records, ","))
		if p.dryRun {
			return nil
		}
		return p.client.UpdateRecordSet(ctx, rs.zoneID, rs.recordSetID, opts)
	}
}
