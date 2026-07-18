# Easy Chess Results API — Technical Specification

Status: proposed implementation specification  
Target: Raspberry Pi, always-on personal/small-public service  
Recommended implementation: Go 1.24+ (use the current stable Go release when generating the project)  
Primary upstream: `chess-results.com` and its shard hosts (`s1`, `s2`, `s3`, etc.)

## 1. Purpose

Build a standalone HTTP JSON API that hides Chess-Results HTML, query-string conventions, ASP.NET form state, and inconsistent table layouts behind stable resource-oriented endpoints.

The API must support:

- searching tournaments;
- searching players already discovered by the service;
- retrieving tournament details;
- retrieving tournament standings, including standings after a specific round;
- retrieving pairings for a tournament round;
- retrieving one player's results in a specific tournament;
- lightweight deployment and operation on a Raspberry Pi;
- caching and request coalescing so the service behaves responsibly toward Chess-Results;
- parser fixtures and diagnostics so upstream HTML changes are detectable.

This is a read-only aggregation and normalization service. It is not a chess tournament management system and is not the system of record. Every response derived from Chess-Results should expose source and freshness metadata.

## 2. Findings from the existing EasyChessResults project

The current project is a static browser prototype. Its reusable behavior is concentrated in `lib/chess-results.js`; `script.js` fetches pages, maintains browser navigation, and renders the parsed view.

### 2.1 Existing upstream flows

#### Tournament search

1. `GET https://s2.chess-results.com/turniersuche.aspx?lan=1`.
2. Extract every hidden form field, especially ASP.NET state fields such as `__VIEWSTATE` and `__EVENTVALIDATION`.
3. POST the hidden fields plus:
   - `ctl00$P1$combo_art=5`
   - `ctl00$P1$combo_sort=3`
   - `ctl00$P1$combo_land=<federation or ->`
   - `ctl00$P1$combo_bedenkzeit=0`
   - `ctl00$P1$combo_anzahl_zeilen=0`
   - optional `ctl00$P1$txt_von_tag=<YYYY-MM-DD>`
   - optional `ctl00$P1$txt_bis_tag=<YYYY-MM-DD>`
   - `ctl00$P1$cb_suchen=Search`
4. Find the direct result table whose header contains `Tournament` and whose direct rows contain `tnr<digits>.aspx` links.
5. Ignore outer layout tables and form rows even if their text also contains the word “Tournament.”

The prototype currently hardcodes federation `LAT`; the API must make federation optional and configurable.

#### Tournament standings/details

1. Fetch a `tnr<id>.aspx` URL with `art=1`.
2. Treat `art=0` links as overview links and normalize them to `art=1`.
3. Find `.defaultDialog` sections.
4. Identify the ranking dialog by either:
   - a heading containing `ranking`, `rank after round`, or `starting rank`; or
   - a `table.CRs1` whose header contains `Name` and either `Pts.` or `Rtg`.
5. Identify a different headed dialog containing a table as the tournament metadata dialog.
6. Read key/value metadata rows and the `.CRsmall` “Last update” text.
7. Parse standings dynamically from the upstream headers rather than fixed indexes.
8. Resolve player links to absolute URLs.
9. Find “ranking list after round” links elsewhere in the document.

#### Player results in a tournament

1. Fetch `tnr<id>.aspx?art=9&snr=<start-number>`.
2. Locate the `.defaultDialog` whose `h2` is exactly `Player info` (case-insensitive).
3. Parse the first table as key/value player metadata.
4. Parse the second table, when present, as round-by-round opponents/results.
5. Detect color through CSS markers:
   - `.FarbewT` means White;
   - `.FarbesT` means Black.
6. Support both observed opponent layouts:
   - 10+ cells, including `Club/City`;
   - 9 cells, without `Club/City`.
7. Derive the parent tournament URL by changing `art=9` to `art=1` and deleting `snr`.

#### Pairings

The current parser does not parse pairings yet. It does distinguish ranking links from pairing links and its tests contain Chess-Results pairing URLs using `art=2&rd=<round>`. Pairing parsing is therefore new work and must be fixture-driven rather than inferred from the standings table.

### 2.2 Existing normalization worth preserving

- Collapse all whitespace inside a cell and trim it.
- Resolve relative links against the exact fetched source URL.
- Normalize FIDE IDs to digits only.
- Accept decimal commas from Chess-Results, but expose JSON numbers when unambiguous.
- Preserve the original text alongside a nullable parsed number where the source can contain values such as `-`, blank, `3,5`, or nonstandard result markers.
- Support missing player opponent tables (valid player metadata with zero games).
- Support missing club columns.
- Detect tournament columns from headers and tolerate unnamed title/flag columns.
- Ignore known presentation/noise columns (`sex`, `FED`, `TB2`, `TB3`, `K`) only in the UI. The API should retain meaningful upstream fields in `extra`, because clients may need tie-break and federation data.

