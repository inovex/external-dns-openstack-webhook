/*
Copyright 2017 The Kubernetes Authors.

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
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/recordsets"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/dns/v2/zones"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

var lastGeneratedDNSID int32

func generateDNSID() string {
	return fmt.Sprintf("id-%d", atomic.AddInt32(&lastGeneratedDNSID, 1))
}

type fakeDNSClient struct {
	managedZones map[string]*struct {
		zone       *zones.Zone
		recordSets map[string]*recordsets.RecordSet
	}
}

func (c fakeDNSClient) AddZone(ctx context.Context, zone zones.Zone) string {
	if zone.ID == "" {
		zone.ID = zone.Name
	}
	c.managedZones[zone.ID] = &struct {
		zone       *zones.Zone
		recordSets map[string]*recordsets.RecordSet
	}{
		zone:       &zone,
		recordSets: make(map[string]*recordsets.RecordSet),
	}
	return zone.ID
}

func (c fakeDNSClient) ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error {
	for _, zone := range c.managedZones {
		if err := handler(zone.zone); err != nil {
			return err
		}
	}
	return nil
}

func (c fakeDNSClient) ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	zone := c.managedZones[zoneID]
	if zone == nil {
		return fmt.Errorf("unknown zone %s", zoneID)
	}
	for _, recordSet := range zone.recordSets {
		if err := handler(recordSet); err != nil {
			return err
		}
	}
	return nil
}

func (c fakeDNSClient) CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error) {
	zone := c.managedZones[zoneID]
	if zone == nil {
		return "", fmt.Errorf("unknown zone %s", zoneID)
	}
	rs := &recordsets.RecordSet{
		ID:          generateDNSID(),
		ZoneID:      zoneID,
		Name:        opts.Name,
		Description: opts.Description,
		Records:     opts.Records,
		TTL:         opts.TTL,
		Type:        opts.Type,
	}
	zone.recordSets[rs.ID] = rs
	return rs.ID, nil
}

func (c fakeDNSClient) UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	zone := c.managedZones[zoneID]
	if zone == nil {
		return fmt.Errorf("unknown zone %s", zoneID)
	}
	rs := zone.recordSets[recordSetID]
	if rs == nil {
		return fmt.Errorf("unknown record-set %s", recordSetID)
	}
	rs.Description = opts.Description
	rs.TTL = opts.TTL

	rs.Records = opts.Records
	return nil
}

func (c fakeDNSClient) DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error {
	zone := c.managedZones[zoneID]
	if zone == nil {
		return fmt.Errorf("unknown zone %s", zoneID)
	}
	delete(zone.recordSets, recordSetID)
	return nil
}

func (c fakeDNSClient) ToProvider() provider.Provider {
	return c.ToProviderWithZoneType(ZoneTypePublic)
}

func (c fakeDNSClient) ToProviderWithZoneType(zoneType string) provider.Provider {
	return &dnsProvider{
		client:       c,
		domainFilter: endpoint.DomainFilter{},
		zoneType:     zoneType,
	}
}

func newFakeDNSClient() *fakeDNSClient {
	return &fakeDNSClient{
		make(map[string]*struct {
			zone       *zones.Zone
			recordSets map[string]*recordsets.RecordSet
		}),
	}
}

func TestNewDNSProvider(t *testing.T) {

	// This simply fakes the existence of a reachable DNS API endpoint (v2)
	tsDNS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
      "versions": [
        {
          "id": "v2",
          "links": [
            {
              "href": "https://dns.fakecloud.local/v2",
              "rel": "self"
            },
            {
              "href": "https://docs.openstack.org/api-ref/dns",
              "rel": "help"
            }
          ],
          "status": "SUPPORTED",
          "updated": "2022-06-29T00:00:00Z"
        },
        {
          "id": "v2.0",
          "links": [
            {
              "href": "https://dns.fakecloud.local/v2",
              "rel": "self"
            },
            {
              "href": "https://docs.openstack.org/api-ref/dns",
              "rel": "help"
            }
          ],
          "status": "SUPPORTED",
          "updated": "2022-06-29T00:00:00Z"
        },
        {
          "id": "v2.1",
          "links": [
            {
              "href": "hhttps://dns.fakecloud.local/v2",
              "rel": "self"
            },
            {
              "href": "https://docs.openstack.org/api-ref/dns",
              "rel": "help"
            }
          ],
          "status": "CURRENT",
          "updated": "2023-01-25T00:00:00Z"
        }
		}`))
	}))
	defer tsDNS.Close()

	// This fakes the catalog response from IAM including the DNS endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Subject-Token", "test-token")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
		  "token": {
		    "expires_at": "2030-01-01T00:00:00Z",
		    "project": {
		      "id": "project-id",
		      "name": "project-name",
		      "domain": {
		        "id": "domain-id",
		        "name": "Default"
		      }
		    },
		    "user": {
		      "id": "user-id",
		      "name": "dns-user",
		      "domain": {
		        "id": "domain-id",
		        "name": "Default"
		      }
		    },
		    "catalog": [
		      {
		        "id": "9615c2dfac3b4b19935226d4c9d4afce",
		        "name": "dns",
		        "type": "dns",
		        "endpoints": [
		          {
		            "id": "3d3cc3a273b54d0490ac43d6572e4c48",
		            "region": "RegionOne",
		            "region_id": "RegionOne",
		            "interface": "public",
		            "url": "` + tsDNS.URL + `/v2"
		          }
		        ]
		      }
		    ]
		  }
		}`))
	}))
	defer ts.Close()

	tmpcloudsyaml, err := os.CreateTemp("", "clouds.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpcloudsyaml.Name())

	fmt.Fprintf(tmpcloudsyaml, `
clouds:
  unittest:
    auth:
      auth_url: %s/v3
      username: dns-user
      password: fakefake
      project_name: project-name
      user_domain_name: Default
      project_domain_name: Default
    region_name: RegionOne
    interface: public
    auth_type: password`, ts.URL)

	os.Setenv("OS_CLIENT_CONFIG_FILE", tmpcloudsyaml.Name())
	os.Setenv("OS_CLOUD", "unittest")

	if _, err := NewDNSProvider(endpoint.DomainFilter{}, ZoneTypePublic, true); err != nil {
		t.Fatalf("Failed to initialize DNS provider: %s", err)
	}
}

func TestDNSRecords(t *testing.T) {
	client := newFakeDNSClient()
	ctx := context.TODO()

	zone1ID := client.AddZone(ctx, zones.Zone{
		Name:     "example.com.",
		ZoneType: ZoneTypePublic,
		Status:   "ACTIVE",
	})
	rs11ID, _ := client.CreateRecordSet(ctx, zone1ID, recordsets.CreateOpts{
		Name:    "www.example.com.",
		Type:    endpoint.RecordTypeA,
		Records: []string{"10.1.1.1"},
	})
	rs12ID, _ := client.CreateRecordSet(ctx, zone1ID, recordsets.CreateOpts{
		Name:    "www.example.com.",
		Type:    endpoint.RecordTypeTXT,
		Records: []string{"text1"},
	})
	client.CreateRecordSet(ctx, zone1ID, recordsets.CreateOpts{
		Name:    "xxx.example.com.",
		Type:    "SRV",
		Records: []string{"http://test.com:1234"},
	})
	rs14ID, _ := client.CreateRecordSet(ctx, zone1ID, recordsets.CreateOpts{
		Name:    "ftp.example.com.",
		Type:    endpoint.RecordTypeA,
		TTL:     120,
		Records: []string{"10.1.1.2"},
	})

	zone2ID := client.AddZone(ctx, zones.Zone{
		Name:     "test.net.",
		ZoneType: ZoneTypePublic,
		Status:   "ACTIVE",
	})
	rs21ID, _ := client.CreateRecordSet(ctx, zone2ID, recordsets.CreateOpts{
		Name:    "srv.test.net.",
		Type:    endpoint.RecordTypeA,
		Records: []string{"10.2.1.1", "10.2.1.2"},
	})
	rs22ID, _ := client.CreateRecordSet(ctx, zone2ID, recordsets.CreateOpts{
		Name:    "db.test.net.",
		Type:    endpoint.RecordTypeCNAME,
		Records: []string{"sql.test.net."},
	})
	expected := []*endpoint.Endpoint{
		{
			DNSName:    "www.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.1"},
			Labels: map[string]string{
				dnsRecordSetID:     rs11ID,
				dnsZoneID:          zone1ID,
				dnsOriginalRecords: "10.1.1.1",
			},
		},
		{
			DNSName:    "www.example.com",
			RecordType: endpoint.RecordTypeTXT,
			Targets:    endpoint.Targets{"text1"},
			Labels: map[string]string{
				dnsRecordSetID:     rs12ID,
				dnsZoneID:          zone1ID,
				dnsOriginalRecords: "text1",
			},
		},
		{
			DNSName:    "ftp.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.2"},
			RecordTTL:  120,
			Labels: map[string]string{
				dnsRecordSetID:     rs14ID,
				dnsZoneID:          zone1ID,
				dnsOriginalRecords: "10.1.1.2",
			},
		},
		{
			DNSName:    "srv.test.net",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.2.1.1", "10.2.1.2"},
			Labels: map[string]string{
				dnsRecordSetID:     rs21ID,
				dnsZoneID:          zone2ID,
				dnsOriginalRecords: "10.2.1.1\00010.2.1.2",
			},
		},
		{
			DNSName:    "db.test.net",
			RecordType: endpoint.RecordTypeCNAME,
			Targets:    endpoint.Targets{"sql.test.net"},
			Labels: map[string]string{
				dnsRecordSetID:     rs22ID,
				dnsZoneID:          zone2ID,
				dnsOriginalRecords: "sql.test.net.",
			},
		},
	}

	endpoints, err := client.ToProvider().Records(context.Background())
	if err != nil {
		t.Fatal(err)
	}
out:
	for _, ep := range endpoints {
		for i, ex := range expected {
			if reflect.DeepEqual(ep, ex) {
				expected = append(expected[:i], expected[i+1:]...)
				continue out
			}
		}
		t.Errorf("unexpected endpoint %s/%s (TTL: %d) -> %s", ep.DNSName, ep.RecordType, ep.RecordTTL, ep.Targets)
	}
	if len(expected) != 0 {
		t.Errorf("not all expected endpoints were returned. Remained: %v", expected)
	}
}

func TestDNSRecordsPrivateZones(t *testing.T) {
	client := newFakeDNSClient()
	ctx := context.TODO()

	privateZoneID := client.AddZone(ctx, zones.Zone{
		ID:       "private-example-internal",
		Name:     "example.internal.",
		ZoneType: ZoneTypePrivate,
		Status:   "ACTIVE",
	})
	publicZoneID := client.AddZone(ctx, zones.Zone{
		ID:       "public-example-internal",
		Name:     "example.internal.",
		ZoneType: ZoneTypePublic,
		Status:   "ACTIVE",
	})

	privateRecordID, _ := client.CreateRecordSet(ctx, privateZoneID, recordsets.CreateOpts{
		Name:    "api.example.internal.",
		Type:    endpoint.RecordTypeA,
		Records: []string{"10.10.0.5"},
	})
	client.CreateRecordSet(ctx, publicZoneID, recordsets.CreateOpts{
		Name:    "api.example.internal.",
		Type:    endpoint.RecordTypeA,
		Records: []string{"198.51.100.5"},
	})

	endpoints, err := client.ToProviderWithZoneType(ZoneTypePrivate).Records(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	expected := []*endpoint.Endpoint{
		{
			DNSName:    "api.example.internal",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.10.0.5"},
			Labels: map[string]string{
				dnsRecordSetID:     privateRecordID,
				dnsZoneID:          privateZoneID,
				dnsOriginalRecords: "10.10.0.5",
			},
		},
	}

	if !reflect.DeepEqual(endpoints, expected) {
		t.Fatalf("unexpected private zone records: got=%v want=%v", endpoints, expected)
	}
}

func TestDNSCreateRecords(t *testing.T) {
	client := newFakeDNSClient()
	testDNSCreateRecords(t, client)
}

func TestDNSCreateRecordsPrivateZones(t *testing.T) {
	client := newFakeDNSClient()
	ctx := context.TODO()

	publicZoneID := client.AddZone(ctx, zones.Zone{
		ID:       "public-zone",
		Name:     "example.internal.",
		ZoneType: ZoneTypePublic,
		Status:   "ACTIVE",
	})
	privateZoneID := client.AddZone(ctx, zones.Zone{
		ID:       "private-zone",
		Name:     "example.internal.",
		ZoneType: ZoneTypePrivate,
		Status:   "ACTIVE",
	})

	endpoints := []*endpoint.Endpoint{
		{
			DNSName:    "api.example.internal",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.0.0.7"},
			Labels:     map[string]string{},
		},
	}

	err := client.ToProviderWithZoneType(ZoneTypePrivate).ApplyChanges(context.Background(), &plan.Changes{Create: endpoints})
	if err != nil {
		t.Fatal(err)
	}

	privateZone := client.managedZones[privateZoneID]
	if len(privateZone.recordSets) != 1 {
		t.Fatalf("expected one private record-set, got %d", len(privateZone.recordSets))
	}

	publicZone := client.managedZones[publicZoneID]
	if len(publicZone.recordSets) != 0 {
		t.Fatalf("expected no public record-sets, got %d", len(publicZone.recordSets))
	}

	for _, rs := range privateZone.recordSets {
		if rs.ZoneID != privateZoneID || rs.Name != "api.example.internal." {
			t.Fatalf("unexpected private record-set created: %+v", rs)
		}
	}
}

func testDNSCreateRecords(t *testing.T, client *fakeDNSClient) []*recordsets.RecordSet {
	ctx := context.TODO()
	for i, zoneName := range []string{"example.com.", "test.net."} {
		client.AddZone(ctx, zones.Zone{
			ID:       fmt.Sprintf("zone-%d", i+1),
			Name:     zoneName,
			ZoneType: ZoneTypePublic,
			Status:   "ACTIVE",
		})
	}

	_, err := client.CreateRecordSet(ctx, "zone-1", recordsets.CreateOpts{
		Name:        "www.example.com.",
		Description: "",
		Records:     []string{"foo"},
		TTL:         60,
		Type:        endpoint.RecordTypeTXT,
	})

	if err != nil {
		t.Fatal("failed to prefill records")
	}

	endpoints := []*endpoint.Endpoint{
		{
			DNSName:    "www.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.1"},
			Labels:     map[string]string{},
		},
		{
			DNSName:    "www.example.com",
			RecordType: endpoint.RecordTypeTXT,
			Targets:    endpoint.Targets{"text1"},
			Labels:     map[string]string{},
		},
		{
			DNSName:    "ftp.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.2"},
			RecordTTL:  120,
			Labels:     map[string]string{},
		},
		{
			DNSName:    "srv.test.net",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.2.1.1"},
			Labels:     map[string]string{},
		},
		{
			DNSName:    "srv.test.net",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.2.1.2"},
			Labels:     map[string]string{},
		},
		{
			DNSName:    "db.test.net",
			RecordType: endpoint.RecordTypeCNAME,
			Targets:    endpoint.Targets{"sql.test.net"},
			Labels:     map[string]string{},
		},
	}
	expected := []*recordsets.RecordSet{
		{
			Name:    "www.example.com.",
			Type:    endpoint.RecordTypeA,
			Records: []string{"10.1.1.1"},
			ZoneID:  "zone-1",
		},
		{
			Name:    "www.example.com.",
			Type:    endpoint.RecordTypeTXT,
			Records: []string{"text1"},
			ZoneID:  "zone-1",
		},
		{
			Name:    "ftp.example.com.",
			Type:    endpoint.RecordTypeA,
			Records: []string{"10.1.1.2"},
			TTL:     120,
			ZoneID:  "zone-1",
		},
		{
			Name:    "srv.test.net.",
			Type:    endpoint.RecordTypeA,
			Records: []string{"10.2.1.1", "10.2.1.2"},
			ZoneID:  "zone-2",
		},
		{
			Name:    "db.test.net.",
			Type:    endpoint.RecordTypeCNAME,
			Records: []string{"sql.test.net."},
			ZoneID:  "zone-2",
		},
	}
	expectedCopy := make([]*recordsets.RecordSet, len(expected))
	copy(expectedCopy, expected)

	err = client.ToProvider().ApplyChanges(context.Background(), &plan.Changes{Create: endpoints})
	if err != nil {
		t.Fatal(err)
	}

	client.ForEachZone(ctx, func(zone *zones.Zone) error {
		client.ForEachRecordSet(ctx, zone.ID, func(recordSet *recordsets.RecordSet) error {
			id := recordSet.ID
			recordSet.ID = ""
			for i, ex := range expected {
				sort.Strings(recordSet.Records)
				if reflect.DeepEqual(ex, recordSet) {
					ex.ID = id
					recordSet.ID = id
					expected = append(expected[:i], expected[i+1:]...)
					return nil
				}
			}
			t.Errorf("unexpected record-set %s/%s -> %v", recordSet.Name, recordSet.Type, recordSet.Records)
			return nil
		})
		return nil
	})

	if len(expected) != 0 {
		t.Errorf("not all expected record-sets were created. Remained: %v", expected)
	}
	return expectedCopy
}

func TestDNSUpdateRecords(t *testing.T) {
	client := newFakeDNSClient()
	testDNSUpdateRecords(t, client)
}

func testDNSUpdateRecords(t *testing.T, client *fakeDNSClient) []*recordsets.RecordSet {
	expected := testDNSCreateRecords(t, client)
	ctx := context.TODO()

	updatesOld := []*endpoint.Endpoint{
		{
			DNSName:    "ftp.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.2"},
			RecordTTL:  120,
			Labels: map[string]string{
				dnsZoneID:          "zone-1",
				dnsRecordSetID:     expected[2].ID,
				dnsOriginalRecords: "10.1.1.2",
			},
		},
		{
			DNSName:    "srv.test.net.",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.2.1.2"},
			Labels: map[string]string{
				dnsZoneID:          "zone-2",
				dnsRecordSetID:     expected[3].ID,
				dnsOriginalRecords: "10.2.1.1\00010.2.1.2",
			},
		},
	}
	updatesNew := []*endpoint.Endpoint{
		{
			DNSName:    "ftp.example.com",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.3.3.1"},
			RecordTTL:  60,
			Labels: map[string]string{
				dnsZoneID:          "zone-1",
				dnsRecordSetID:     expected[2].ID,
				dnsOriginalRecords: "10.1.1.2",
			},
		},
		{
			DNSName:    "srv.test.net.",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.3.3.2"},
			Labels: map[string]string{
				dnsZoneID:          "zone-2",
				dnsRecordSetID:     expected[3].ID,
				dnsOriginalRecords: "10.2.1.1\00010.2.1.2",
			},
		},
	}
	expectedCopy := make([]*recordsets.RecordSet, len(expected))
	copy(expectedCopy, expected)

	expected[2].Records = []string{"10.3.3.1"}
	expected[2].TTL = 60
	expected[3].Records = []string{"10.2.1.1", "10.3.3.2"}

	err := client.ToProvider().ApplyChanges(context.Background(), &plan.Changes{UpdateOld: updatesOld, UpdateNew: updatesNew})
	if err != nil {
		t.Fatal(err)
	}

	client.ForEachZone(ctx, func(zone *zones.Zone) error {
		client.ForEachRecordSet(ctx, zone.ID, func(recordSet *recordsets.RecordSet) error {
			for i, ex := range expected {
				sort.Strings(recordSet.Records)
				if reflect.DeepEqual(ex, recordSet) {
					expected = append(expected[:i], expected[i+1:]...)
					return nil
				}
			}
			t.Errorf("unexpected record-set %s/%s -> %v", recordSet.Name, recordSet.Type, recordSet.Records)
			return nil
		})
		return nil
	})

	if len(expected) != 0 {
		t.Errorf("not all expected record-sets were updated. Remained: %v", expected)
	}
	return expectedCopy
}

func TestDNSDeleteRecords(t *testing.T) {
	client := newFakeDNSClient()
	testDNSDeleteRecords(t, client)
}

func testDNSDeleteRecords(t *testing.T, client *fakeDNSClient) {
	expected := testDNSUpdateRecords(t, client)
	ctx := context.TODO()

	deletes := []*endpoint.Endpoint{
		{
			DNSName:    "www.example.com.",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.1.1.1"},
			Labels: map[string]string{
				dnsZoneID:          "zone-1",
				dnsRecordSetID:     expected[0].ID,
				dnsOriginalRecords: "10.1.1.1",
			},
		},
		{
			DNSName:    "srv.test.net.",
			RecordType: endpoint.RecordTypeA,
			Targets:    endpoint.Targets{"10.2.1.1"},
			Labels: map[string]string{
				dnsZoneID:          "zone-2",
				dnsRecordSetID:     expected[3].ID,
				dnsOriginalRecords: "10.2.1.1\00010.3.3.2",
			},
		},
	}
	expected[3].Records = []string{"10.3.3.2"}
	expected = expected[1:]

	err := client.ToProvider().ApplyChanges(context.Background(), &plan.Changes{Delete: deletes})
	if err != nil {
		t.Fatal(err)
	}

	client.ForEachZone(ctx, func(zone *zones.Zone) error {
		client.ForEachRecordSet(ctx, zone.ID, func(recordSet *recordsets.RecordSet) error {
			for i, ex := range expected {
				sort.Strings(recordSet.Records)
				if reflect.DeepEqual(ex, recordSet) {
					expected = append(expected[:i], expected[i+1:]...)
					return nil
				}
			}
			t.Errorf("unexpected record-set %s/%s -> %v", recordSet.Name, recordSet.Type, recordSet.Records)
			return nil
		})
		return nil
	})

	if len(expected) != 0 {
		t.Errorf("not all expected record-sets were deleted. Remained: %v", expected)
	}
}

func TestGetHostZoneID(t *testing.T) {
	tests := []struct {
		name     string
		zones    []string
		hostname string
		want     string
	}{
		{
			name:     "no zone",
			zones:    []string{},
			hostname: "example.com.",
			want:     "",
		},
		{
			name:     "one mismatched zone",
			zones:    []string{"foo.com."},
			hostname: "example.com.",
			want:     "",
		},
		{
			name:     "one matching zone",
			zones:    []string{"example.com."},
			hostname: "example.com.",
			want:     "example.com.",
		},
		{
			name:     "one matching zone, multiple mismatched ones",
			zones:    []string{"example.com.", "foo.com.", "bar.com."},
			hostname: "example.com.",
			want:     "example.com.",
		},
		{
			name:     "should use longer of two matching zones",
			zones:    []string{"example.com.", "test.example.com."},
			hostname: "foo.test.example.com.",
			want:     "test.example.com.",
		},
		{
			name:     "should not match on suffix",
			zones:    []string{"example.com.", "test.example.com."},
			hostname: "first-test.example.com.",
			want:     "example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zoneMap := map[string]string{}
			for _, zone := range tt.zones {
				zoneMap[zone] = zone
			}
			got := getHostZoneID(tt.hostname, zoneMap)
			if got != tt.want {
				t.Errorf("got=%s, want=%s for hostname=%s and zones=%s", got, tt.want, tt.hostname, tt.zones)
			}
		})
	}
}
