package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// seed creates the one demo tenant used throughout the MVP and registers
// its two sandbox API connections:
//   1. mock-api  - parsed live from mock-api/openapi.yaml via py-planner
//   2. stripe    - hand-crafted tool set against Stripe's test-mode API
//
// Usage (from go-engine/):
//   go run ./cmd/seed
//
// Requires go-engine and py-planner already running (docker-compose up),
// and go-engine/.env + py-planner/.env populated (INTERNAL_API_SECRET must
// match in both, and py-planner/.env needs STRIPE_TEST_SECRET_KEY set for
// the Stripe tools to actually work end-to-end).

const (
	goEngineURL    = "http://localhost:8080"
	pyPlannerURL   = "http://localhost:8000"
	demoTenantName = "IntegraDock Demo"
	demoTenantSlug = "integradock-demo"
)

var internalSecret = os.Getenv("INTERNAL_API_SECRET")

func main() {
	if internalSecret == "" {
		log.Fatal("seed: INTERNAL_API_SECRET env var is required (same value as go-engine/.env)")
	}

	tenantID, err := ensureTenant()
	if err != nil {
		log.Fatalf("seed: failed to create demo tenant: %v", err)
	}
	fmt.Printf("seed: demo tenant ready -> id=%s slug=%s\n", tenantID, demoTenantSlug)

	mockToolCount, err := registerMockAPI(tenantID)
	if err != nil {
		log.Fatalf("seed: failed to register mock-api: %v", err)
	}
	fmt.Printf("seed: mock-api connected -> %d tool(s) registered\n", mockToolCount)

	stripeToolCount, err := registerStripe(tenantID)
	if err != nil {
		log.Fatalf("seed: failed to register stripe: %v", err)
	}
	fmt.Printf("seed: stripe (test mode) connected -> %d tool(s) registered\n", stripeToolCount)

	fmt.Println("\nseed: done. Use this tenant_id in the frontend's \"Set tenant\" field:")
	fmt.Println(tenantID)
}

// ---- Tenant ----

func ensureTenant() (string, error) {
	// Try fetching by slug first so re-running seed is idempotent.
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/tenants/%s", goEngineURL, demoTenantSlug), nil)
	req.Header.Set("X-Internal-Secret", internalSecret)
	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var t struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&t); err == nil && t.ID != "" {
			return t.ID, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	body, _ := json.Marshal(map[string]string{"name": demoTenantName, "slug": demoTenantSlug})
	createReq, _ := http.NewRequest(http.MethodPost, goEngineURL+"/api/tenants/", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Internal-Secret", internalSecret)

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(createResp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", createResp.StatusCode, string(b))
	}

	var t struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&t); err != nil {
		return "", fmt.Errorf("failed to decode created tenant: %w", err)
	}
	return t.ID, nil
}

// ---- Mock API (parsed via py-planner) ----

func registerMockAPI(tenantID string) (int, error) {
	specPath := filepath.Join("..", "mock-api", "openapi.yaml")
	f, err := os.Open(specPath)
	if err != nil {
		return 0, fmt.Errorf("could not open %s (run seed from go-engine/): %w", specPath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "openapi.yaml")
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return 0, err
	}
	_ = writer.WriteField("connection_name", "Mock Inventory & Orders API")
	_ = writer.WriteField("base_url", "http://localhost:9090")
	_ = writer.WriteField("auth_type", "none")
	_ = writer.WriteField("auth_config", "{}")
	writer.Close()

	req, err := http.NewRequest(http.MethodPost, pyPlannerURL+"/parse/upload", &buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("py-planner /parse/upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("py-planner returned %d: %s", resp.StatusCode, string(b))
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("failed to decode parsed connection: %w", err)
	}

	return forwardConnectionToGoEngine(tenantID, parsed)
}

// ---- Stripe (hand-crafted tool set, test mode) ----

func registerStripe(tenantID string) (int, error) {
	stripeKey := os.Getenv("STRIPE_TEST_SECRET_KEY")
	if stripeKey == "" {
		fmt.Println("seed: WARNING - STRIPE_TEST_SECRET_KEY not set in this shell; registering tools anyway, but calls will fail auth until py-planner's .env has it and go-engine's auth_config below is updated with a real key")
		stripeKey = "sk_test_REPLACE_ME"
	}

	connection := map[string]any{
		"tenant_id": tenantID,
		"name":      "Stripe (Test Mode)",
		"base_url":  "https://api.stripe.com/v1",
		"auth_type": "bearer",
		"auth_config": map[string]string{
			"token": stripeKey,
		},
		"spec_raw": map[string]any{"note": "hand-curated subset of Stripe's Customers API for demo purposes"},
		"tools": []map[string]any{
			{
				"tool_name":     "list_customers",
				"description":   "List existing Stripe customers (read-only)",
				"http_method":   "GET",
				"path_template": "/customers",
				"parameters_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"limit": map[string]any{"type": "integer", "description": "max number of customers to return"},
						"email": map[string]any{"type": "string", "description": "filter by exact email match"},
					},
					"required": []string{},
				},
				"is_destructive": false,
			},
			{
				"tool_name":     "create_customer",
				"description":   "Create a new Stripe customer (destructive - creates real billing record in test mode)",
				"http_method":   "POST",
				"path_template": "/customers",
				"parameters_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"body": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"email": map[string]any{"type": "string"},
								"name":  map[string]any{"type": "string"},
							},
						},
					},
					"required": []string{"body"},
				},
				"is_destructive": true,
			},
		},
	}

	return forwardConnectionToGoEngine(tenantID, connection)
}

func forwardConnectionToGoEngine(tenantID string, parsedOrManual map[string]any) (int, error) {
	payload := map[string]any{
		"tenant_id":   tenantID,
		"name":        parsedOrManual["name"],
		"base_url":    parsedOrManual["base_url"],
		"auth_type":   valueOr(parsedOrManual["auth_type"], "none"),
		"auth_config": valueOr(parsedOrManual["auth_config"], map[string]any{}),
		"spec_raw":    valueOr(parsedOrManual["spec_raw"], map[string]any{}),
		"tools":       parsedOrManual["tools"],
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequest(http.MethodPost, goEngineURL+"/api/tools/connections", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", internalSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("go-engine /api/tools/connections failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("go-engine returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode registration result: %w", err)
	}

	return len(result.Tools), nil
}

func valueOr(v any, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}
