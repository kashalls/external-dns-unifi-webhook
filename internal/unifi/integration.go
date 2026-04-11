package unifi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// Integration API record type discriminator values.
const (
	integrationTypeA     = "A_RECORD"
	integrationTypeAAAA  = "AAAA_RECORD"
	integrationTypeCNAME = "CNAME_RECORD"
	integrationTypeMX    = "MX_RECORD"
	integrationTypeTXT   = "TXT_RECORD"
	integrationTypeSRV   = "SRV_RECORD"

	integrationMaxPageLimit = 200
	srvDNSNameParts         = 3
)

// getIntegrationPolicies fetches all DNS policies from the Integration API,
// handling pagination transparently, and converts them to the shared DNSRecord
// format so the rest of the provider layer needs no changes.
func (c *httpClient) getIntegrationPolicies(ctx context.Context) ([]DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	var allRecords []DNSRecord
	var offset int64

	for {
		url := fmt.Sprintf("%s?offset=%d&limit=%d",
			FormatURL(c.ClientURLs.Records, c.Host, c.Site),
			offset,
			integrationMaxPageLimit,
		)

		resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			m.RecordUniFiAPICall("get_integration_policies", time.Since(start), 0, err)

			return nil, errors.Wrap(err, "failed to fetch DNS policies from UniFi Integration API")
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			m.RecordUniFiAPICall("get_integration_policies", time.Since(start), 0, err)

			return nil, NewDataError("read", "integration policies response body", err)
		}

		var page DNSPolicyPage
		err = json.Unmarshal(bodyBytes, &page)
		if err != nil {
			m.RecordUniFiAPICall("get_integration_policies", time.Since(start), len(bodyBytes), err)

			return nil, NewDataError("unmarshal", "DNS policies page", err)
		}

		for i := range page.Data {
			record, ok := policyToDNSRecord(&page.Data[i])
			if ok {
				allRecords = append(allRecords, record)
			}
		}

		offset += int64(page.Count)
		if page.Count == 0 || offset >= page.TotalCount {
			break
		}
	}

	m.RecordUniFiAPICall("get_integration_policies", time.Since(start), 0, nil)
	log.Debug("fetched integration policies", "count", len(allRecords))

	return allRecords, nil
}

// createIntegrationPolicy creates one policy per target via the Integration API.
func (c *httpClient) createIntegrationPolicy(ctx context.Context, ep *externaldnsendpoint.Endpoint) ([]*DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	var createdRecords []*DNSRecord

	for _, target := range ep.Targets {
		policy, err := endpointToDNSPolicy(ep, target)
		if err != nil {
			m.RecordUniFiAPICall("create_integration_policy", time.Since(start), 0, err)

			return nil, err
		}

		jsonBody, err := json.Marshal(policy)
		if err != nil {
			m.RecordUniFiAPICall("create_integration_policy", time.Since(start), 0, err)

			return nil, NewDataError("marshal", "DNS policy", err)
		}

		resp, err := c.doRequest(
			ctx,
			http.MethodPost,
			FormatURL(c.ClientURLs.Records, c.Host, c.Site),
			bytes.NewReader(jsonBody),
		)
		if err != nil {
			m.RecordUniFiAPICall("create_integration_policy", time.Since(start), 0, err)

			return nil, errors.Wrap(err, "failed to create DNS policy")
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			m.RecordUniFiAPICall("create_integration_policy", time.Since(start), 0, err)

			return nil, NewDataError("read", "create policy response body", err)
		}

		var createdPolicy DNSPolicy
		err = json.Unmarshal(bodyBytes, &createdPolicy)
		if err != nil {
			m.RecordUniFiAPICall("create_integration_policy", time.Since(start), len(bodyBytes), err)

			return nil, NewDataError("unmarshal", "created DNS policy", err)
		}

		record, ok := policyToDNSRecord(&createdPolicy)
		if ok {
			createdRecords = append(createdRecords, &record)
			log.Debug("created integration policy", "key", record.Key, "type", record.RecordType, "target", record.Value)
		}
	}

	m.RecordUniFiAPICall("create_integration_policy", time.Since(start), 0, nil)

	return createdRecords, nil
}

