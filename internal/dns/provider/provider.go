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

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/recordsets"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/zones"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

import "external-dns-t-cloud-public-webhook/internal/dns/client"

const (
	ZoneTypePublic  = "public"
	ZoneTypePrivate = "private"

	// ID of the RecordSet from which endpoint was created
	dnsRecordSetID = "dns-recordset-id"
	// Zone ID of the RecordSet
	dnsZoneID = "dns-zone-id"
	// Zone Type of the RecordSet
	dnsZoneType = "dns-zone-type"

	// Initial records values of the RecordSet. This label is required in order not to loose records that haven't
	// changed where there are several targets per domain and only some of them changed.
	// Values are joined by zero-byte to in order to get a single string
	dnsOriginalRecords = "dns-original-records"

	// Provider-specific key. In Kubernetes manifests this is set through the
	// external-dns.alpha.kubernetes.io/webhook-zone-type annotation.
	zoneTypeProviderSpecificKey = "webhook/zone-type"
)

// dns provider type
type dnsProvider struct {
	provider.BaseProvider
	client client.DNSClientInterface

	// only consider hosted zones managing domains ending in this suffix
	domainFilter endpoint.DomainFilter
	dryRun       bool
}

type managedZone struct {
	name     string
	zoneType string
}

// NewDNSProvider is a factory function for T-Cloud Public DNS providers
func NewDNSProvider(domainFilter endpoint.DomainFilter, dryRun bool) (provider.Provider, error) {
	c, err := client.NewDNSClient()
	if err != nil {
		return nil, err
	}
	return &dnsProvider{
		client:       c,
		domainFilter: domainFilter,
		dryRun:       dryRun,
	}, nil
}

func IsSupportedZoneType(zoneType string) bool {
	switch strings.ToLower(zoneType) {
	case ZoneTypePublic, ZoneTypePrivate:
		return true
	default:
		return false
	}
}

func zoneMatchesVisibility(zone *zones.Zone, zoneType string) bool {
	if zoneType == "" || zone.ZoneType == "" {
		return true
	}
	return strings.EqualFold(zone.ZoneType, zoneType)
}

