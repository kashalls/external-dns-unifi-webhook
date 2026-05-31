package unifi

import (
	"fmt"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
)

// UniFi Integration API discriminator values for /dns/policies entries.
const (
	policyTypeA       = "A_RECORD"
	policyTypeAAAA    = "AAAA_RECORD"
	policyTypeCNAME   = "CNAME_RECORD"
	policyTypeMX      = "MX_RECORD"
	policyTypeTXT     = "TXT_RECORD"
	policyTypeSRV     = "SRV_RECORD"
	policyTypeForward = "FORWARD_DOMAIN"
)

// Per-spec TTL ceilings (in seconds).
const (
	ttlMaxA     = 86400
	ttlMaxAAAA  = 86400
	ttlMaxCNAME = 604800
)

// dnsPolicyEnvelope is the wire shape for DNS Policies. Every per-type field
// is optional and omitempty — discrimination is by Type.
//
//nolint:tagliatelle // UniFi API field names cannot be changed
type dnsPolicyEnvelope struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Domain  string `json:"domain,omitempty"`

	IPv4Address      string `json:"ipv4Address,omitempty"`
	IPv6Address      string `json:"ipv6Address,omitempty"`
	TargetDomain     string `json:"targetDomain,omitempty"`
	MailServerDomain string `json:"mailServerDomain,omitempty"`
	Text             string `json:"text,omitempty"`
	ServerDomain     string `json:"serverDomain,omitempty"`
	Service          string `json:"service,omitempty"`
	Protocol         string `json:"protocol,omitempty"`
	IPAddress        string `json:"ipAddress,omitempty"`

	Port       *int `json:"port,omitempty"`
	Priority   *int `json:"priority,omitempty"`
	Weight     *int `json:"weight,omitempty"`
	TTLSeconds *int `json:"ttlSeconds,omitempty"`
}

// dnsPolicyPage is the paginated list envelope returned by GET /dns/policies.
type dnsPolicyPage struct {
	Offset     int                 `json:"offset"`
	Limit      int                 `json:"limit"`
	Count      int                 `json:"count"`
	TotalCount int                 `json:"totalCount"`
	Data       []dnsPolicyEnvelope `json:"data"`
}

// sitePage is the paginated list envelope returned by GET /sites.
type sitePage struct {
	Offset     int         `json:"offset"`
	Limit      int         `json:"limit"`
	Count      int         `json:"count"`
	TotalCount int         `json:"totalCount"`
	Data       []siteEntry `json:"data"`
}

//nolint:tagliatelle // UniFi API field names cannot be changed
type siteEntry struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// toDNSRecord converts a wire envelope into the internal DNSRecord shape.
// ok=false means the envelope should be skipped (FORWARD_DOMAIN entries or
// unknown discriminator values — we don't pretend to manage them).
func toDNSRecord(env dnsPolicyEnvelope) (DNSRecord, bool) {
	r := DNSRecord{
		ID:      env.ID,
		Enabled: env.Enabled,
		Key:     env.Domain,
	}
	if env.TTLSeconds != nil {
		r.TTL = endpoint.TTL(*env.TTLSeconds)
	}

	switch env.Type {
	case policyTypeA:
		r.RecordType = recordTypeA
		r.Value = env.IPv4Address
	case policyTypeAAAA:
		r.RecordType = recordTypeAAAA
		r.Value = env.IPv6Address
	case policyTypeCNAME:
		r.RecordType = recordTypeCNAME
		r.Value = env.TargetDomain
	case policyTypeTXT:
		r.RecordType = recordTypeTXT
		r.Value = env.Text
	case policyTypeMX:
		r.RecordType = recordTypeMX
		r.Value = formatMXValue(env.Priority, env.MailServerDomain)
	case policyTypeSRV:
		r.RecordType = recordTypeSRV
		// External-DNS expresses SRV names as _service._proto.domain.
		r.Key = joinSRVName(env.Service, env.Protocol, env.Domain)
		r.Value = formatSRVValue(env.Priority, env.Weight, env.Port, env.ServerDomain)
	default:
		// FORWARD_DOMAIN and anything else we don't manage.
		return DNSRecord{}, false
	}

	return r, true
}

