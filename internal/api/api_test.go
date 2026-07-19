package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex/easy-chess-results-api/internal/config"
	"github.com/alex/easy-chess-results-api/internal/service"
	"github.com/alex/easy-chess-results-api/internal/store"
	"github.com/alex/easy-chess-results-api/internal/upstream"
)

func TestDocumentationEndpoints(t *testing.T) {
	cfg := config.Config{APIKeys: map[string]struct{}{"secret": {}}}
	handler := New(cfg, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/openapi.yaml", contentType: "application/yaml", contains: "openapi: 3.1.0"},
		{path: "/docs", contentType: "text/html", contains: "SwaggerUIBundle"},
		{path: "/docs/", contentType: "text/html", contains: "/openapi.yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", got, tc.contentType)
			}
			if !strings.Contains(recorder.Body.String(), tc.contains) {
				t.Fatalf("response does not contain %q", tc.contains)
			}
		})
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/openapi.yaml", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /openapi.yaml status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestCORSAllowsConfiguredFrontendAndPreflight(t *testing.T) {
	cfg := config.Config{CORSAllowedOrigins: map[string]struct{}{"https://alexzavalny.github.io": {}}}
	handler := New(cfg, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set("Origin", "https://alexzavalny.github.io")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://alexzavalny.github.io" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}

	request = httptest.NewRequest(http.MethodOptions, "/api/v1/tournaments/42", nil)
	request.Header.Set("Origin", "https://alexzavalny.github.io")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodGet) {
		t.Fatalf("Access-Control-Allow-Methods = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Ngrok-Skip-Browser-Warning") {
		t.Fatalf("Access-Control-Allow-Headers = %q", got)
	}
}

func TestCORSDoesNotAllowUnconfiguredOrigin(t *testing.T) {
	cfg := config.Config{CORSAllowedOrigins: map[string]struct{}{"https://alexzavalny.github.io": {}}}
	handler := New(cfg, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set("Origin", "https://example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestOpenAPICoversImplementedRoutes(t *testing.T) {
	spec := string(openAPISpec)
	paths := []string{
		"/openapi.yaml",
		"/docs",
		"/health/live",
		"/health/ready",
		"/api/v1/tournaments",
		"/api/v1/tournaments/{tournament_id}",
		"/api/v1/tournaments/{tournament_id}/standings",
		"/api/v1/tournaments/{tournament_id}/players/{start_number}",
		"/api/v1/tournaments/{tournament_id}/players/{start_number}/results",
		"/api/v1/players",
		"/api/v1/players/{player_key}",
	}
	for _, path := range paths {
		if !strings.Contains(spec, "  "+path+":\n") {
			t.Errorf("OpenAPI document does not define %s", path)
		}
	}
	if strings.Contains(spec, "/pairings:") {
		t.Error("OpenAPI document advertises the unimplemented pairings endpoint")
	}
}

func TestTournamentCoalescingCacheAndETag(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "event.html"))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(fixture)
	}))
	defer upstreamServer.Close()
	cfg := config.Config{UpstreamBaseURL: upstreamServer.URL, UpstreamLanguage: "1", UpstreamTimeout: 2 * time.Second, UpstreamMaxConcurrency: 2, UpstreamMinInterval: time.Millisecond, UpstreamMaxBodyBytes: 1024 * 1024, ActiveStandingsTTL: time.Hour, CompletedTTL: time.Hour, MaxStaleActive: time.Hour, MaxStaleCompleted: time.Hour, CacheControlMaxAge: 30, DefaultFederation: "LAT", MaxSearchResults: 100, SearchTTL: time.Hour}
	st, err := store.Open(filepath.Join(t.TempDir(), "api.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	client, err := upstream.New(cfg.UpstreamBaseURL, cfg.UpstreamLanguage, cfg.UpstreamTimeout, cfg.UpstreamMaxConcurrency, cfg.UpstreamMinInterval, cfg.UpstreamMaxBodyBytes)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(cfg, service.New(cfg, st, client), st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(handler)
	defer server.Close()

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, e := http.Get(server.URL + "/api/v1/tournaments/1359649")
			if e == nil && resp.StatusCode != http.StatusOK {
				e = &statusError{status: resp.StatusCode}
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("identical refreshes made %d upstream requests", calls.Load())
	}
	resp, err := http.Get(server.URL + "/api/v1/tournaments/1359649")
	if err != nil {
		t.Fatal(err)
	}
	etag := resp.Header.Get("ETag")
	_ = resp.Body.Close()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/tournaments/1359649", nil)
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified || etag == "" || calls.Load() != 1 {
		t.Fatalf("status=%d etag=%q calls=%d", resp.StatusCode, etag, calls.Load())
	}
}

type statusError struct{ status int }

func (e *statusError) Error() string { return http.StatusText(e.status) }

func TestCountryFilter(t *testing.T) {
	tests := []struct {
		country, federation, fallback, want string
		wantError                           bool
	}{
		{country: "est", fallback: "LAT", want: "EST"},
		{federation: "LTU", fallback: "LAT", want: "LTU"},
		{fallback: "LAT", want: "LAT"},
		{country: "LAT", federation: "EST", fallback: "LTU", wantError: true},
	}
	for _, tc := range tests {
		got, err := countryFilter(tc.country, tc.federation, tc.fallback)
		if (err != nil) != tc.wantError || got != tc.want {
			t.Fatalf("countryFilter(%q,%q,%q) = %q,%v", tc.country, tc.federation, tc.fallback, got, err)
		}
	}
}