// converts domain names to FQDN
func canonicalizeDomainNames(domains []string) []string {
	var cDomains []string
	for _, d := range domains {
		if !strings.HasSuffix(d, ".") {
			d += "."
		}
		cDomains = append(cDomains, strings.ToLower(d))
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

// returns ZoneID -> zone metadata mapping for zones that match domain filter
func (p dnsProvider) getZones(ctx context.Context, zoneType string) (map[string]managedZone, error) {
	result := map[string]managedZone{}

	err := p.client.ForEachZone(ctx, zoneType,
		func(zone *zones.Zone) error {
			if zone.Status == "DELETE" || !zoneMatchesVisibility(zone, zoneType) {
				return nil
			}

			zoneName := canonicalizeDomainName(zone.Name)
			if !p.domainFilter.Match(zoneName) {
				return nil
			}
			result[zone.ID] = managedZone{name: zoneName, zoneType: effectiveZoneType(zone, zoneType)}
			return nil
		},
	)

	return result, err
}

func effectiveZoneType(zone *zones.Zone, requestedZoneType string) string {
	if zone.ZoneType != "" {
		return strings.ToLower(zone.ZoneType)
	}
	return strings.ToLower(requestedZoneType)
}

func getEndpointZoneType(ep *endpoint.Endpoint) (string, error) {
	zoneType := ZoneTypePublic
	if value, ok := ep.GetProviderSpecificProperty(zoneTypeProviderSpecificKey); ok {
		zoneType = strings.ToLower(strings.TrimSpace(value))
	} else if value, ok := ep.Labels[dnsZoneType]; ok {
		zoneType = strings.ToLower(strings.TrimSpace(value))
	}

	if IsSupportedZoneType(zoneType) {
		return zoneType, nil
	}
	return "", fmt.Errorf("invalid %s: %q (allowed: %s, %s)", zoneTypeProviderSpecificKey, zoneType, ZoneTypePublic, ZoneTypePrivate)
}

func (p dnsProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	for _, ep := range endpoints {
		zoneTypeValue, ok := ep.GetProviderSpecificProperty(zoneTypeProviderSpecificKey)
		if !ok {
			continue
		}

		zoneType := strings.ToLower(strings.TrimSpace(zoneTypeValue))
		switch zoneType {
		case ZoneTypePublic:
			ep.DeleteProviderSpecificProperty(zoneTypeProviderSpecificKey)
		case ZoneTypePrivate:
			ep.SetProviderSpecificProperty(zoneTypeProviderSpecificKey, ZoneTypePrivate)
		default:
			return nil, fmt.Errorf("invalid %s: %q (allowed: %s, %s)", zoneTypeProviderSpecificKey, zoneTypeValue, ZoneTypePublic, ZoneTypePrivate)
		}
	}
	return endpoints, nil
}

func ensureEndpointLabels(ep *endpoint.Endpoint) {
	if ep.Labels == nil {
		ep.Labels = endpoint.NewLabels()
	}
}

// finds the best suitable DNS zone for the hostname
func getHostZoneID(hostname string, managedZones map[string]managedZone) string {
	longestZoneLength := 0
	resultID := ""

	for zoneID, zone := range managedZones {
		zoneName := zone.name
		if !strings.HasSuffix(hostname, "."+zoneName) && hostname != zoneName {
			continue
		}
		ln := len(zoneName)
		if ln > longestZoneLength {
			resultID = zoneID
			longestZoneLength = ln
		}
	}

	return resultID
}

// Records returns the list of records.
func (p dnsProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	var result []*endpoint.Endpoint

	for _, zoneType := range []string{ZoneTypePublic, ZoneTypePrivate} {
		managedZones, err := p.getZones(ctx, zoneType)
		if err != nil {
			return nil, err
		}
		for zoneID, zone := range managedZones {
			err = p.client.ForEachRecordSet(ctx, zoneID,
				func(recordSet *recordsets.RecordSet) error {
					if recordSet.Type != endpoint.RecordTypeA && recordSet.Type != endpoint.RecordTypeTXT && recordSet.Type != endpoint.RecordTypeCNAME {
						return nil
					}

					ep := endpoint.NewEndpointWithTTL(recordSet.Name, recordSet.Type, endpoint.TTL(recordSet.TTL), recordSet.Records...)
					ep.Labels[dnsRecordSetID] = recordSet.ID
					ep.Labels[dnsZoneID] = recordSet.ZoneID
					ep.Labels[dnsZoneType] = zone.zoneType
					ep.Labels[dnsOriginalRecords] = strings.Join(recordSet.Records, "\000")
					if zone.zoneType == ZoneTypePrivate {
						ep.ProviderSpecific = endpoint.ProviderSpecific{
							{
								Name:  zoneTypeProviderSpecificKey,
								Value: zone.zoneType,
							},
						}
					}
					result = append(result, ep)

					return nil
				},
			)
			if err != nil {
				return nil, err
			}
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
	zoneType    string
}

// adds endpoint into recordset aggregation, loading original values from endpoint labels first
func addEndpoint(ep *endpoint.Endpoint, recordSets map[string]*recordSet, oldEndpoints []*endpoint.Endpoint, delete bool) error {
	ensureEndpointLabels(ep)

	zoneType, err := getEndpointZoneType(ep)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s/%s/%s", ep.DNSName, ep.RecordType, zoneType)
	rs := recordSets[key]
	if rs == nil {
		rs = &recordSet{
			dnsName:    canonicalizeDomainName(ep.DNSName),
			recordType: ep.RecordType,
			names:      make(map[string]bool),
			zoneType:   zoneType,
		}
	}

	addDNSIDLabelsFromExistingEndpoints(oldEndpoints, ep)

	if rs.zoneID == "" {
		rs.zoneID = ep.Labels[dnsZoneID]
	}
	if rs.recordSetID == "" {
		rs.recordSetID = ep.Labels[dnsRecordSetID]
	}
	rs.ttl = int(ep.RecordTTL)
	for _, rec := range strings.Split(ep.Labels[dnsOriginalRecords], "\000") {
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
	return nil
}

// addDNSIDLabelsFromExistingEndpoints adds the labels identified by the constants dnsZoneID and dnsRecordSetID
// to an Endpoint. Therefore, it searches all given existing endpoints for an endpoint with the same record type and record
// value. If the given Endpoint already has the labels set, they are left untouched. This fixes an issue with the
// TXTRegistry which generates new TXT entries instead of updating the old ones.
func addDNSIDLabelsFromExistingEndpoints(existingEndpoints []*endpoint.Endpoint, ep *endpoint.Endpoint) {
	ensureEndpointLabels(ep)

	_, hasZoneIDLabel := ep.Labels[dnsZoneID]
	_, hasRecordSetIDLabel := ep.Labels[dnsRecordSetID]
	_, hasZoneTypeLabel := ep.Labels[dnsZoneType]
	if hasZoneIDLabel && hasRecordSetIDLabel && hasZoneTypeLabel {
		return
	}
	desiredZoneType, err := getEndpointZoneType(ep)
	if err != nil {
		return
	}
	for _, oep := range existingEndpoints {
		existingZoneType, err := getEndpointZoneType(oep)
		if err != nil {
			continue
		}
		if ep.RecordType != oep.RecordType || ep.DNSName != oep.DNSName || desiredZoneType != existingZoneType {
			continue
		}
		if !hasZoneIDLabel {
			ep.Labels[dnsZoneID] = oep.Labels[dnsZoneID]
		}
		if !hasRecordSetIDLabel {
			ep.Labels[dnsRecordSetID] = oep.Labels[dnsRecordSetID]
		}
		if !hasZoneTypeLabel {
			ep.Labels[dnsZoneType] = oep.Labels[dnsZoneType]
		}
		return
	}
}

// ApplyChanges applies a given set of changes in a given zone.
func (p dnsProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	endpoints, err := p.Records(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch active records: %w", err)
	}

	recordSets := map[string]*recordSet{}
	for _, ep := range changes.Create {
		if err := addEndpoint(ep, recordSets, endpoints, false); err != nil {
			return err
		}
	}
	for _, ep := range changes.UpdateOld {
		if err := addEndpoint(ep, recordSets, endpoints, true); err != nil {
			return err
		}
	}
	for _, ep := range changes.UpdateNew {
		if err := addEndpoint(ep, recordSets, endpoints, false); err != nil {
			return err
		}
	}
	for _, ep := range changes.Delete {
		if err := addEndpoint(ep, recordSets, endpoints, true); err != nil {
			return err
		}
	}

	managedZonesByType := map[string]map[string]managedZone{}
	for _, rs := range recordSets {
		managedZones := managedZonesByType[rs.zoneType]
		if managedZones == nil {
			var err2 error
			managedZones, err2 = p.getZones(ctx, rs.zoneType)
			if err2 != nil {
				if err == nil {
					err = err2
				}
				continue
			}
			managedZonesByType[rs.zoneType] = managedZones
		}
		if err2 := p.upsertRecordSet(ctx, rs, managedZones); err == nil {
			err = err2
		}
	}
	return err
}

// apply recordset changes by inserting/updating/deleting recordsets
func (p dnsProvider) upsertRecordSet(ctx context.Context, rs *recordSet, managedZones map[string]managedZone) error {
	if rs.zoneID == "" {
		rs.zoneID = getHostZoneID(rs.dnsName, managedZones)
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
			TTL:     rs.ttl,
		}
		log.Infof("Updating records: %s/%s: %s", rs.dnsName, rs.recordType, strings.Join(records, ","))
		if p.dryRun {
			return nil
		}
		return p.client.UpdateRecordSet(ctx, rs.zoneID, rs.recordSetID, opts)
	}
}