// deleteIntegrationPolicy deletes every policy whose key and record type match
// the given endpoint, using the same fetch-then-delete pattern as the static-dns path.
func (c *httpClient) deleteIntegrationPolicy(ctx context.Context, ep *externaldnsendpoint.Endpoint) error {
	m := metrics.Get()
	start := time.Now()

	records, err := c.getIntegrationPolicies(ctx)
	if err != nil {
		m.RecordUniFiAPICall("delete_integration_policy", time.Since(start), 0, err)

		return errors.Wrap(err, "failed to fetch policies before deletion")
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key != ep.DNSName || record.RecordType != ep.RecordType {
			continue
		}

		deleteURL := FormatURL(c.ClientURLs.Records, c.Host, c.Site, record.ID)
		resp, err := c.doRequest(ctx, http.MethodDelete, deleteURL, nil)
		if err != nil {
			deleteErrors = append(deleteErrors, err)
		} else {
			_ = resp.Body.Close()
			log.Debug("deleted integration policy", "key", record.Key, "type", record.RecordType, "target", record.Value)
		}
	}

	duration := time.Since(start)
	if len(deleteErrors) > 0 {
		combined := errors.Newf("failed to delete %d integration policies", len(deleteErrors))
		for _, e := range deleteErrors {
			combined = errors.Wrap(e, combined.Error())
		}
		m.RecordUniFiAPICall("delete_integration_policy", duration, 0, combined)

		return combined
	}

	m.RecordUniFiAPICall("delete_integration_policy", duration, 0, nil)

	return nil
}

// policyToDNSRecord converts an Integration API DNSPolicy to the shared DNSRecord
// format. Returns (record, false) for unsupported types (e.g. FORWARD_DOMAIN) so
// the caller can skip them cleanly.
func policyToDNSRecord(p *DNSPolicy) (DNSRecord, bool) {
	switch p.Type {
	case integrationTypeA:
		return policyToARecord(p), true
	case integrationTypeAAAA:
		return policyToAAAARecord(p), true
	case integrationTypeCNAME:
		return policyToCNAMERecord(p), true
	case integrationTypeMX:
		return policyToMXRecord(p), true
	case integrationTypeTXT:
		return policyToTXTRecord(p), true
	case integrationTypeSRV:
		return policyToSRVRecord(p), true
	default:
		// FORWARD_DOMAIN and any future types are silently skipped.
		return DNSRecord{}, false
	}
}

func policyToARecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, Key: p.Domain, RecordType: recordTypeA}
	if p.IPv4Address != nil {
		rec.Value = *p.IPv4Address
	}
	if p.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*p.TTLSeconds)
	}

	return rec
}

func policyToAAAARecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, Key: p.Domain, RecordType: recordTypeAAAA}
	if p.IPv6Address != nil {
		rec.Value = *p.IPv6Address
	}
	if p.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*p.TTLSeconds)
	}

	return rec
}

func policyToCNAMERecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, Key: p.Domain, RecordType: recordTypeCNAME}
	if p.TargetDomain != nil {
		rec.Value = *p.TargetDomain
	}
	if p.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*p.TTLSeconds)
	}

	return rec
}

func policyToMXRecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, Key: p.Domain, RecordType: recordTypeMX}
	if p.Priority != nil && p.MailServerDomain != nil {
		rec.Value = fmt.Sprintf("%d %s", *p.Priority, *p.MailServerDomain)
	}

	return rec
}

func policyToTXTRecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, Key: p.Domain, RecordType: recordTypeTXT}
	if p.Text != nil {
		rec.Value = *p.Text
	}

	return rec
}

