package hostinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"setec-manager/internal/hosting"
)

const (
	defaultBaseURL = "https://developers.hostinger.com"
	maxRetries     = 3
)

// APIError represents an error response from the Hostinger API.
type APIError struct {
	StatusCode    int    `json:"-"`
	Message       string `json:"error"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

func (e *APIError) Error() string {
	if e.CorrelationID != "" {
		return fmt.Sprintf("hostinger API error %d: %s (correlation_id: %s)", e.StatusCode, e.Message, e.CorrelationID)
	}
	return fmt.Sprintf("hostinger API error %d: %s", e.StatusCode, e.Message)
}

// Client is the Hostinger API client. It implements hosting.Provider.
type Client struct {
	apiToken   string
	httpClient *http.Client
	baseURL    string
}

// Compile-time check that Client implements hosting.Provider.
var _ hosting.Provider = (*Client)(nil)

// New creates a new Hostinger API client with the given bearer token.
func New(token string) *Client {
	return &Client{
		apiToken: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: defaultBaseURL,
	}
}

// Name returns the provider identifier.
func (c *Client) Name() string { return "hostinger" }

// DisplayName returns the human-readable provider name.
func (c *Client) DisplayName() string { return "Hostinger" }

// Configure applies the given configuration to the client.
func (c *Client) Configure(cfg hosting.ProviderConfig) error {
	if cfg.APIKey == "" {
		return fmt.Errorf("hostinger: API key is required")
	}
	c.apiToken = cfg.APIKey
	if cfg.BaseURL != "" {
		c.baseURL = cfg.BaseURL
	}
	return nil
}

// TestConnection verifies the API token by making a lightweight API call.
func (c *Client) TestConnection() error {
	_, err := c.ListDomains()
	return err
}

// doRequest executes an HTTP request against the Hostinger API.
// body may be nil for requests with no body. result may be nil if the
// response body should be discarded.
func (c *Client) doRequest(method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var rawBody []byte
	if body != nil {
		var err error
		rawBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var bodyReader io.Reader
		if rawBody != nil {
			bodyReader = bytes.NewReader(rawBody)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("execute request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}

		// Handle rate limiting with retry.
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt < maxRetries {
				retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
				time.Sleep(retryAfter)
				lastErr = &APIError{StatusCode: 429, Message: "rate limited"}
				continue
			}
			return &APIError{StatusCode: 429, Message: "rate limited after retries"}
		}

		// Handle error responses.
		if resp.StatusCode >= 400 {
			apiErr := &APIError{StatusCode: resp.StatusCode}
			if jsonErr := json.Unmarshal(respBody, apiErr); jsonErr != nil {
				apiErr.Message = string(respBody)
			}
			return apiErr
		}

		// Parse successful response.
		if result != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("unmarshal response: %w", err)
			}
		}

		return nil
	}

	return lastErr
}

// parseRetryAfter parses the Retry-After header value.
// Returns a default of 1 second if the header is missing or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return time.Second
	}
	if seconds <= 0 {
		return time.Second
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}