### 2.3 UI behavior that must not leak into the API

Do not port these presentation concerns into domain data:

- star-highlighting a particular surname;
- bookmarks stored in `localStorage`;
- desktop/mobile row view models;
- HTML chips, alignment, escaping, or CSS classes;
- browser history and internal app links;
- the public CORS proxy;
- cache-busting every request.

The server can fetch Chess-Results directly and should use its own cache/revalidation policy.

## 3. Language and stack decision

### 3.1 Recommendation: Go

Use Go for this service.

Why it fits this project:

- Deploys as one native executable with no application runtime or dependency installation on the Pi.
- Official Go targets include Linux `arm` and `arm64`, and Go documents mature ARM support: [Go on ARM](https://go.dev/wiki/GoArm).
- The standard `net/http` package is sufficient for the server and upstream client: [Go `net/http`](https://pkg.go.dev/net/http).
- Goroutines are a natural fit for bounded background refreshes, while a semaphore and request coalescing can keep upstream concurrency low.
- Static types help stabilize the public JSON contract even when the HTML parser has to be defensive.
- Cross-compilation is straightforward (`GOOS=linux GOARCH=arm64` for a 64-bit Pi OS; use `arm` plus the correct `GOARM` for 32-bit systems). Go's supported targets are documented in [Installing Go from source](https://go.dev/doc/install/source).
- HTML parsing can use `github.com/PuerkitoBio/goquery`, whose selector-oriented API maps closely to the existing DOM parser. It is based on Go's HTML parser rather than regular expressions: [goquery](https://github.com/PuerkitoBio/goquery).

### 3.2 Why not Ruby as the default

Ruby plus Nokogiri and Sinatra/Roda would be concise and pleasant for scraping. It is a reasonable choice if the maintainer is substantially faster in Ruby. For this target, however, it has these disadvantages:

- a Ruby runtime and gems must be installed and kept compatible on the Pi;
- native gem deployment can complicate ARM installation;
- process memory is normally higher than a small Go service;
- using YJIT trades more memory for speed, and Ruby's own documentation explicitly discusses that tradeoff: [Ruby YJIT memory usage](https://docs.ruby-lang.org/en/3.4/yjit/yjit_md.html#label-Reducing+YJIT+Memory+Usage).

This API is network/HTML-latency bound, so maximum CPU speed is not the deciding factor. Operational simplicity and predictable idle footprint are.

### 3.3 Proposed dependencies

Keep the dependency list short:

- router/server: standard `net/http` (Go 1.22+ route patterns), no web framework initially;
- HTML: `github.com/PuerkitoBio/goquery`;
- request coalescing: `golang.org/x/sync/singleflight`;
- persistence: `modernc.org/sqlite` if a CGO-free SQLite driver is desired, or `github.com/mattn/go-sqlite3` if CGO is acceptable;
- migrations: embedded numbered SQL files and a small in-project runner;
- logging: standard `log/slog`;
- metrics: Prometheus client only if metrics scraping will actually be used; otherwise expose compact internal counters at `/health/ready` or omit metrics in v1.

Avoid a full ORM. SQL is small, explicit, and easier to reason about on constrained hardware.

## 4. Architecture

```text
HTTP client
    |
    v
API handlers -> validation -> application services -> cache repository (SQLite)
                                      |                       ^
                                      v                       |
                                refresh coordinator ----------+
                                      |
                                      v
                              Chess-Results client
                                      |
                                      v
                          HTML page-specific parsers
```

Suggested packages:

```text
cmd/api/main.go
internal/api/             HTTP handlers, middleware, DTOs, errors
internal/config/          environment parsing and validation
internal/domain/          canonical domain models
internal/service/         use cases and freshness policy
internal/upstream/        HTTP client, URL builder, throttling
internal/parser/search/   tournament-search parser
internal/parser/event/    metadata and standings parser
internal/parser/player/   player info/results parser
internal/parser/pairing/  round pairings parser
internal/store/           SQLite repositories and migrations
internal/observe/         structured logging, counters
testdata/                 sanitized HTML fixtures
deploy/systemd/           unit file
```

Important boundaries:

- Parsers accept HTML plus the final source URL and return domain-oriented parsed records.
- Parsers do not perform HTTP requests, database writes, or JSON rendering.
- The upstream client does not know HTML layout.
- Services own caching, stale fallback, refresh coalescing, and index updates.
- API DTOs are explicit; never JSON-encode parser or database structs directly.

## 5. Resource identifiers

### 5.1 Tournament ID

Canonical string extracted from `tnr<digits>.aspx`, for example `1359649`. Keep it a string in JSON and Go to avoid needless numeric assumptions.

### 5.2 Tournament player ID

Chess-Results `snr` is a starting-number identifier scoped to one tournament. It is not a global player ID.

Represent it as:

```json
{
  "tournament_id": "1359649",
  "start_number": "61"
}
```

### 5.3 Global-ish player identity

Use FIDE ID when present. Many entries have `0` or no FIDE ID, so it cannot be the sole key. Maintain an internal `player_key`:

- `fide:<digits>` when a nonzero FIDE ID exists;
- otherwise `local:<normalized-name>:<federation>` with a collision-safe hash suffix.

Do not claim that name-based identities are globally unique. Return `identity_confidence: "fide" | "name_federation" | "tournament_only"`.

## 6. Public API conventions

Base path: `/api/v1`  
Media type: `application/json; charset=utf-8`  
Dates: ISO 8601 `YYYY-MM-DD`  
Timestamps: RFC 3339 UTC  
IDs: JSON strings  
Pagination: cursor-based where practical; v1 may use `limit`/`offset` for local player search only  
Unknown fields: clients must ignore them  
Nullability: use `null` when a known field has no source value; omit only fields explicitly documented as optional expansions

Every successful upstream-derived response should contain:

```json
"meta": {
  "source_url": "https://s2.chess-results.com/tnr1359649.aspx?lan=1&art=1",
  "fetched_at": "2026-07-18T12:00:00Z",
  "age_seconds": 24,
  "cache": "hit",
  "stale": false
}
```

`cache` values: `hit`, `miss`, `refreshed`, `stale-fallback`.

Support request header `If-None-Match` and return `ETag` based on the canonical JSON payload. Use `Cache-Control: private, max-age=<short value>` unless the operator explicitly configures public caching.

## 7. Endpoint specification

### 7.1 Liveness

`GET /health/live`

Returns `200` when the process can serve HTTP. Does not contact Chess-Results or SQLite beyond process-local checks.

```json
{"status":"ok"}
```

### 7.2 Readiness

`GET /health/ready`

Checks that configuration is valid and SQLite is readable/writable. Do not make an upstream request on every readiness probe.

Returns `200` when ready, otherwise `503`.

### 7.3 Search tournaments

`GET /api/v1/tournaments`

Query parameters:

- `federation`: optional three-letter code, e.g. `LAT`; default comes from `DEFAULT_FEDERATION`, and `-` means all;
- `end_from`: optional date;
- `end_to`: optional date;
- `q`: optional local substring filter applied after upstream results are parsed;
- `time_control`: optional local case-insensitive filter;
- `refresh`: optional boolean; allowed only for trusted clients or subject to stricter rate limits.

Validation:

- `end_from <= end_to`;
- maximum search interval configurable, recommended 366 days;
- reject malformed federation/date values with `400`;
- cap result count in the API even if the upstream form requests all rows.

Response:

```json
{
  "data": [
    {
      "id": "1416130",
      "name": "Latvijas atrspeles ligas vasaras sezona 2026 | 1.",
      "federation": "LAT",
      "starts_on": "2026-05-20",
      "ends_on": "2026-05-20",
      "time_control": "Rapid",
      "source_url": "https://s2.chess-results.com/tnr1416130.aspx?lan=1"
    }
  ],
  "count": 1,
  "meta": {}
}
```

Implementation note: tournament search requires the two-request GET-hidden-fields then POST flow. Hidden fields belong to that search transaction and must not be reused indefinitely.

### 7.4 Get tournament details

`GET /api/v1/tournaments/{tournament_id}`

Returns tournament metadata without requiring clients to know `art` values.

```json
{
  "data": {
    "id": "1359649",
    "name": "Tournament name",
    "federation": "LAT",
    "starts_on": "2026-05-20",
    "ends_on": "2026-05-24",
    "date_text": "2026/05/20 - 2026/05/24",
    "round_count": 7,
    "tournament_type": "Swiss-System",
    "time_control": "15 min + 5 sec",
    "last_upstream_update": "2026-05-24T15:42:00+03:00",
    "available_standing_rounds": [1, 2, 3, 4, 5, 6, 7],
    "available_pairing_rounds": [1, 2, 3, 4, 5, 6, 7],
    "extra": {
      "chief_arbiter": "...",
      "location": "..."
    }
  },
  "meta": {}
}
```

Preserve unknown metadata in normalized `snake_case` keys under `extra`, but keep the original label map internally for parser debugging.

### 7.5 Get standings

`GET /api/v1/tournaments/{tournament_id}/standings`

Query parameters:

- `round`: optional positive integer; absent means latest/current ranking;
- `group`: optional exact value for tournaments with a `Typ` column;
- `limit`, `offset`: optional response slicing, applied after parsing/caching;
- `refresh`: optional controlled refresh.

```json
{
  "data": {
    "tournament_id": "1359649",
    "round": 5,
    "heading": "Rank after Round 5",
    "standings": [
      {
        "rank": 1,
        "start_number": "23",
        "player_key": "fide:11600000",
        "name": "Barasovs, Dimitrijs",
        "title": "II",
        "rating": 1468,
        "federation": "LAT",
        "club": "Jazdanovs/Mifan chess",
        "points": 4.5,
        "points_text": "4,5",
        "tie_breaks": {"tb1": "...", "tb2": "..."},
        "group": null,
        "player_results_path": "/api/v1/tournaments/1359649/players/23/results",
        "extra": {}
      }
    ]
  },
  "meta": {}
}
```

The parser must map common multilingual/variant headers through aliases, while retaining unmapped columns in `extra`. Do not drop tie-break fields in the API merely because the current UI drops them.

### 7.6 Get one tournament participant

`GET /api/v1/tournaments/{tournament_id}/players/{start_number}`

Returns the player info table scoped to the tournament plus links to results and global/local player resources.

```json
{
  "data": {
    "tournament_id": "1359649",
    "start_number": "61",
    "player_key": "fide:11653949",
    "name": "Zavalnijs, Grigorijs",
    "title": "II",
    "starting_rank": 61,
    "rating": 0,
    "national_rating": 0,
    "international_rating": 0,
    "performance_rating": 1309,
    "points": 2.0,
    "rank": 26,
    "federation": "LAT",
    "club": "J.Moisejeva",
    "fide_id": "11653949",
    "birth_year": 2019,
    "rating_change": null,
    "extra": {}
  },
  "meta": {}
}
```

### 7.7 Get player results in a tournament

`GET /api/v1/tournaments/{tournament_id}/players/{start_number}/results`

```json
{
  "data": {
    "tournament_id": "1359649",
    "start_number": "61",
    "player": {
      "player_key": "fide:11653949",
      "name": "Zavalnijs, Grigorijs",
      "fide_id": "11653949",
      "points": 2.0,
      "rank": 26
    },
    "games": [
      {
        "round": 1,
        "board": 31,
        "color": null,
        "opponent_start_number": null,
        "opponent_name": "bye",
        "opponent_title": null,
        "opponent_rating": null,
        "opponent_federation": null,
        "opponent_club": null,
        "opponent_points": null,
        "result": "1",
        "result_kind": "bye",
        "source_result_text": "- 1"
      },
      {
        "round": 2,
        "board": 10,
        "color": "white",
        "opponent_start_number": "23",
        "opponent_name": "Barasovs, Dimitrijs",
        "opponent_rating": 1468,
        "opponent_federation": "LAT",
        "opponent_club": "Jazdanovs/Mifan chess",
        "opponent_points": 3.5,
        "result": "0",
        "result_kind": "loss",
        "source_result_text": "0"
      }
    ]
  },
  "meta": {}
}
```

`result` must be a string enum-like value because chess results include more than numeric scores. Suggested normalized values: `1`, `0.5`, `0`, `+`, `-`, `null`. `result_kind`: `win`, `draw`, `loss`, `forfeit_win`, `forfeit_loss`, `bye`, `unplayed`, `unknown`.

Do not infer a bye solely from a missing opponent ID; also inspect opponent name and source result text.

### 7.8 Get round pairings

`GET /api/v1/tournaments/{tournament_id}/rounds/{round}/pairings`

Source route is expected to be `art=2&rd=<round>`. This must be confirmed with real fixtures during implementation.

```json
{
  "data": {
    "tournament_id": "1359649",
    "round": 3,
    "heading": "Round 3",
    "pairings": [
      {
        "board": 1,
        "white": {
          "start_number": "4",
          "name": "Player, White",
          "title": "FM",
          "rating": 2100,
          "points_before_round": 2.0
        },
        "black": {
          "start_number": "9",
          "name": "Player, Black",
          "title": null,
          "rating": 1980,
          "points_before_round": 2.0
        },
        "result": "0.5",
        "status": "finished",
        "extra": {}
      }
    ]
  },
  "meta": {}
}
```

Pairing parser requirements:

- identify a pairing table by heading/header semantics, not only `table.CRs1`;
- support published future pairings with blank results;
- support byes and forfeits;
- resolve both player links and extract each `snr`;
- keep raw result text;
- fixture at least one Swiss tournament and one round-robin/team-like layout before declaring the endpoint stable;
- if an unsupported layout is encountered, return `502 upstream_parse_error`, log a parser fingerprint, and retain the sanitized fixture when explicitly enabled.

### 7.9 Search players

`GET /api/v1/players?q=<name>`

Additional parameters:

- `fide_id`: exact digits-only match;
- `federation`: optional filter;
- `limit`: default 20, maximum 100;
- `offset`: default 0.

Important semantic limitation: the current prototype does not contain an upstream global player-search flow. Therefore v1 player search is a search over the service's local player index, populated whenever tournament standings, player pages, or pairings are parsed. The response must state its scope; it must not pretend to be a complete global Chess-Results directory.

```json
{
  "data": [
    {
      "player_key": "fide:11653949",
      "name": "Zavalnijs, Grigorijs",
      "fide_id": "11653949",
      "federation": "LAT",
      "club": "J.Moisejeva",
      "identity_confidence": "fide",
      "tournament_count": 3,
      "last_seen_at": "2026-07-18T12:00:00Z"
    }
  ],
  "count": 1,
  "scope": "locally_indexed_tournaments"
}
```

Search normalization:

- Unicode-aware lowercase;
- collapse whitespace;
- optionally fold diacritics into an additional search key, never into the displayed name;
- support comma-order names as stored upstream;
- exact FIDE ID match takes precedence;
- rank exact normalized name before prefix, then substring.

### 7.10 Get indexed player

`GET /api/v1/players/{player_key}`

Returns the best merged local identity plus tournament appearances. This is local indexed data and may be stale independently of any one tournament.

## 8. Error contract

All errors use one shape:

```json
{
  "error": {
    "code": "upstream_parse_error",
    "message": "Chess-Results returned a page layout this parser does not recognize.",
    "request_id": "01J...",
    "details": {
      "resource": "tournament_standings",
      "tournament_id": "1359649"
    }
  }
}
```

Codes and status mapping:

- `invalid_request` -> `400`;
- `not_found` -> `404`;
- `method_not_allowed` -> `405`;
- `rate_limited` -> `429` with `Retry-After`;
- `upstream_not_found` -> `404` when confidently detected;
- `upstream_rate_limited` -> `503`;
- `upstream_unavailable` -> `503`;
- `upstream_parse_error` -> `502`;
- `stale_data_unavailable` -> `503`;
- `internal_error` -> `500`.

Never expose upstream HTML, stack traces, SQLite errors, or internal file paths in public responses.

## 9. Upstream HTTP client

### 9.1 URL construction and SSRF prevention

Public callers provide IDs and filters, not arbitrary source URLs. Construct upstream URLs internally.

Allow only HTTPS and an explicit hostname policy:

- `chess-results.com`;
- a constrained pattern such as `s<digits>.chess-results.com` after DNS/IP validation.

Do not offer a generic `?url=` endpoint. If an operator-only import endpoint is later added, reject redirects or redirect targets outside the allowlist and reject loopback, private, link-local, and metadata IP ranges after every resolution.

### 9.2 Client limits

Configure one shared `http.Client` and transport:

- total request timeout: 15 seconds;
- response-header timeout: 8 seconds;
- dial timeout: 5 seconds;
- TLS handshake timeout: 5 seconds;
- max response body: 5 MiB, configurable;
- max idle connections per host: small (2–4);
- automatic decompression allowed;
- user agent identifies the service and includes an operator contact URL/email if public;
- validate status and content type, while tolerating mislabelled HTML content types if the body clearly begins as HTML.

### 9.3 Responsible request policy

- Global upstream concurrency default: 2.
- Per-host minimum interval default: 500–1000 ms.
- Coalesce identical refreshes with `singleflight`.
- Retry at most twice for connect failures, `429`, `502`, `503`, and `504` using exponential backoff plus jitter.
- Honor `Retry-After`.
- Do not retry parse errors, `400`, or confident `404` responses.
- Check Chess-Results terms and robots policy before exposing the service publicly. Cache aggressively and do not crawl tournaments without a user/operator purpose.

## 10. Cache and refresh policy

SQLite is both a parsed-data cache and the local player index. It is not authoritative storage.

Recommended TTL defaults:

| Resource | Active/recent tournament | Old tournament |
|---|---:|---:|
| tournament search | 10 min | n/a |
| tournament metadata | 5 min | 24 h |
| current standings | 2 min | 24 h |
| historical round standings | 1 h | 30 d |
| player tournament results | 2 min | 24 h |
| future/current round pairings | 1 min | n/a |
| completed round pairings | 1 h | 30 d |

Classify “old” as `ends_on < today - 7 days`, configurable.

Use stale-if-error:

- if refresh fails and cached parsed data exists, return it with `meta.stale=true`, `cache=stale-fallback`, an HTTP `Warning` header, and a logged upstream error;
- never silently label stale data fresh;
- configurable maximum stale age, recommended 30 days for completed events and 24 hours for active events.

Background work should be demand-driven:

- refresh recently requested active resources shortly before expiry;
- do not continuously crawl all indexed tournaments;
- cap the refresh queue and drop duplicate/low-priority work;
- stop cleanly on SIGTERM and allow in-flight writes a short grace period.

## 11. Persistence model

Suggested tables:

```text
tournaments(
  id PK, name, federation, starts_on, ends_on, tournament_type,
  time_control, round_count, last_upstream_update, source_url,
  payload_json, fetched_at, parser_version, content_hash
)

standings_snapshots(
  tournament_id, round_key, payload_json, fetched_at,
  parser_version, content_hash,
  PK(tournament_id, round_key)
)

participant_results(
  tournament_id, start_number, player_key, payload_json,
  fetched_at, parser_version, content_hash,
  PK(tournament_id, start_number)
)

pairing_rounds(
  tournament_id, round, payload_json, fetched_at,
  parser_version, content_hash,
  PK(tournament_id, round)
)

players(
  player_key PK, fide_id, canonical_name, normalized_name,
  folded_name, federation, club, identity_confidence,
  first_seen_at, last_seen_at
)

player_appearances(
  player_key, tournament_id, start_number, display_name,
  federation, club, rating, seen_at,
  PK(player_key, tournament_id, start_number)
)

search_cache(
  cache_key PK, payload_json, fetched_at, expires_at, content_hash
)
```

Enable WAL mode, set a busy timeout, and use one writer path. Back up the database by copying through SQLite's backup mechanism or `VACUUM INTO`, not by blindly copying a live WAL database.

Raw HTML should not be retained by default. Optional diagnostic retention must be bounded by size/time and sanitized where possible. HTML can contain names and birth years, so treat it as personal data.

## 12. Parser design

### 12.1 General rules

- Parse HTML with an HTML5 parser, never XML mode.
- Select by semantic evidence (heading plus headers plus relevant links), not a single fragile CSS class.
- Normalize Unicode whitespace including non-breaking spaces.
- Keep `raw_text` internally before typed conversion.
- Parse decimal comma and decimal point.
- Accept absent cells; distinguish absent, blank, and literal `-` internally.
- Resolve URLs using the final response URL after redirects.
- Extract `tnr` and `snr` with URL parsing plus validated path/query patterns.
- Attach a `parser_version` to cached payloads.
- Emit a stable page fingerprint on failures: headings, table count, normalized header rows, response hash, and source route; never log full HTML by default.

### 12.2 Typed conversion

Conversion helpers return `(value, present, error)` or an equivalent typed result. A strange optional value should generally remain raw in `extra`; a structurally essential value such as tournament ID should fail parsing.

Examples:

- `"3,5"` -> `3.5`, raw `"3,5"`;
- `"-"` -> `null`, raw `"-"`;
- FIDE ID `"11 653 949"` -> `"11653949"`;
- upstream FIDE ID `"0"` -> `null`, not `"0"`;
- color marker absent -> `null`, not an empty string.

### 12.3 Tournament search parser

Port the direct-row logic from the prototype. Nested layout tables are common; a parent table must not qualify because a descendant contains a tournament link. Qualifying links must belong directly to a row cell of the selected table.

Map headers through aliases:

```text
DB-Key, dbkey, No.       -> upstream/search ID (not always tournament ID)
Tournament               -> name and source link
FED, Federation          -> federation
from, Start, von         -> starts_on
to, End, bis             -> ends_on
Time-control, Bedenkzeit -> time_control
Last update, Update      -> last_upstream_update
```

Always extract canonical tournament ID from the `tnr...aspx` link rather than trusting a table's numeric column.

### 12.4 Tournament parser

Preserve existing ranking-dialog recognition. Improve metadata-dialog selection by scoring candidates instead of taking the first other headed table:

- +3 for keys such as `Federation`, `Date`, `Number of rounds`, `Tournament type`;
- +2 for a non-ranking `h2`;
- -5 for player-info or pairing headings.

Fail when the score is below a tested threshold instead of returning unrelated layout content.

For columns:

- identify flag-only blank columns and omit them;
- map blank title columns only when sample values match known title patterns;
- canonicalize common headers (`Rk.`, `No.`, `SNo`, `Name`, `Rtg`, `Pts.`, `Club/City`, `FED`, `Typ`);
- preserve every unrecognized non-decoration column in `extra` by normalized label;
- link `Name` cells to `snr` when available.

### 12.5 Player-results parser

Port the existing two-layout support. Improve it by mapping the header row first and using positional fallback only for the known 9/10-cell shapes. This reduces breakage when Chess-Results inserts a column.

Capture all player metadata, including:

- name, title, starting rank;
- rating, national/international rating;
- performance rating, rating change;
- points and final/current rank;
- federation and club/city;
- identifier number, FIDE ID, birth year.

For each game capture round, board, opponent `snr`, title/name/rating/federation/club/current points, color, result, and raw text.

### 12.6 Pairings parser discovery plan

Before implementation:

1. Save fixtures from at least three live tournaments: active Swiss, completed Swiss, and a materially different format.
2. Include one unplayed round, one completed round, one bye, and one forfeit if available.
3. Record headings, direct header labels, and link patterns.
4. Implement semantic identification and alias maps.
5. Add golden JSON tests.
6. Only then finalize any fields that cannot be reliably sourced.

## 13. Security and privacy

- Bind to `127.0.0.1` by default. Put Caddy/nginx/Tailscale in front when remote access is needed.
- If internet-exposed, require an API key or reverse-proxy authentication; store only a hash of keys.
- Apply per-IP/key rate limits locally and stricter limits to `refresh=true`.
- Set maximum URL, header, query, and response sizes.
- Use server read-header, read, write, and idle timeouts.
- CORS disabled by default; configure explicit allowed origins rather than `*` when credentials are used.
- Do not expose a fetch-any-URL facility.
- Avoid logging names in high-volume access logs when not needed.
- Document that the source includes personal data such as player names and sometimes birth years. Provide a cache purge/operator procedure.
- Do not synthesize or guess missing personal attributes.

## 14. Configuration

Environment variables:

```text
LISTEN_ADDR=127.0.0.1:8080
DATABASE_PATH=/var/lib/easy-chess-results/api.sqlite
DEFAULT_FEDERATION=LAT
UPSTREAM_LANGUAGE=1
UPSTREAM_TIMEOUT=15s
UPSTREAM_MAX_CONCURRENCY=2
UPSTREAM_MIN_INTERVAL=750ms
UPSTREAM_MAX_BODY_BYTES=5242880
CACHE_ACTIVE_STANDINGS_TTL=2m
CACHE_COMPLETED_TTL=24h
MAX_STALE_ACTIVE=24h
MAX_STALE_COMPLETED=720h
LOG_LEVEL=info
LOG_FORMAT=json
TRUST_PROXY_HEADERS=false
API_KEYS=                         # empty only for loopback/private deployment
DIAGNOSTIC_HTML_RETENTION=false
```

Validate all configuration at startup. Do not start with an unwritable database directory, malformed durations, wildcard public bind plus missing auth (unless an explicit insecure override is set), or zero/negative safety limits.

## 15. Observability

Structured log fields:

- request ID, method, route pattern, status, duration;
- cache disposition and cache age;
- upstream host, route type, status, duration, attempt count, bytes;
- parser name/version, table/header fingerprint, parse duration;
- refresh queue depth and coalesced waiter count;
- error code, never raw full HTML.

Recommended counters/histograms:

- API requests by route/status;
- upstream requests by route/status;
- cache hits/misses/stale fallbacks;
- parse successes/failures by parser;
- refresh duration;
- SQLite operation duration/errors.

Health endpoints must remain cheap and must not leak configuration secrets.

## 16. Testing strategy

### 16.1 Parser fixtures

Store representative, sanitized HTML in `testdata/`. Each fixture has a golden canonical JSON result. Fixtures should cover:

- tournament search nested inside an outer layout table;
- misleading layout/form rows without tournament links;
- relative and absolute tournament links;
- ranking, “Rank after Round,” and “Starting rank” headings;
- unnamed title column and flag-only column;
- extra/tie-break/group columns;
- player table with and without club column;
- player metadata with no opponent table;
- decimal commas, blank values, `-`, bye, forfeits;
- White and Black CSS markers;
- all pairing layouts supported in v1;
- upstream error/login/empty pages that return HTTP 200.

### 16.2 Unit tests

- URL building and ID extraction;
- allowlist and redirect validation;
- form hidden-field extraction and payload construction;
- header alias normalization;
- numeric/result conversion;
- identity merging rules;
- freshness/stale fallback decisions;
- error mapping.

### 16.3 HTTP integration tests

Use `httptest.Server` as a fake Chess-Results server. Verify:

- the two-step search form flow and cookies/state;
- redirect and timeout behavior;
- response body limit;
- retries and `Retry-After`;
- singleflight coalescing under concurrent identical API requests;
- cached stale response after upstream failure;
- ETag/304 behavior;
- graceful cancellation when the client disconnects.

No routine test should depend on the live Chess-Results site. A manual/cron canary test may do so at a very low rate and should not block normal CI.

### 16.4 Quality gates

```sh
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...        # optional CI tool
```

Cross-compile at least `linux/arm64`. If 32-bit Raspberry Pi OS is supported, also compile and smoke-test the chosen `linux/arm` target.

## 17. Raspberry Pi deployment

Preferred deployment is a native binary managed by systemd. A container adds little value for a single-purpose Pi unless the operator already standardizes on containers.

Build example for 64-bit Raspberry Pi OS:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -trimpath -ldflags='-s -w' -o easy-chess-results-api ./cmd/api
```

If the selected SQLite driver requires CGO, either cross-compile with an appropriate C toolchain or build on the Pi. This is why a CGO-free driver is attractive here.

Systemd hardening outline:

```ini
[Unit]
Description=Easy Chess Results API
After=network-online.target
Wants=network-online.target

[Service]
User=easychess
Group=easychess
ExecStart=/usr/local/bin/easy-chess-results-api
EnvironmentFile=/etc/easy-chess-results-api.env
WorkingDirectory=/var/lib/easy-chess-results
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/easy-chess-results
MemoryMax=256M
TasksMax=64
TimeoutStopSec=20

[Install]
WantedBy=multi-user.target
```

Adjust `MemoryMax` only after measuring fixture-heavy parsing and SQLite behavior on the actual Pi. Do not set an arbitrarily tiny limit that creates restart loops.

For HTTPS/public access, terminate TLS in Caddy or another maintained reverse proxy. Keep the Go service on loopback. Tailscale is a simpler choice for private remote use.

## 18. Delivery phases

### Phase 1 — Core read API

- Go project skeleton and configuration;
- upstream client with safety limits;
- port tournament search, tournament details/standings, and player-results parsers;
- SQLite cache;
- endpoints 7.1–7.7;
- fixture/golden tests;
- systemd deployment.

Acceptance criteria:

- all existing JavaScript parser scenarios have equivalent Go fixtures/tests;
- no public arbitrary-URL fetching;
- repeated identical requests use cache/singleflight;
- stale fallback is explicit;
- binary runs on target Pi architecture.

### Phase 2 — Pairings

- gather and classify real `art=2` fixtures;
- implement pairing parser and endpoint;
- add active-round shorter TTL;
- document supported tournament layouts and unsupported-layout behavior.

### Phase 3 — Player index

- update player/appearance index transactionally after successful parses;
- local player search and merged player endpoint;
- identity collision tests and operator merge/split tooling if real collisions require it.

### Phase 4 — Production polish

- authentication/rate limiting for public exposure;
- metrics and low-rate parser canary;
- backup/restore runbook;
- cache purge endpoints or CLI restricted to operators;
- OpenAPI 3.1 document generated from or tested against the DTO contract.

## 19. Explicit non-goals for v1

- scraping every Chess-Results tournament or building a complete global player database;
- writing results back to Chess-Results;
- user accounts, bookmarks, notifications, or UI;
- engine analysis or PGN reconstruction;
- guessing identities for players without FIDE IDs;
- preserving the prototype's generic table/cell JSON as the public contract;
- promising pairing support before fixture validation.

## 20. Decisions to preserve during project generation

1. Use Go unless there is a strong maintainer-familiarity reason to choose Ruby.
2. Public APIs take validated IDs/filters, never arbitrary upstream URLs.
3. Use stable domain JSON, not HTML-table-shaped JSON.
4. Keep raw source text internally when typed normalization may lose information.
5. Player search is explicitly local-index search until a real upstream global-search flow is researched and implemented.
6. `snr` is tournament-scoped; FIDE ID is optional; name matching is not guaranteed identity.
7. Cache and coalesce aggressively, throttle upstream access, and serve clearly marked stale data on temporary upstream failure.
8. Port the proven parser behaviors from this repository, but build pairings from real fixtures.
9. Keep dependencies and deployment small enough for a Raspberry Pi.
10. Treat parser changes as contract-sensitive work backed by HTML fixtures and golden JSON tests.

## 21. Open questions before implementation

These do not block generating the initial project, but they should become explicit configuration or product decisions:

- Which Raspberry Pi model and whether its OS is 32-bit or 64-bit?
- Is the API private (LAN/Tailscale) or internet-facing?
- Should federation default to `LAT`, or should all searches require it?
- How complete must player search be: locally observed players, a user-selected tournament set, or a separately researched global source?
- Should historical completed tournaments be retained indefinitely or evicted by LRU/age?
- Which Chess-Results tournament formats must pairing v1 guarantee?
- Is SQLite backup required, or is the cache/index fully disposable and rebuildable?
- Does the operator have permission and an acceptable-use plan for the expected scraping volume?