func policyToSRVRecord(p *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: p.ID, Enabled: p.Enabled, RecordType: recordTypeSRV}
	// Reconstruct the canonical external-dns SRV name: _service._protocol.domain
	if p.Service != nil && p.Protocol != nil {
		rec.Key = fmt.Sprintf("%s.%s.%s", *p.Service, *p.Protocol, p.Domain)
	} else {
		rec.Key = p.Domain
	}
	// Encode as "priority weight port target" matching the static-dns transformation.
	if p.Priority != nil && p.Weight != nil && p.Port != nil && p.ServerDomain != nil {
		rec.Value = fmt.Sprintf("%d %d %d %s", *p.Priority, *p.Weight, *p.Port, *p.ServerDomain)
	}

	return rec
}

// endpointToDNSPolicy converts an external-dns Endpoint + single target string
// into an Integration API DNSPolicy create/update request body.
func endpointToDNSPolicy(ep *externaldnsendpoint.Endpoint, target string) (DNSPolicy, error) {
	policy := DNSPolicy{
		Enabled: true,
		Domain:  ep.DNSName,
	}

	ttl := int(ep.RecordTTL)

	switch ep.RecordType {
	case recordTypeA:
		policy.Type = integrationTypeA
		policy.IPv4Address = strPtr(target)
		policy.TTLSeconds = &ttl

	case recordTypeAAAA:
		policy.Type = integrationTypeAAAA
		policy.IPv6Address = strPtr(target)
		policy.TTLSeconds = &ttl

	case recordTypeCNAME:
		policy.Type = integrationTypeCNAME
		policy.TargetDomain = strPtr(target)
		policy.TTLSeconds = &ttl

	case recordTypeMX:
		policy.Type = integrationTypeMX
		priority, hostname, err := parseMXTarget(target)
		if err != nil {
			return DNSPolicy{}, err
		}
		policy.Priority = &priority
		policy.MailServerDomain = strPtr(hostname)

	case recordTypeTXT:
		policy.Type = integrationTypeTXT
		policy.Text = strPtr(target)

	case recordTypeSRV:
		policy.Type = integrationTypeSRV
		if err := fillSRVPolicy(ep, target, &policy); err != nil {
			return DNSPolicy{}, err
		}

	default:
		return DNSPolicy{}, errors.Newf("record type %q is not supported by the Integration API", ep.RecordType)
	}

	return policy, nil
}

// fillSRVPolicy populates the SRV-specific fields of a DNSPolicy from the
// external-dns endpoint DNS name ("_service._protocol.domain") and target
// ("priority weight port serverDomain").
func fillSRVPolicy(ep *externaldnsendpoint.Endpoint, target string, policy *DNSPolicy) error {
	service, protocol, domain, err := parseSRVDNSName(ep.DNSName)
	if err != nil {
		return err
	}
	policy.Domain = domain
	policy.Service = strPtr(service)
	policy.Protocol = strPtr(protocol)

	var priority, weight, port int
	var serverDomain string
	if _, err := fmt.Sscanf(target, "%d %d %d %s", &priority, &weight, &port, &serverDomain); err != nil {
		return NewDataError("parse", "SRV target for integration API", err)
	}
	policy.Priority = &priority
	policy.Weight = &weight
	policy.Port = &port
	policy.ServerDomain = strPtr(serverDomain)

	return nil
}

// parseSRVDNSName splits "_service._protocol.domain" into its three components.
// Both service and protocol must start with an underscore per RFC 2782.
func parseSRVDNSName(name string) (service, protocol, domain string, err error) {
	parts := strings.SplitN(name, ".", srvDNSNameParts)
	if len(parts) != srvDNSNameParts || !strings.HasPrefix(parts[0], "_") || !strings.HasPrefix(parts[1], "_") {
		return "", "", "", NewDataError("parse", "SRV DNS name: "+name, nil)
	}

	return parts[0], parts[1], parts[2], nil
}

// parseMXTarget splits "priority hostname" (e.g. "10 mail.example.com") for MX records.
func parseMXTarget(target string) (priority int, hostname string, err error) {
	if _, scanErr := fmt.Sscanf(target, "%d %s", &priority, &hostname); scanErr != nil {
		return 0, "", NewDataError("parse", "MX record target: "+target, scanErr)
	}

	return priority, hostname, nil
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string {
	return &s
}
