package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseRecorder_DefaultsToOK(t *testing.T) {
	t.Parallel()
	rec := NewResponseRecorder(httptest.NewRecorder())

	if got := rec.Status(); got != http.StatusOK {
		t.Errorf("Status() = %d, want %d before any WriteHeader", got, http.StatusOK)
	}
	if got := rec.Written(); got != 0 {
		t.Errorf("Written() = %d, want 0 before any Write", got)
	}
}

func TestResponseRecorder_CapturesStatus(t *testing.T) {
	t.Parallel()
	under := httptest.NewRecorder()
	rec := NewResponseRecorder(under)

	rec.WriteHeader(http.StatusTeapot)

	if got := rec.Status(); got != http.StatusTeapot {
		t.Errorf("Status() = %d, want %d", got, http.StatusTeapot)
	}
	if under.Code != http.StatusTeapot {
		t.Errorf("underlying status = %d, want %d (WriteHeader must pass through)", under.Code, http.StatusTeapot)
	}
}

func TestResponseRecorder_CountsBytesAndPassesThrough(t *testing.T) {
	t.Parallel()
	under := httptest.NewRecorder()
	rec := NewResponseRecorder(under)

	body := []byte("hello world")
	n, err := rec.Write(body)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(body) {
		t.Errorf("Write returned n = %d, want %d", n, len(body))
	}
	if got := rec.Written(); got != len(body) {
		t.Errorf("Written() = %d, want %d", got, len(body))
	}
	if under.Body.String() != string(body) {
		t.Errorf("underlying body = %q, want %q", under.Body.String(), body)
	}
}

func TestResponseRecorder_AccumulatesWrites(t *testing.T) {
	t.Parallel()
	rec := NewResponseRecorder(httptest.NewRecorder())

	_, _ = rec.Write([]byte("abc"))
	_, _ = rec.Write([]byte("defg"))

	if got := rec.Written(); got != 7 {
		t.Errorf("Written() = %d, want 7 across two writes", got)
	}
}
