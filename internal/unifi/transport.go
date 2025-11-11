package unifi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"

	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
)

// httpTransport implements the HTTPTransport interface.
type httpTransport struct {
	config     *Config
	httpClient *http.Client
	csrf       string
	clientURLs *ClientURLs
	metrics    MetricsRecorder
	logger     Logger
}

// NewHTTPTransport creates a new HTTP transport for UniFi controller communication.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func NewHTTPTransport(config *Config, metricsRecorder MetricsRecorder, logger Logger) (HTTPTransport, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cookie jar")
	}

	transport := &httpTransport{
		config: config,
		httpClient: &http.Client{
			Transport: &http.Transport{
				//nolint:gosec // InsecureSkipVerify is configurable via UNIFI_SKIP_TLS_VERIFY for self-signed certs
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTLSVerify},
			},
			Jar: jar,
		},
		clientURLs: &ClientURLs{
			Login:   unifiLoginPath,
			Records: unifiRecordPath,
		},
		metrics: metricsRecorder,
		logger:  logger,
	}

	if config.ExternalController {
		transport.clientURLs.Login = unifiLoginPathExternal
		transport.clientURLs.Records = unifiRecordPathExternal
	}

	// Perform initial login if using User/Password authentication
	if config.APIKey == "" {
		transport.logger.Info("UNIFI_USER and UNIFI_PASSWORD are deprecated, please switch to using UNIFI_API_KEY instead")

		err = transport.Login(context.Background())
		if err != nil {
			return nil, errors.Wrap(err, "initial login failed")
		}
	}

	return transport, nil
}

// Login authenticates with the UniFi controller using username/password (deprecated).
func (t *httpTransport) Login(ctx context.Context) error {
	jsonBody, err := json.Marshal(Login{
		Username: t.config.User,
		Password: t.config.Password,
		Remember: true,
	})
	if err != nil {
		return NewDataError("marshal", "login credentials", err)
	}

	// Perform the login request
	resp, err := t.DoRequest(
		ctx,
		http.MethodPost,
		FormatURL(t.clientURLs.Login, t.config.Host),
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		t.metrics.UniFiLoginTotal().WithLabelValues(metrics.ProviderName, "failure").Inc()
		t.metrics.UniFiConnected().WithLabelValues(metrics.ProviderName).Set(0)

		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		t.metrics.UniFiLoginTotal().WithLabelValues(metrics.ProviderName, "failure").Inc()
		t.metrics.UniFiConnected().WithLabelValues(metrics.ProviderName).Set(0)
		respBody, readErr := io.ReadAll(resp.Body)
		responseMsg := ""
		if readErr == nil {
			responseMsg = string(respBody)
		}
		t.logger.Error("login failed", "status", resp.Status, "response", responseMsg)

		return NewAuthError("login", resp.StatusCode, resp.Status, nil)
	}

	t.metrics.UniFiLoginTotal().WithLabelValues(metrics.ProviderName, "success").Inc()
	t.metrics.UniFiConnected().WithLabelValues(metrics.ProviderName).Set(1)

	// Retrieve CSRF token from the response headers
	if csrf := resp.Header.Get("X-Csrf-Token"); csrf != "" {
		t.csrf = resp.Header.Get("X-Csrf-Token")
		t.metrics.UniFiCSRFRefreshesTotal().WithLabelValues(metrics.ProviderName).Inc()
	}

	return nil
}

// DoRequest performs an HTTP request with automatic retry on 401 and CSRF token management.
func (t *httpTransport) DoRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	t.SetHeaders(req)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, NewNetworkError(method, path, err)
	}

	// TODO: Deprecation Notice - Use UNIFI_API_KEY instead
	//nolint:godox // This TODO is intentional and will remain until the deprecated auth method is removed
	if t.config.APIKey == "" {
		t.handleCSRFToken(resp)

		// If the status code is 401, re-login and retry the request
		if resp.StatusCode == http.StatusUnauthorized {
			resp, err = t.handleUnauthorized(ctx, req, method, path)
			if err != nil {
				return nil, err
			}
		}
	}

	// It is unknown at this time if the UniFi API returns anything other than 200 for these types of requests.
	if resp.StatusCode != http.StatusOK {
		return nil, t.handleErrorResponse(resp, method, path)
	}

	return resp, nil
}

// SetHeaders sets the required headers for UniFi API requests (API key or CSRF token).
func (t *httpTransport) SetHeaders(req *http.Request) {
	if t.config.APIKey != "" {
		req.Header.Set("X-Api-Key", t.config.APIKey)
	} else {
		req.Header.Set("X-Csrf-Token", t.csrf)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
}

// handleCSRFToken updates the CSRF token from response headers.
func (t *httpTransport) handleCSRFToken(resp *http.Response) {
	csrf := resp.Header.Get("X-Csrf-Token")
	if csrf == "" {
		return
	}

	if t.csrf != csrf {
		t.metrics.UniFiCSRFRefreshesTotal().WithLabelValues(metrics.ProviderName).Inc()
	}
	t.csrf = csrf
}

// handleUnauthorized handles 401 responses by re-logging in and retrying the request.
func (t *httpTransport) handleUnauthorized(ctx context.Context, req *http.Request, method, path string) (*http.Response, error) {
	t.metrics.UniFiReloginTotal().WithLabelValues(metrics.ProviderName).Inc()

	t.logger.Debug("received 401 unauthorized, attempting to re-login")

	err := t.Login(ctx)
	if err != nil {
		t.logger.Error("re-login failed", "error", err)

		return nil, errors.Wrap(err, "re-login after 401 failed")
	}

	// Update the headers with new CSRF token
	t.SetHeaders(req)

	// Retry the request
	t.logger.Debug("retrying request after re-login")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.logger.Error("Retry request failed", "error", err)

		return nil, NewNetworkError(method+" (retry)", path, err)
	}

	return resp, nil
}

// handleErrorResponse processes non-200 status codes and returns appropriate errors.
func (t *httpTransport) handleErrorResponse(resp *http.Response, method, path string) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, errorBodyBufferSize))
	if err != nil {
		return NewDataError("read", "error response body", err)
	}

	var apiError UnifiErrorResponse
	err = json.Unmarshal(bodyBytes, &apiError)
	if err != nil {
		return NewDataError("unmarshal", "API error response", err)
	}

	return NewAPIError(method, path, resp.StatusCode, apiError.Message)
}

// GetClientURLs returns the ClientURLs for this transport.
func (t *httpTransport) GetClientURLs() *ClientURLs {
	return t.clientURLs
}

// GetConfig returns the Config for this transport.
func (t *httpTransport) GetConfig() *Config {
	return t.config
}
