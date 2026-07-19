package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alex/easy-chess-results-api/internal/config"
	"github.com/alex/easy-chess-results-api/internal/domain"
	"github.com/alex/easy-chess-results-api/internal/service"
	"github.com/alex/easy-chess-results-api/internal/store"
)

type API struct {
	cfg     config.Config
	service *service.Service
	store   *store.Store
	log     *slog.Logger
	keys    [][32]byte
}
type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}
type contextKey string

const requestIDKey contextKey = "request_id"

var digits = regexp.MustCompile(`^[0-9]+$`)

//go:embed openapi.yaml
var openAPISpec []byte

const docsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Easy Chess Results API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      displayRequestDuration: true,
      persistAuthorization: true
    });
  </script>
</body>
</html>
`

func New(cfg config.Config, svc *service.Service, st *store.Store, log *slog.Logger) http.Handler {
	a := &API{cfg: cfg, service: svc, store: st, log: log}
	for k := range cfg.APIKeys {
		a.keys = append(a.keys, sha256.Sum256([]byte(k)))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.yaml", a.openAPI)
	mux.HandleFunc("/openapi.yaml", a.methodNotAllowed)
	mux.HandleFunc("GET /docs", a.docs)
	mux.HandleFunc("/docs", a.methodNotAllowed)
	mux.HandleFunc("GET /docs/", a.docs)
	mux.HandleFunc("/docs/", a.methodNotAllowed)
	mux.HandleFunc("GET /health/live", a.live)
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.HandleFunc("/health/live", a.methodNotAllowed)
	mux.HandleFunc("/health/ready", a.methodNotAllowed)
	mux.Handle("GET /api/v1/tournaments", a.auth(http.HandlerFunc(a.searchTournaments)))
	mux.Handle("/api/v1/tournaments", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/tournaments/{id}", a.auth(http.HandlerFunc(a.tournament)))
	mux.Handle("/api/v1/tournaments/{id}", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/tournaments/{id}/standings", a.auth(http.HandlerFunc(a.standings)))
	mux.Handle("/api/v1/tournaments/{id}/standings", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/tournaments/{id}/players/{start}", a.auth(http.HandlerFunc(a.participant)))
	mux.Handle("/api/v1/tournaments/{id}/players/{start}", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/tournaments/{id}/players/{start}/results", a.auth(http.HandlerFunc(a.results)))
	mux.Handle("/api/v1/tournaments/{id}/players/{start}/results", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/players", a.auth(http.HandlerFunc(a.searchPlayers)))
	mux.Handle("/api/v1/players", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.Handle("GET /api/v1/players/{key}", a.auth(http.HandlerFunc(a.player)))
	mux.Handle("/api/v1/players/{key}", a.auth(http.HandlerFunc(a.methodNotAllowed)))
	mux.HandleFunc("/", a.notFound)
	return a.middleware(mux)
}

func (a *API) openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func (a *API) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(docsHTML))
}

func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := requestID()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		rw := &statusWriter{ResponseWriter: w, status: 200}
		if len(r.RequestURI) > 8192 {
			a.writeError(rw, r.WithContext(ctx), http.StatusBadRequest, "invalid_request", "Request URL is too long.", nil)
			a.log.Info("http request", "request_id", id, "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
			return
		}
		next.ServeHTTP(rw, r.WithContext(ctx))
		a.log.Info("http request", "request_id", id, "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }
func (a *API) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	a.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "The HTTP method is not allowed for this resource.", nil)
}
func (a *API) notFound(w http.ResponseWriter, r *http.Request) {
	a.writeError(w, r, http.StatusNotFound, "not_found", "The requested resource was not found.", nil)
}
func (a *API) auth(next http.Handler) http.Handler {
	if len(a.keys) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		sum := sha256.Sum256([]byte(token))
		ok := 0
		for _, k := range a.keys {
			ok |= subtle.ConstantTimeCompare(sum[:], k[:])
		}
		if ok != 1 {
			a.writeError(w, r, 401, "invalid_request", "A valid API key is required.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *API) live(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, 200, map[string]string{"status": "ok"}, false)
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.Ready(ctx); err != nil {
		a.writeError(w, r, 503, "internal_error", "Service is not ready.", nil)
		return
	}
	a.writeJSON(w, r, 200, map[string]string{"status": "ok"}, false)
}

func (a *API) searchTournaments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fed, err := countryFilter(q.Get("country"), q.Get("federation"), a.cfg.DefaultFederation)
	if err != nil {
		a.invalid(w, r, err.Error())
		return
	}
	if fed != "-" && (len(fed) != 3 || !letters(fed)) {
		a.invalid(w, r, "country must be a three-letter code or -")
		return
	}
	from, err := date(q.Get("end_from"))
	if err != nil {
		a.invalid(w, r, "end_from must be YYYY-MM-DD")
		return
	}
	to, err := date(q.Get("end_to"))
	if err != nil {
		a.invalid(w, r, "end_to must be YYYY-MM-DD")
		return
	}
	if from != nil && to != nil {
		if from.After(*to) {
			a.invalid(w, r, "end_from must not be after end_to")
			return
		}
		if to.Sub(*from) > time.Duration(a.cfg.MaxSearchIntervalDays)*24*time.Hour {
			a.invalid(w, r, "search interval exceeds configured maximum")
			return
		}
	}
	refresh, ok := boolean(q.Get("refresh"))
	if !ok {
		a.invalid(w, r, "refresh must be boolean")
		return
	}
	items, meta, err := a.service.SearchTournaments(r.Context(), fed, formatDate(from), formatDate(to), q.Get("q"), q.Get("time_control"), refresh)
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "tournament_search"})
		return
	}
	a.writeJSON(w, r, 200, domain.SearchResponse{Data: items, Count: len(items), Meta: meta}, meta.Stale)
}

func countryFilter(country, federation, fallback string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	federation = strings.ToUpper(strings.TrimSpace(federation))
	if country != "" && federation != "" && country != federation {
		return "", errors.New("country and federation must match when both are provided")
	}
	return strings.ToUpper(value(country, value(federation, fallback))), nil
}
func (a *API) tournament(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		a.invalid(w, r, "invalid tournament_id")
		return
	}
	refresh, ok := boolean(r.URL.Query().Get("refresh"))
	if !ok {
		a.invalid(w, r, "refresh must be boolean")
		return
	}
	data, meta, err := a.service.Tournament(r.Context(), id, refresh)
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "tournament", "tournament_id": id})
		return
	}
	a.writeJSON(w, r, 200, domain.Envelope[domain.Tournament]{Data: data, Meta: meta}, meta.Stale)
}
func (a *API) standings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		a.invalid(w, r, "invalid tournament_id")
		return
	}
	var round *int
	if s := r.URL.Query().Get("round"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n <= 0 {
			a.invalid(w, r, "round must be a positive integer")
			return
		}
		round = &n
	}
	limit, offset, ok := pagination(r, 500)
	if !ok {
		a.invalid(w, r, "invalid limit or offset")
		return
	}
	refresh, bok := boolean(r.URL.Query().Get("refresh"))
	if !bok {
		a.invalid(w, r, "refresh must be boolean")
		return
	}
	data, meta, err := a.service.Standings(r.Context(), id, round, refresh)
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "tournament_standings", "tournament_id": id})
		return
	}
	group := r.URL.Query().Get("group")
	filtered := data.Standings[:0]
	for _, s := range data.Standings {
		if group != "" && (s.Group == nil || *s.Group != group) {
			continue
		}
		filtered = append(filtered, s)
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := len(filtered)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	data.Standings = filtered[offset:end]
	a.writeJSON(w, r, 200, domain.Envelope[domain.Standings]{Data: data, Meta: meta}, meta.Stale)
}
func (a *API) participant(w http.ResponseWriter, r *http.Request) {
	id, start, ok := a.ids(w, r)
	if !ok {
		return
	}
	refresh, b := boolean(r.URL.Query().Get("refresh"))
	if !b {
		a.invalid(w, r, "refresh must be boolean")
		return
	}
	data, meta, err := a.service.Participant(r.Context(), id, start, refresh)
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "tournament_player", "tournament_id": id, "start_number": start})
		return
	}
	a.writeJSON(w, r, 200, domain.Envelope[domain.Participant]{Data: data, Meta: meta}, meta.Stale)
}
func (a *API) results(w http.ResponseWriter, r *http.Request) {
	id, start, ok := a.ids(w, r)
	if !ok {
		return
	}
	refresh, b := boolean(r.URL.Query().Get("refresh"))
	if !b {
		a.invalid(w, r, "refresh must be boolean")
		return
	}
	data, meta, err := a.service.PlayerResults(r.Context(), id, start, refresh)
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "player_results", "tournament_id": id, "start_number": start})
		return
	}
	a.writeJSON(w, r, 200, domain.Envelope[domain.PlayerResults]{Data: data, Meta: meta}, meta.Stale)
}
func (a *API) ids(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	id, start := r.PathValue("id"), r.PathValue("start")
	if !validID(id) || !validID(start) {
		a.invalid(w, r, "invalid tournament_id or start_number")
		return "", "", false
	}
	return id, start, true
}
func (a *API) searchPlayers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fide := q.Get("fide_id")
	if fide != "" && !digits.MatchString(fide) {
		a.invalid(w, r, "fide_id must contain digits only")
		return
	}
	fed := strings.ToUpper(q.Get("federation"))
	if fed != "" && (len(fed) != 3 || !letters(fed)) {
		a.invalid(w, r, "federation must be a three-letter code")
		return
	}
	limit, offset, ok := pagination(r, 100)
	if !ok {
		a.invalid(w, r, "invalid limit or offset")
		return
	}
	if limit == 0 {
		limit = 20
	}
	items, err := a.service.SearchPlayers(r.Context(), q.Get("q"), fide, fed, limit, offset)
	if err != nil {
		a.internal(w, r)
		return
	}
	a.writeJSON(w, r, 200, map[string]any{"data": items, "count": len(items), "scope": "locally_indexed_tournaments"}, false)
}
func (a *API) player(w http.ResponseWriter, r *http.Request) {
	data, err := a.service.Player(r.Context(), r.PathValue("key"))
	if err != nil {
		a.serviceError(w, r, err, map[string]any{"resource": "player"})
		return
	}
	a.writeJSON(w, r, 200, map[string]any{"data": data, "scope": "locally_indexed_tournaments"}, false)
}

func (a *API) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any, stale bool) {
	b, err := json.Marshal(v)
	if err != nil {
		a.internal(w, r)
		return
	}
	sum := sha256.Sum256(b)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", a.cfg.CacheControlMaxAge))
	w.Header().Set("ETag", etag)
	if stale {
		w.Header().Set("Warning", `110 - "Response is stale"`)
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}
func (a *API) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Code: code, Message: message, RequestID: idFrom(r), Details: details}})
}
func (a *API) serviceError(w http.ResponseWriter, r *http.Request, err error, details map[string]any) {
	var e *service.Error
	if errors.As(err, &e) {
		switch e.Kind {
		case service.ParseError:
			a.writeError(w, r, 502, "upstream_parse_error", "Chess-Results returned a page layout this parser does not recognize.", details)
		case service.NotFound:
			a.writeError(w, r, 404, "upstream_not_found", "The requested Chess-Results resource was not found.", details)
		default:
			a.writeError(w, r, 503, "upstream_unavailable", "Chess-Results is temporarily unavailable and no usable cached data exists.", details)
		}
		a.log.Warn("request failed", "request_id", idFrom(r), "resource", e.Resource, "error", e.Err)
		return
	}
	a.internal(w, r)
}
func (a *API) invalid(w http.ResponseWriter, r *http.Request, m string) {
	a.writeError(w, r, 400, "invalid_request", m, nil)
}
func (a *API) internal(w http.ResponseWriter, r *http.Request) {
	a.writeError(w, r, 500, "internal_error", "An internal error occurred.", nil)
}
func validID(s string) bool { return s != "" && len(s) <= 20 && digits.MatchString(s) }
func letters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
func value(v, d string) string {
	if v != "" {
		return v
	}
	return d
}
func date(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, e := time.Parse("2006-01-02", s)
	return &t, e
}
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
func boolean(s string) (bool, bool) {
	if s == "" {
		return false, true
	}
	v, e := strconv.ParseBool(s)
	return v, e == nil
}
func pagination(r *http.Request, max int) (int, int, bool) {
	limit, offset := 0, 0
	var e error
	if s := r.URL.Query().Get("limit"); s != "" {
		limit, e = strconv.Atoi(s)
		if e != nil || limit < 0 || limit > max {
			return 0, 0, false
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		offset, e = strconv.Atoi(s)
		if e != nil || offset < 0 {
			return 0, 0, false
		}
	}
	return limit, offset, true
}
func requestID() string             { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func idFrom(r *http.Request) string { v, _ := r.Context().Value(requestIDKey).(string); return v }
