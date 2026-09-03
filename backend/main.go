package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type config struct {
	Port       string
	Message    string
	AppEnv     string
	Version    string
	APIKeyFile string
}

type metrics struct {
	mu       sync.Mutex
	requests map[string]uint64
	started  time.Time
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func loadConfig() config {
	return config{
		Port:       envOrDefault("PORT", "8080"),
		Message:    envOrDefault("APP_MESSAGE", "Hello from Kubernetes"),
		AppEnv:     envOrDefault("APP_ENV", "development"),
		Version:    envOrDefault("APP_VERSION", "dev"),
		APIKeyFile: envOrDefault("API_KEY_FILE", "/var/run/secrets/webapp/api-key"),
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func newMetrics() *metrics {
	return &metrics{requests: make(map[string]uint64), started: time.Now()}
}

func (m *metrics) record(method, path string, code int) {
	key := fmt.Sprintf("%s|%s|%d", method, path, code)
	m.mu.Lock()
	m.requests[key]++
	m.mu.Unlock()
}

func (m *metrics) handler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintln(w, "# HELP webapp_http_requests_total Total HTTP requests handled by the backend.")
		fmt.Fprintln(w, "# TYPE webapp_http_requests_total counter")
		for key, count := range m.requests {
			parts := strings.Split(key, "|")
			fmt.Fprintf(w, "webapp_http_requests_total{method=%q,path=%q,code=%q} %d\n", parts[0], parts[1], parts[2], count)
		}
		fmt.Fprintln(w, "# HELP webapp_process_uptime_seconds Backend process uptime in seconds.")
		fmt.Fprintln(w, "# TYPE webapp_process_uptime_seconds gauge")
		fmt.Fprintf(w, "webapp_process_uptime_seconds %.0f\n", time.Since(m.started).Seconds())
		fmt.Fprintln(w, "# HELP webapp_build_info Build information for the backend.")
		fmt.Fprintln(w, "# TYPE webapp_build_info gauge")
		fmt.Fprintf(w, "webapp_build_info{version=%q} 1\n", version)
	}
}

func instrument(next http.Handler, m *metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		m.record(metricMethod(r.Method), metricPath(r.URL.Path), sw.status)
	})
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func metricPath(path string) string {
	switch path {
	case "/healthz", "/readyz", "/api/message", "/api/private":
		return path
	default:
		return "unmatched"
	}
}

func readAPIKey(path string) (string, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(value))
	if key == "" {
		return "", errors.New("API key is empty")
	}
	return key, nil
}

func authorized(got, expected string) bool {
	if len(got) != len(expected) || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func newHandler(cfg config, m *metrics) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := readAPIKey(cfg.APIKeyFile); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/message", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"message":     cfg.Message,
			"environment": cfg.AppEnv,
			"version":     cfg.Version,
		})
	})
	mux.HandleFunc("GET /api/private", func(w http.ResponseWriter, r *http.Request) {
		expected, err := readAPIKey(cfg.APIKeyFile)
		if err != nil {
			slog.Error("read API key", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "secret unavailable"})
			return
		}
		if !authorized(r.Header.Get("X-API-Key"), expected) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "authenticated response"})
	})
	mux.Handle("GET /metrics", m.handler(cfg.Version))
	return instrument(mux, m)
}

func main() {
	cfg := loadConfig()
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           newHandler(cfg, newMetrics()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("backend listening", "port", cfg.Port, "environment", cfg.AppEnv, "version", cfg.Version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	deadline, deadlineCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer deadlineCancel()
	if err := server.Shutdown(deadline); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
