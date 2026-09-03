package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testConfig(t *testing.T) config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(path, []byte("test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config{Port: "8080", Message: "test message", AppEnv: "test", Version: "v-test", APIKeyFile: path}
}

func TestMessage(t *testing.T) {
	h := newHandler(testConfig(t), newMetrics())
	req := httptest.NewRequest(http.MethodGet, "/api/message", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "test message" || body["environment"] != "test" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestPrivateEndpoint(t *testing.T) {
	h := newHandler(testConfig(t), newMetrics())

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	unauthorized.Header.Set("X-API-Key", "wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", rr.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	authorizedRequest.Header.Set("X-API-Key", "test-secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, authorizedRequest)
	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rr.Code)
	}
}

func TestMetrics(t *testing.T) {
	h := newHandler(testConfig(t), newMetrics())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/message", nil))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `webapp_http_requests_total{method="GET",path="/api/message",code="200"} 1`) {
		t.Fatalf("metric missing: %s", rr.Body.String())
	}
}

func TestReadinessFailsWithoutSecret(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKeyFile = "/does/not/exist"
	rr := httptest.NewRecorder()
	newHandler(cfg, newMetrics()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want 503", rr.Code)
	}
}

func TestMetricPathHasBoundedCardinality(t *testing.T) {
	if got := metricPath("/api/users/12345"); got != "unmatched" {
		t.Fatalf("got %q, want unmatched", got)
	}
	if got := metricPath("/api/message"); got != "/api/message" {
		t.Fatalf("got %q, want /api/message", got)
	}
	if got := metricMethod("CUSTOM-12345"); got != "OTHER" {
		t.Fatalf("got %q, want OTHER", got)
	}
}
