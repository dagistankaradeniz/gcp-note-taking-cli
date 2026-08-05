// Package client is the HTTP client for the Quillink /v1 API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultAPIBase = "https://note-taking-app-prod.web.app"

// ProblemDetail mirrors the backend's RFC 7807 error shape, scoped to
// every /v1 response (see gcp-note-taking-backend app/models/api_access.py).
type ProblemDetail struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// APIError wraps a non-2xx /v1 response so callers can branch on Status
// for exit codes (see internal/output/exitcodes.go).
type APIError struct {
	Status  int
	Title   string
	Detail  string
	RawBody string
}

func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Title, e.Detail)
	}
	return e.Title
}

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			// A low-frequency CLI (single request, or a device-grant poll
			// every few seconds) gains nothing from keep-alive connection
			// reuse but is exposed to the classic Go http.Client race: the
			// server closes an idle pooled connection right as a new
			// request tries to reuse it, surfacing as a bare EOF instead
			// of a clean error. One connection per request avoids it.
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}
}

// Do issues a /v1 request. path must start with "/v1/". body, if non-nil,
// is marshaled as the JSON request body. out, if non-nil, receives the
// unmarshaled JSON response body.
func (c *Client) Do(method, path string, query url.Values, body, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, u, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.Token == "" {
		return fmt.Errorf("not logged in -- run `quillink login` or set QUILLINK_TOKEN")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "quillink-cli/"+Version)
	// The usage-tracking middleware reads this specific header, not
	// User-Agent (see gcp-note-taking-backend app/middleware/auth.py) --
	// this is what lets a CLI version be deprecated deliberately (see CLI
	// Access Confluence page, "Distribution").
	req.Header.Set("X-Client-Version", "quillink-cli/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var problem ProblemDetail
		apiErr := &APIError{Status: resp.StatusCode, RawBody: string(respBody)}
		if json.Unmarshal(respBody, &problem) == nil && problem.Title != "" {
			apiErr.Title = problem.Title
			apiErr.Detail = problem.Detail
		} else {
			apiErr.Title = http.StatusText(resp.StatusCode)
			apiErr.Detail = string(respBody)
		}
		return apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// DeviceCodeRequest/Response and OAuthTokenResponse are unauthenticated
// endpoints (RFC 8628) called without a bearer token, so they bypass Do.
func (c *Client) PostPublic(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "quillink-cli/"+Version)
	req.Header.Set("X-Client-Version", "quillink-cli/"+Version)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var problem struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(respBody, &problem)
		return &APIError{Status: resp.StatusCode, Title: "request failed", Detail: problem.Detail, RawBody: string(respBody)}
	}

	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}
