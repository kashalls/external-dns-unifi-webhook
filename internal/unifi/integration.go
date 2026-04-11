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
func (c *httpClient) createIntegrationPolicy(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) ([]*DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	var createdRecords []*DNSRecord

	for _, target := range endpoint.Targets {
		policy, err := endpointToDNSPolicy(endpoint, target)
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
func (c *httpClient) deleteIntegrationPolicy(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) error {
	m := metrics.Get()
	start := time.Now()

	records, err := c.getIntegrationPolicies(ctx)
	if err != nil {
		m.RecordUniFiAPICall("delete_integration_policy", time.Since(start), 0, err)

		return errors.Wrap(err, "failed to fetch policies before deletion")
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key != endpoint.DNSName || record.RecordType != endpoint.RecordType {
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
func policyToDNSRecord(policy *DNSPolicy) (DNSRecord, bool) {
	switch policy.Type {
	case integrationTypeA:
		return policyToARecord(policy), true
	case integrationTypeAAAA:
		return policyToAAAARecord(policy), true
	case integrationTypeCNAME:
		return policyToCNAMERecord(policy), true
	case integrationTypeMX:
		return policyToMXRecord(policy), true
	case integrationTypeTXT:
		return policyToTXTRecord(policy), true
	case integrationTypeSRV:
		return policyToSRVRecord(policy), true
	default:
		// FORWARD_DOMAIN and any future types are silently skipped.
		return DNSRecord{}, false
	}
}

func policyToARecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, Key: policy.Domain, RecordType: recordTypeA}
	if policy.IPv4Address != nil {
		rec.Value = *policy.IPv4Address
	}
	if policy.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*policy.TTLSeconds)
	}

	return rec
}

func policyToAAAARecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, Key: policy.Domain, RecordType: recordTypeAAAA}
	if policy.IPv6Address != nil {
		rec.Value = *policy.IPv6Address
	}
	if policy.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*policy.TTLSeconds)
	}

	return rec
}

func policyToCNAMERecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, Key: policy.Domain, RecordType: recordTypeCNAME}
	if policy.TargetDomain != nil {
		rec.Value = *policy.TargetDomain
	}
	if policy.TTLSeconds != nil {
		rec.TTL = externaldnsendpoint.TTL(*policy.TTLSeconds)
	}

	return rec
}

func policyToMXRecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, Key: policy.Domain, RecordType: recordTypeMX}
	if policy.Priority != nil && policy.MailServerDomain != nil {
		rec.Value = fmt.Sprintf("%d %s", *policy.Priority, *policy.MailServerDomain)
	}

	return rec
}

func policyToTXTRecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, Key: policy.Domain, RecordType: recordTypeTXT}
	if policy.Text != nil {
		rec.Value = *policy.Text
	}

	return rec
}

func policyToSRVRecord(policy *DNSPolicy) DNSRecord {
	rec := DNSRecord{ID: policy.ID, Enabled: policy.Enabled, RecordType: recordTypeSRV}
	// Reconstruct the canonical external-dns SRV name: _service._protocol.domain
	if policy.Service != nil && policy.Protocol != nil {
		rec.Key = fmt.Sprintf("%s.%s.%s", *policy.Service, *policy.Protocol, policy.Domain)
	} else {
		rec.Key = policy.Domain
	}
	// Encode as "priority weight port target" matching the static-dns transformation.
	if policy.Priority != nil && policy.Weight != nil && policy.Port != nil && policy.ServerDomain != nil {
		rec.Value = fmt.Sprintf("%d %d %d %s", *policy.Priority, *policy.Weight, *policy.Port, *policy.ServerDomain)
	}

	return rec
}

// endpointToDNSPolicy converts an external-dns Endpoint + single target string
// into an Integration API DNSPolicy create/update request body.
func endpointToDNSPolicy(endpoint *externaldnsendpoint.Endpoint, target string) (DNSPolicy, error) {
	policy := DNSPolicy{
		Enabled: true,
		Domain:  endpoint.DNSName,
	}

	ttl := int(endpoint.RecordTTL)

	switch endpoint.RecordType {
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
		err := fillSRVPolicy(endpoint, target, &policy)
		if err != nil {
			return DNSPolicy{}, err
		}

	default:
		return DNSPolicy{}, fmt.Errorf("record type %q is not supported by the Integration API", endpoint.RecordType)
	}

	return policy, nil
}

// fillSRVPolicy populates the SRV-specific fields of a DNSPolicy from the
// external-dns endpoint DNS name ("_service._protocol.domain") and target
// ("priority weight port serverDomain").
func fillSRVPolicy(endpoint *externaldnsendpoint.Endpoint, target string, policy *DNSPolicy) error {
	service, protocol, domain, err := parseSRVDNSName(endpoint.DNSName)
	if err != nil {
		return err
	}
	policy.Domain = domain
	policy.Service = strPtr(service)
	policy.Protocol = strPtr(protocol)

	var priority, weight, port int
	var serverDomain string
	_, err = fmt.Sscanf(target, "%d %d %d %s", &priority, &weight, &port, &serverDomain)
	if err != nil {
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
	_, scanErr := fmt.Sscanf(target, "%d %s", &priority, &hostname)
	if scanErr != nil {
		return 0, "", NewDataError("parse", "MX record target: "+target, scanErr)
	}

	return priority, hostname, nil
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string {
	return &s
}
