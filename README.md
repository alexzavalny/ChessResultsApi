# Easy Chess Results API

A small read-only Go API that turns Chess-Results pages into stable JSON resources. It is designed for an always-on Raspberry Pi and includes bounded upstream access, SQLite caching, request coalescing, explicit stale fallback, ETags, and a locally populated player index.

## Implemented endpoints

```text
GET /health/live
GET /health/ready
GET /api/v1/tournaments
GET /api/v1/tournaments/{tournament_id}
GET /api/v1/tournaments/{tournament_id}/standings
GET /api/v1/tournaments/{tournament_id}/players/{start_number}
GET /api/v1/tournaments/{tournament_id}/players/{start_number}/results
GET /api/v1/players
GET /api/v1/players/{player_key}
```

## API documentation

The server publishes an OpenAPI 3.1 contract alongside the API:

```text
GET /openapi.yaml
GET /docs
```

`/openapi.yaml` is the machine-readable API description for client generation,
testing tools, and API clients such as Postman. `/docs` renders the same contract
as interactive Swagger UI documentation. Both routes are unauthenticated; API
operations shown in the documentation still require a Bearer key when `API_KEYS`
is configured.

The source contract is kept in `internal/api/openapi.yaml` and embedded in the Go
binary, so deployment still requires only the executable.

Tournament search supports `country`, `end_from`, `end_to`, `q`, `time_control`, and `refresh`. `country` accepts a three-letter federation code such as `LAT`, while `-` searches all countries. The older `federation` name remains available as a compatibility alias. Standings support `round`, `group`, `limit`, `offset`, and `refresh`. Player search is explicitly limited to players observed while parsing tournaments.

Round pairings are not advertised yet. The technical specification requires representative live fixtures from multiple tournament layouts before that parser can be declared stable.

## Run locally

Requires Go 1.24 or newer.

```sh
cp .env.example .env
set -a; source .env; set +a
go run ./cmd/api
```

The default address is `127.0.0.1:8080` and the default database is `./data/api.sqlite`. Configuration is validated before startup. A non-private bind is rejected unless API keys are configured or `ALLOW_INSECURE_PUBLIC_BIND=true` is explicitly set.

When `API_KEYS` is a comma-separated list, API calls require:

```text
Authorization: Bearer <key>
```

Health routes remain unauthenticated. Keys are only retained as SHA-256 digests by the HTTP layer after configuration is loaded.

Browser clients are allowed only from the comma-separated origins in `CORS_ALLOWED_ORIGINS`. The default covers this project's GitHub Pages site and local development on port 8000; replace it when the frontend moves to another origin.

## Examples

```sh
curl 'http://127.0.0.1:8080/api/v1/tournaments?country=LAT&end_from=2026-01-01&end_to=2026-12-31'
curl 'http://127.0.0.1:8080/api/v1/tournaments/1359649/standings?round=5&limit=20'
curl 'http://127.0.0.1:8080/api/v1/players?q=Zavalnijs'
```

Every successful upstream-derived response includes source, fetch time, age, cache disposition, and stale status under `meta`. A stale fallback also sends an HTTP `Warning` header.

## Quality gates

```sh
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o easy-chess-results-api ./cmd/api
```

No routine test contacts Chess-Results. Parser behavior is exercised with sanitized fixtures in `testdata/`.

## Raspberry Pi deployment

Cross-compile as shown above, install the binary at `/usr/local/bin/easy-chess-results-api`, copy the unit from `deploy/systemd`, and create `/etc/easy-chess-results-api.env`. The service binds to loopback by default; use Caddy, another maintained reverse proxy, or Tailscale for remote access.

After the initial installation, update the Pi from the repository root with:

```sh
./deploy/update-raspberry-pi.sh
```

The script fast-forwards the current branch from GitHub, runs the tests, cross-compiles the ARM64 binary, uploads it over SSH, restarts the service, and checks readiness. It defaults to `alex@192.168.68.62`; pass another SSH destination as its first argument when needed. SSH credentials are never stored by the script.

The database is a disposable cache/index but may contain player names and birth years. Do not copy a live WAL database directly; use SQLite's backup mechanism or `VACUUM INTO` when backups are required.
