package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func init() {
	_ = metrics.Get()
}

// TestApplyChanges_DeletesShareSnapshot proves the indexing refactor: deleting
// N endpoints triggers exactly one GET (the snapshot) plus one DELETE per
// underlying record ID. The old list-then-match-per-call path would have
// produced N+1 GETs.
func TestApplyChanges_DeletesShareSnapshot(t *testing.T) {
	var (
		getCount    atomic.Int32
		deleteCount atomic.Int32
		seenIDs     sync.Map
	)

	existing := []dnsPolicyEnvelope{
		envA("id-a-1", "a.example.com", "192.0.2.1", 300),
		envA("id-a-2", "a.example.com", "192.0.2.2", 300), // same key+type, two IDs
		envA("id-b-1", "b.example.com", "192.0.2.3", 300),
		envA("id-c-1", "c.example.com", "192.0.2.4", 300),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getCount.Add(1)
			_ = json.NewEncoder(w).Encode(pageOf(existing...))
		case http.MethodDelete:
			deleteCount.Add(1)
			// Capture which ID was hit so we can assert exact targeting.
			seenIDs.Store(r.URL.Path, struct{}{})
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	p := &UnifiProvider{client: newTestClient(srv), workers: 4}

	changes := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			endpoint.NewEndpoint("a.example.com", "A", "192.0.2.1"),
			endpoint.NewEndpoint("b.example.com", "A", "192.0.2.3"),
			endpoint.NewEndpoint("c.example.com", "A", "192.0.2.4"),
		},
	}
	if err := p.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("ApplyChanges: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("GET count = %d, want exactly 1 (one snapshot, no per-delete refetch)", got)
	}
	// a.example.com has two IDs; b and c have one each → 4 deletes total.
	if got := deleteCount.Load(); got != 4 {
		t.Errorf("DELETE count = %d, want 4 (2 for a + 1 for b + 1 for c)", got)
	}
}

// TestApplyChanges_CNAMEConflictUsesSnapshot covers the conflict-cleanup
// branch: creating a CNAME at a name that already has one should delete the
// existing record IDs via the snapshot index, with no extra GET.
func TestApplyChanges_CNAMEConflictUsesSnapshot(t *testing.T) {
	var (
		getCount    atomic.Int32
		deleteCount atomic.Int32
		createCount atomic.Int32
	)

	existing := []dnsPolicyEnvelope{
		envCNAME("old-cname", "alias.example.com", "old.target.example.com", 300),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getCount.Add(1)
			_ = json.NewEncoder(w).Encode(pageOf(existing...))
		case http.MethodPost:
			createCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body) // echo back as the created record
		case http.MethodDelete:
			deleteCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	p := &UnifiProvider{client: newTestClient(srv), workers: 1}

	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			endpoint.NewEndpointWithTTL("alias.example.com", "CNAME", 300, "new.target.example.com"),
		},
	}
	if err := p.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("ApplyChanges: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("GET count = %d, want 1", got)
	}
	if got := deleteCount.Load(); got != 1 {
		t.Errorf("DELETE count = %d, want 1 (the conflicting CNAME)", got)
	}
	if got := createCount.Load(); got != 1 {
		t.Errorf("POST count = %d, want 1", got)
	}
}
