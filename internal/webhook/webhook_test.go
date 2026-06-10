package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// fakeProvider implements the external-dns provider.Provider interface for
// testing. Callers populate the fields that the test needs to exercise.
type fakeProvider struct {
	records         []*endpoint.Endpoint
	recordsErr      error
	applyErr        error
	adjustEndpoints []*endpoint.Endpoint
	adjustErr       error
	domainFilter    endpoint.DomainFilterInterface

	gotChanges *plan.Changes
}

func (f *fakeProvider) Records(_ context.Context) ([]*endpoint.Endpoint, error) {
	return f.records, f.recordsErr
}

func (f *fakeProvider) ApplyChanges(_ context.Context, changes *plan.Changes) error {
	f.gotChanges = changes

	return f.applyErr
}

func (f *fakeProvider) AdjustEndpoints(eps []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	if f.adjustEndpoints != nil {
		return f.adjustEndpoints, f.adjustErr
	}

	return eps, f.adjustErr
}

func (f *fakeProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	if f.domainFilter != nil {
		return f.domainFilter
	}
	df := endpoint.NewDomainFilter([]string{"example.com"})

	return df
}

func newRequest(method, path string, body []byte, headers map[string]string) *http.Request {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}

	return r
}

func TestRecords_HappyPath(t *testing.T) {
	t.Parallel()
	prov := &fakeProvider{
		records: []*endpoint.Endpoint{
			endpoint.NewEndpoint("a.example.com", "A", "1.2.3.4"),
		},
	}
	wh := New(prov)

	rec := httptest.NewRecorder()
	req := newRequest(http.MethodGet, "/records", nil, map[string]string{
		"Accept": string(mediaTypeVersion1),
	})

	wh.Records(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != string(mediaTypeVersion1) {
		t.Errorf("Content-Type = %q, want %q", got, mediaTypeVersion1)
	}

	var got []*endpoint.Endpoint
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].DNSName != "a.example.com" {
		t.Errorf("unexpected records body: %+v", got)
	}
}

func TestRecords_MissingAcceptHeader(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.Records(rec, newRequest(http.MethodGet, "/records", nil, nil))

	if rec.Code != http.StatusNotAcceptable {
		t.Errorf("status = %d, want 406", rec.Code)
	}
}

func TestRecords_UnsupportedAcceptHeader(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.Records(rec, newRequest(http.MethodGet, "/records", nil, map[string]string{
		"Accept": "application/json",
	}))

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", rec.Code)
	}
}

func TestRecords_ProviderError(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{recordsErr: errors.New("boom")})

	rec := httptest.NewRecorder()
	wh.Records(rec, newRequest(http.MethodGet, "/records", nil, map[string]string{
		"Accept": string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestApplyChanges_HappyPath(t *testing.T) {
	t.Parallel()
	prov := &fakeProvider{}
	wh := New(prov)

	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{endpoint.NewEndpoint("a.example.com", "A", "1.2.3.4")},
	}
	body, err := json.Marshal(changes)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	rec := httptest.NewRecorder()
	wh.ApplyChanges(rec, newRequest(http.MethodPost, "/records", body, map[string]string{
		"Content-Type": string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if prov.gotChanges == nil || len(prov.gotChanges.Create) != 1 {
		t.Errorf("provider did not receive changes: %+v", prov.gotChanges)
	}
}

func TestApplyChanges_BadJSON(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.ApplyChanges(rec, newRequest(http.MethodPost, "/records", []byte("{not json"), map[string]string{
		"Content-Type": string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestApplyChanges_ProviderError(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{applyErr: errors.New("boom")})

	body, _ := json.Marshal(&plan.Changes{})
	rec := httptest.NewRecorder()
	wh.ApplyChanges(rec, newRequest(http.MethodPost, "/records", body, map[string]string{
		"Content-Type": string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestAdjustEndpoints_HappyPath(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	in := []*endpoint.Endpoint{endpoint.NewEndpoint("a.example.com", "A", "1.2.3.4")}
	body, _ := json.Marshal(in)

	rec := httptest.NewRecorder()
	wh.AdjustEndpoints(rec, newRequest(http.MethodPost, "/adjustendpoints", body, map[string]string{
		"Content-Type": string(mediaTypeVersion1),
		"Accept":       string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var out []*endpoint.Endpoint
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(out))
	}
}

func TestAdjustEndpoints_BadJSON(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.AdjustEndpoints(rec, newRequest(http.MethodPost, "/adjustendpoints", []byte("nope"), map[string]string{
		"Content-Type": string(mediaTypeVersion1),
		"Accept":       string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestNegotiate_ReturnsDomainFilter(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.Negotiate(rec, newRequest(http.MethodGet, "/", nil, map[string]string{
		"Accept": string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != string(mediaTypeVersion1) {
		t.Errorf("Content-Type = %q, want %q", got, mediaTypeVersion1)
	}
	if !strings.Contains(rec.Body.String(), "example.com") {
		t.Errorf("response missing expected domain: %s", rec.Body.String())
	}
}

func TestNegotiate_MissingAccept(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	rec := httptest.NewRecorder()
	wh.Negotiate(rec, newRequest(http.MethodGet, "/", nil, nil))

	if rec.Code != http.StatusNotAcceptable {
		t.Errorf("status = %d, want 406", rec.Code)
	}
}

// TestAdjustEndpoints_ProviderError is the #227 regression: a provider error
// from AdjustEndpoints must be logged (matching the Records / ApplyChanges
// handlers), not silently turned into a bare 500.
func TestAdjustEndpoints_ProviderError(t *testing.T) {
	// Not parallel: captures the global slog default.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	wh := New(&fakeProvider{adjustErr: errors.New("boom")})
	body, _ := json.Marshal([]*endpoint.Endpoint{endpoint.NewEndpoint("a.example.com", "A", "1.2.3.4")})

	rec := httptest.NewRecorder()
	wh.AdjustEndpoints(rec, newRequest(http.MethodPost, "/adjustendpoints", body, map[string]string{
		"Content-Type": string(mediaTypeVersion1),
		"Accept":       string(mediaTypeVersion1),
	}))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(buf.String(), "adjusting endpoints") {
		t.Errorf("AdjustEndpoints provider error was not logged; log = %q", buf.String())
	}
}

// recordingFailWriter fails every Write and records WriteHeader codes, so a
// test can prove a handler does not attempt a (no-op, superfluous) WriteHeader
// after the response is already committed.
type recordingFailWriter struct {
	header http.Header
	codes  []int
}

func (w *recordingFailWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}

	return w.header
}
func (w *recordingFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (w *recordingFailWriter) WriteHeader(code int)      { w.codes = append(w.codes, code) }

// TestNegotiate_NoSuperfluousWriteHeaderOnEncodeError is the #228 regression:
// once the JSON encoder has started writing (committing a 200), a later
// WriteHeader(500) is a no-op that only logs a "superfluous WriteHeader"
// warning. Negotiate must not make that call.
func TestNegotiate_NoSuperfluousWriteHeaderOnEncodeError(t *testing.T) {
	t.Parallel()
	wh := New(&fakeProvider{})

	w := &recordingFailWriter{}
	wh.Negotiate(w, newRequest(http.MethodGet, "/", nil, map[string]string{
		"Accept": string(mediaTypeVersion1),
	}))

	for _, c := range w.codes {
		if c == http.StatusInternalServerError {
			t.Errorf("Negotiate called WriteHeader(500) after the response was committed (superfluous)")
		}
	}
}
