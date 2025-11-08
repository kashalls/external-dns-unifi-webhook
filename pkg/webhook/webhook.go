package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"

	"go.uber.org/zap"
)

const (
	contentTypeHeader    = "Content-Type"
	contentTypePlaintext = "text/plain"
	acceptHeader         = "Accept"
	varyHeader           = "Vary"
)

// Webhook for external dns provider
type Webhook struct {
	provider provider.Provider
}

// New creates a new instance of the Webhook
func New(provider provider.Provider) *Webhook {
	p := Webhook{provider: provider}
	return &p
}

func (p *Webhook) contentTypeHeaderCheck(w http.ResponseWriter, r *http.Request) error {
	return p.headerCheck(true, w, r)
}

func (p *Webhook) acceptHeaderCheck(w http.ResponseWriter, r *http.Request) error {
	return p.headerCheck(false, w, r)
}

func (p *Webhook) headerCheck(isContentType bool, w http.ResponseWriter, r *http.Request) error {
	m := metrics.Get()
	var header string
	var headerType string
	if isContentType {
		header = r.Header.Get(contentTypeHeader)
		headerType = "content-type"
	} else {
		header = r.Header.Get(acceptHeader)
		headerType = "accept"
	}

	if len(header) == 0 {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusNotAcceptable)
		m.HTTPValidationErrorsTotal.WithLabelValues(metrics.ProviderName, headerType).Inc()

		var msg string
		if isContentType {
			msg = "client must provide a content type"
		} else {
			msg = "client must provide an accept header"
		}
		err := errors.New(msg)

		_, writeErr := fmt.Fprint(w, err.Error())
		if writeErr != nil {
			requestLog(r).With(zap.Error(writeErr)).Fatal("error writing error message to response writer")
		}
		return err
	}

	// as we support only one media type version, we can ignore the returned value
	if _, err := checkAndGetMediaTypeHeaderValue(header); err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		m.HTTPValidationErrorsTotal.WithLabelValues(metrics.ProviderName, headerType).Inc()

		msg := "client must provide a valid versioned media type in the "
		if isContentType {
			msg += "content type"
		} else {
			msg += "accept header"
		}

		err := errors.Wrap(err, msg)
		_, writeErr := fmt.Fprint(w, err.Error())
		if writeErr != nil {
			requestLog(r).With(zap.Error(writeErr)).Fatal("error writing error message to response writer")
		}
		return err
	}

	return nil
}

// Records handles the get request for records
func (p *Webhook) Records(w http.ResponseWriter, r *http.Request) {
	if err := p.acceptHeaderCheck(w, r); err != nil {
		requestLog(r).With(zap.Error(err)).Error("accept header check failed")
		return
	}

	ctx := r.Context()
	records, err := p.provider.Records(ctx)
	if err != nil {
		requestLog(r).With(zap.Error(err)).Error("error getting records")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	w.Header().Set(varyHeader, contentTypeHeader)
	err = json.NewEncoder(w).Encode(records)
	if err != nil {
		requestLog(r).With(zap.Error(err)).Error("error encoding records")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// ApplyChanges handles the post request for record changes
func (p *Webhook) ApplyChanges(w http.ResponseWriter, r *http.Request) {
	m := metrics.Get()
	if err := p.contentTypeHeaderCheck(w, r); err != nil {
		requestLog(r).With(zap.Error(err)).Error("content type header check failed")
		return
	}

	var changes plan.Changes
	ctx := r.Context()
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		m.HTTPJSONErrorsTotal.WithLabelValues(metrics.ProviderName, "/records").Inc()
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)

		errMsg := fmt.Sprintf("error decoding changes: %s", err.Error())
		if _, writeError := fmt.Fprint(w, errMsg); writeError != nil {
			requestLog(r).With(zap.Error(writeError)).Fatal("error writing error message to response writer")
		}
		requestLog(r).With(zap.Error(err)).Info(errMsg)
		return
	}

	requestLog(r).With(
		zap.Int("create", len(changes.Create)),
		zap.Int("update_old", len(changes.UpdateOld)),
		zap.Int("update_new", len(changes.UpdateNew)),
		zap.Int("delete", len(changes.Delete)),
	).Debug("executing plan changes")
	if err := p.provider.ApplyChanges(ctx, &changes); err != nil {
		requestLog(r).Error("error when applying changes", zap.Error(err))
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdjustEndpoints handles the post request for adjusting endpoints
func (p *Webhook) AdjustEndpoints(w http.ResponseWriter, r *http.Request) {
	m := metrics.Get()
	m.AdjustEndpointsTotal.WithLabelValues(metrics.ProviderName).Inc()

	if err := p.contentTypeHeaderCheck(w, r); err != nil {
		log.Error("content-type header check failed", zap.String("req_method", r.Method), zap.String("req_path", r.URL.Path))
		return
	}
	if err := p.acceptHeaderCheck(w, r); err != nil {
		log.Error("accept header check failed", zap.String("req_method", r.Method), zap.String("req_path", r.URL.Path))
		return
	}

	var pve []*endpoint.Endpoint
	if err := json.NewDecoder(r.Body).Decode(&pve); err != nil {
		m.HTTPJSONErrorsTotal.WithLabelValues(metrics.ProviderName, "/adjustendpoints").Inc()
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)

		errMessage := fmt.Sprintf("failed to decode request body: %v", err)
		requestLog(r).With(zap.Error(err)).Info("failed to decode request body")
		if _, writeError := fmt.Fprint(w, errMessage); writeError != nil {
			requestLog(r).With(zap.Error(writeError)).Fatal("error writing error message to response writer")
		}
		return
	}

	pve, err := p.provider.AdjustEndpoints(pve)
	if err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out, err := json.Marshal(&pve)
	if err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
		requestLog(r).With(zap.Error(err)).Error("failed to marshal endpoints")
		return
	}

	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	w.Header().Set(varyHeader, contentTypeHeader)
	if _, writeError := fmt.Fprint(w, string(out)); writeError != nil {
		requestLog(r).With(zap.Error(writeError)).Fatal("error writing response")
	}
}

func (p *Webhook) Negotiate(w http.ResponseWriter, r *http.Request) {
	m := metrics.Get()
	m.NegotiateTotal.WithLabelValues(metrics.ProviderName).Inc()

	if err := p.acceptHeaderCheck(w, r); err != nil {
		requestLog(r).With(zap.Error(err)).Error("accept header check failed")
		return
	}

	b, err := json.Marshal(p.provider.GetDomainFilter())
	if err != nil {
		requestLog(r).Error("failed to marshal domain filter")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	if _, writeError := w.Write(b); writeError != nil {
		requestLog(r).With(zap.Error(writeError)).Error("error writing response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func requestLog(r *http.Request) *zap.Logger {
	return log.With(zap.String("req_method", r.Method), zap.String("req_path", r.URL.Path))
}
