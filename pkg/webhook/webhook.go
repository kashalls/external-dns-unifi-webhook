package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/log"

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
	var header string
	if isContentType {
		header = r.Header.Get(contentTypeHeader)
	} else {
		header = r.Header.Get(acceptHeader)
	}

	if len(header) == 0 {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusNotAcceptable)

		msg := "client must provide "
		if isContentType {
			msg += "a content type"
		} else {
			msg += "an accept header"
		}
		err := fmt.Errorf(msg)

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

		msg := "client must provide a valid versioned media type in the "
		if isContentType {
			msg += "content type"
		} else {
			msg += "accept header"
		}

		err := fmt.Errorf(msg+": %s", err.Error())
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

	requestLog(r).Debug("requesting records")
	ctx := r.Context()
	records, err := p.provider.Records(ctx)
	if err != nil {
		requestLog(r).With(zap.Error(err)).Error("error getting records")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	requestLog(r).With(zap.Int("count", len(records))).Debug("returning records")
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
	if err := p.contentTypeHeaderCheck(w, r); err != nil {
		requestLog(r).With(zap.Error(err)).Error("content type header check failed")
		return
	}

	var changes plan.Changes
	ctx := r.Context()
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
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
	).Debug("requesting apply changes")
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
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)

		errMessage := fmt.Sprintf("failed to decode request body: %v", err)
		requestLog(r).With(zap.Error(err)).Info("failed to decode request body")
		if _, writeError := fmt.Fprint(w, errMessage); writeError != nil {
			requestLog(r).With(zap.Error(writeError)).Fatal("error writing error message to response writer")
		}
		return
	}

	log.Debug("requesting adjust endpoints count", zap.Int("endpoints", len(pve)))
	pve, err := p.provider.AdjustEndpoints(pve)
	if err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out, _ := json.Marshal(&pve)

	log.Debug("return adjust endpoints response", zap.Int("endpoints", len(pve)))

	w.Header().Set(contentTypeHeader, string(mediaTypeVersion1))
	w.Header().Set(varyHeader, contentTypeHeader)
	if _, writeError := fmt.Fprint(w, string(out)); writeError != nil {
		requestLog(r).With(zap.Error(writeError)).Fatal("error writing response")
	}
}

func (p *Webhook) Negotiate(w http.ResponseWriter, r *http.Request) {
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