// fromDNSRecord converts an internal DNSRecord into a wire envelope ready for
// POST /dns/policies. TTL is clamped per-type to the spec's documented maxima.
func fromDNSRecord(r DNSRecord) (dnsPolicyEnvelope, error) {
	env := dnsPolicyEnvelope{
		Enabled: true,
		Domain:  r.Key,
	}

	switch r.RecordType {
	case recordTypeA:
		env.Type = policyTypeA
		env.IPv4Address = r.Value
		env.TTLSeconds = new(clampTTL(int(r.TTL), ttlMaxA))
	case recordTypeAAAA:
		env.Type = policyTypeAAAA
		env.IPv6Address = r.Value
		env.TTLSeconds = new(clampTTL(int(r.TTL), ttlMaxAAAA))
	case recordTypeCNAME:
		env.Type = policyTypeCNAME
		env.TargetDomain = r.Value
		env.TTLSeconds = new(clampTTL(int(r.TTL), ttlMaxCNAME))
	case recordTypeTXT:
		env.Type = policyTypeTXT
		env.Text = r.Value
	case recordTypeMX:
		env.Type = policyTypeMX
		prio, mail, err := parseMXValue(r.Value)
		if err != nil {
			return dnsPolicyEnvelope{}, err
		}
		env.Priority = &prio
		env.MailServerDomain = mail
	case recordTypeSRV:
		env.Type = policyTypeSRV
		service, protocol, domain, err := splitSRVName(r.Key)
		if err != nil {
			return dnsPolicyEnvelope{}, err
		}
		env.Domain = domain
		env.Service = service
		env.Protocol = protocol

		prio, weight, port, server, err := parseSRVValue(r.Value)
		if err != nil {
			return dnsPolicyEnvelope{}, err
		}
		env.Priority = &prio
		env.Weight = &weight
		env.Port = &port
		env.ServerDomain = server
	default:
		return dnsPolicyEnvelope{}, NewDataError("convert", "DNS record",
			fmt.Errorf("unsupported record type %q", r.RecordType))
	}

	return env, nil
}

// clampTTL clips ttl into [0, max]. A zero TTL means "use controller default"
// to external-dns, so we don't substitute anything for unset.
func clampTTL(ttl, maxSecs int) int {
	if ttl < 0 {
		return 0
	}
	if ttl > maxSecs {
		return maxSecs
	}

	return ttl
}

// joinSRVName reassembles the external-dns SRV FQDN from the per-field
// representation. Service and Protocol already carry their leading underscore
// in the API ("_ldap", "_tcp"), so we just join with dots.
func joinSRVName(service, protocol, domain string) string {
	if service == "" || protocol == "" {
		return domain
	}

	return fmt.Sprintf("%s.%s.%s", service, protocol, domain)
}

// splitSRVName parses "_service._proto.domain.example.com" into its three
// pieces. Returns an error if the FQDN lacks the two leading "_xxx" labels.
func splitSRVName(name string) (service, protocol, domain string, err error) {
	parts := strings.SplitN(name, ".", 3)
	if len(parts) < 3 ||
		!strings.HasPrefix(parts[0], "_") ||
		!strings.HasPrefix(parts[1], "_") {
		return "", "", "", NewDataError("parse", "SRV record name",
			fmt.Errorf("expected _service._proto.domain, got %q", name))
	}

	return parts[0], parts[1], parts[2], nil
}

// formatSRVValue renders the external-dns SRV target convention. Nil pointers
// (shouldn't happen for well-formed records) are rendered as 0 to keep the
// shape consistent.
func formatSRVValue(priority, weight, port *int, server string) string {
	return fmt.Sprintf("%d %d %d %s",
		derefInt(priority), derefInt(weight), derefInt(port), server)
}

// parseSRVValue reverses formatSRVValue.
func parseSRVValue(target string) (priority, weight, port int, server string, err error) {
	if _, err := fmt.Sscanf(target, "%d %d %d %s", &priority, &weight, &port, &server); err != nil {
		return 0, 0, 0, "", NewDataError("parse", "SRV record target", err)
	}

	return priority, weight, port, server, nil
}

// formatMXValue renders the external-dns MX target convention: "priority host".
func formatMXValue(priority *int, mail string) string {
	return fmt.Sprintf("%d %s", derefInt(priority), mail)
}

// parseMXValue reverses formatMXValue.
func parseMXValue(target string) (priority int, mail string, err error) {
	if _, err := fmt.Sscanf(target, "%d %s", &priority, &mail); err != nil {
		return 0, "", NewDataError("parse", "MX record target", err)
	}

	return priority, mail, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}

	return *p
}
