package unifi

import (
	"fmt"

	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// recordTransformer implements the RecordTransformer interface.
type recordTransformer struct{}

// NewRecordTransformer creates a new record transformer.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func NewRecordTransformer() RecordTransformer {
	return &recordTransformer{}
}

// PrepareDNSRecord creates a DNSRecord from endpoint and target value.
func (t *recordTransformer) PrepareDNSRecord(endpoint *externaldnsendpoint.Endpoint, target string) DNSRecord {
	return DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      target,
	}
}

// ParseSRVTarget parses SRV record format and populates Priority, Weight, Port fields.
func (t *recordTransformer) ParseSRVTarget(record *DNSRecord, target string) error {
	record.Priority = new(int)
	record.Weight = new(int)
	record.Port = new(int)

	_, err := fmt.Sscanf(target, "%d %d %d %s", record.Priority, record.Weight, record.Port, &record.Value)
	if err != nil {
		return NewDataError("parse", "SRV record target", err)
	}

	return nil
}

// FormatSRVValue formats SRV record fields into the target string format
// used by external-dns (priority weight port target).
func (t *recordTransformer) FormatSRVValue(priority, weight, port int, target string) string {
	return fmt.Sprintf("%d %d %d %s", priority, weight, port, target)
}
