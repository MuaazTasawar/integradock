package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AuthConfig mirrors api_connections.auth_config for building outbound requests.
// Supported shapes:
//
//	bearer:  {"token": "sk_test_..."}                     -> Authorization: Bearer <token>
//	api_key: {"header": "X-API-Key", "value": "..."}      -> custom header
//	basic:   {"username": "...", "password": "..."}       -> Authorization: Basic <b64>
type AuthConfig struct {
	Token    string `json:"token,omitempty"`
	Header   string `json:"header,omitempty"`
	Value    string `json:"value,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// RequestSpec describes exactly what real HTTP call to make.
type RequestSpec struct {
	BaseURL      string
	HTTPMethod   string
	PathTemplate string // e.g. /v1/customers/{id}
	AuthType     string // none | bearer | api_key | basic
	AuthConfig   AuthConfig
	Arguments    map[string]any // path params, query params, and "body" key for request body
}

// HTTPResult is the normalized outcome of a tool's real API call.
type HTTPResult struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
	DurationMs int64           `json:"duration_ms"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Execute performs the real HTTP request described by spec and returns a normalized result.
func Execute(ctx context.Context, spec RequestSpec) (*HTTPResult, error) {
	url, remainingArgs := resolvePathParams(spec.BaseURL, spec.PathTemplate, spec.Arguments)

	var bodyReader io.Reader
	var bodyBytes []byte

	if bodyVal, ok := remainingArgs["body"]; ok {
		delete(remainingArgs, "body")
		b, err := json.Marshal(bodyVal)
		if err != nil {
			return nil, fmt.Errorf("executor: failed to marshal request body: %w", err)
		}
		bodyBytes = b
		bodyReader = bytes.NewReader(b)
	}

	// Remaining args become query params for GET/DELETE-style calls.
	if len(remainingArgs) > 0 && (spec.HTTPMethod == "GET" || spec.HTTPMethod == "DELETE") {
		url = appendQueryParams(url, remainingArgs)
	}

	req, err := http.NewRequestWithContext(ctx, spec.HTTPMethod, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("executor: failed to build request: %w", err)
	}
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	applyAuth(req, spec.AuthType, spec.AuthConfig)

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executor: request failed: %w", err)
	}
	defer resp.Body.Close()
	duration := time.Since(start).Milliseconds()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("executor: failed to read response body: %w", err)
	}

	// Normalize non-JSON responses into a JSON string so callers can always json.Unmarshal.
	var normalized json.RawMessage
	if json.Valid(respBody) {
		normalized = respBody
	} else {
		wrapped, _ := json.Marshal(map[string]string{"raw": string(respBody)})
		normalized = wrapped
	}

	return &HTTPResult{
		StatusCode: resp.StatusCode,
		Body:       normalized,
		DurationMs: duration,
	}, nil
}

func resolvePathParams(baseURL, pathTemplate string, args map[string]any) (string, map[string]any) {
	remaining := make(map[string]any, len(args))
	for k, v := range args {
		remaining[k] = v
	}

	resolvedPath := pathTemplate
	for k, v := range args {
		placeholder := "{" + k + "}"
		if strings.Contains(resolvedPath, placeholder) {
			resolvedPath = strings.ReplaceAll(resolvedPath, placeholder, fmt.Sprintf("%v", v))
			delete(remaining, k)
		}
	}

	full := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resolvedPath, "/")
	return full, remaining
}

func appendQueryParams(url string, params map[string]any) string {
	if len(params) == 0 {
		return url
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	parts := make([]string, 0, len(params))
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return url + sep + strings.Join(parts, "&")
}

func applyAuth(req *http.Request, authType string, cfg AuthConfig) {
	switch authType {
	case "bearer":
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
	case "api_key":
		if cfg.Header != "" && cfg.Value != "" {
			req.Header.Set(cfg.Header, cfg.Value)
		}
	case "basic":
		if cfg.Username != "" {
			req.SetBasicAuth(cfg.Username, cfg.Password)
		}
	case "none", "":
		// no-op
	}
}
