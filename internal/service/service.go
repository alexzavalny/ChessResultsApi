package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alex/easy-chess-results-api/internal/config"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
	eventparser "github.com/alex/easy-chess-results-api/internal/parser/event"
	playerparser "github.com/alex/easy-chess-results-api/internal/parser/player"
	playersearchparser "github.com/alex/easy-chess-results-api/internal/parser/playersearch"
	searchparser "github.com/alex/easy-chess-results-api/internal/parser/search"
	"github.com/alex/easy-chess-results-api/internal/store"
	"github.com/alex/easy-chess-results-api/internal/upstream"
	"golang.org/x/sync/singleflight"
)

type Kind string

const (
	ParseError  Kind = "parse"
	Unavailable Kind = "unavailable"
	NotFound    Kind = "not_found"
)

type Error struct {
	Kind     Kind
	Resource string
	Err      error
}

func (e *Error) Error() string { return e.Resource + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Service struct {
	cfg      config.Config
	store    *store.Store
	upstream *upstream.Client
	group    singleflight.Group
	now      func() time.Time
}

func New(cfg config.Config, st *store.Store, up *upstream.Client) *Service {
	return &Service{cfg: cfg, store: st, upstream: up, now: time.Now}
}

func (s *Service) SearchTournaments(ctx context.Context, fed, from, to, q, timeControl string, refresh bool) ([]domain.TournamentSearchResult, domain.Meta, error) {
	key := "search:" + strings.Join([]string{fed, from, to}, "|")
	var data []domain.TournamentSearchResult
	meta, err := s.cached(ctx, key, s.cfg.SearchTTL, s.cfg.MaxStaleActive, refresh, &data, func(ctx context.Context) (any, string, error) {
		html, source, e := s.upstream.Search(ctx, fed, from, to)
		if e != nil {
			return nil, "", &Error{Unavailable, "tournament_search", e}
		}
		items, e := searchparser.Parse(html, source, s.cfg.MaxSearchResults)
		if e != nil {
			return nil, "", &Error{ParseError, "tournament_search", e}
		}
		return items, source, nil
	})
	if err != nil {
		return nil, meta, err
	}
	sortTournamentSearchResults(data)
	filtered := data[:0]
	for _, t := range data {
		if q != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(q)) {
			continue
		}
		if timeControl != "" && (t.TimeControl == nil || !strings.Contains(strings.ToLower(*t.TimeControl), strings.ToLower(timeControl))) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered, meta, nil
}

func sortTournamentSearchResults(data []domain.TournamentSearchResult) {
	sort.SliceStable(data, func(i, j int) bool {
		left, right := data[i].StartsOn, data[j].StartsOn
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return *left < *right
	})
}

func (s *Service) Tournament(ctx context.Context, id string, refresh bool) (domain.Tournament, domain.Meta, error) {
	var t domain.Tournament
	meta, err := s.cached(ctx, "tournament:"+id, s.cfg.ActiveStandingsTTL, s.cfg.MaxStaleActive, refresh, &t, func(ctx context.Context) (any, string, error) {
		html, source, e := s.upstream.GetTournament(ctx, id, 1, "", "")
		if e != nil {
			return nil, "", upErr("tournament", e)
		}
		t, standings, e := eventparser.Parse(html, source, id)
		if e != nil {
			return nil, "", &Error{ParseError, "tournament", e}
		}
		now := s.now().UTC()
		b, _ := json.Marshal(standings)
		_ = s.store.Put(ctx, "standings:"+id+":latest", source, b, now)
		_ = s.store.IndexStandings(ctx, id, standings.Standings, now)
		return t, source, nil
	})
	return t, meta, err
}

func (s *Service) Standings(ctx context.Context, id string, round *int, refresh bool) (domain.Standings, domain.Meta, error) {
	roundKey := "latest"
	roundArg := ""
	ttl := s.cfg.ActiveStandingsTTL
	if round != nil {
		roundKey = strconv.Itoa(*round)
		roundArg = roundKey
		ttl = s.cfg.CompletedTTL
	}
	var result domain.Standings
	meta, err := s.cached(ctx, "standings:"+id+":"+roundKey, ttl, s.cfg.MaxStaleCompleted, refresh, &result, func(ctx context.Context) (any, string, error) {
		html, source, e := s.upstream.GetTournament(ctx, id, 1, roundArg, "")
		if e != nil {
			return nil, "", upErr("tournament_standings", e)
		}
		t, standings, e := eventparser.Parse(html, source, id)
		if e != nil {
			return nil, "", &Error{ParseError, "tournament_standings", e}
		}
		now := s.now().UTC()
		tb, _ := json.Marshal(t)
		_ = s.store.Put(ctx, "tournament:"+id, source, tb, now)
		if e = s.store.IndexStandings(ctx, id, standings.Standings, now); e != nil {
			return nil, "", fmt.Errorf("index standings: %w", e)
		}
		return standings, source, nil
	})
	return result, meta, err
}

func (s *Service) Participant(ctx context.Context, id, start string, refresh bool) (domain.Participant, domain.Meta, error) {
	var p domain.Participant
	meta, err := s.cached(ctx, "participant:"+id+":"+start, s.cfg.ActiveStandingsTTL, s.cfg.MaxStaleActive, refresh, &p, func(ctx context.Context) (any, string, error) {
		p, r, source, e := s.fetchPlayer(ctx, id, start)
		if e != nil {
			return nil, "", e
		}
		now := s.now().UTC()
		b, _ := json.Marshal(r)
		_ = s.store.Put(ctx, "results:"+id+":"+start, source, b, now)
		if e = s.store.IndexParticipant(ctx, p, now); e != nil {
			return nil, "", e
		}
		return p, source, nil
	})
	return p, meta, err
}
func (s *Service) PlayerResults(ctx context.Context, id, start string, refresh bool) (domain.PlayerResults, domain.Meta, error) {
	var r domain.PlayerResults
	meta, err := s.cached(ctx, "results:"+id+":"+start, s.cfg.ActiveStandingsTTL, s.cfg.MaxStaleActive, refresh, &r, func(ctx context.Context) (any, string, error) {
		p, r, source, e := s.fetchPlayer(ctx, id, start)
		if e != nil {
			return nil, "", e
		}
		now := s.now().UTC()
		b, _ := json.Marshal(p)
		_ = s.store.Put(ctx, "participant:"+id+":"+start, source, b, now)
		if e = s.store.IndexParticipant(ctx, p, now); e != nil {
			return nil, "", e
		}
		return r, source, nil
	})
	return r, meta, err
}
func (s *Service) fetchPlayer(ctx context.Context, id, start string) (domain.Participant, domain.PlayerResults, string, error) {
	html, source, e := s.upstream.GetTournament(ctx, id, 9, "", start)
	if e != nil {
		return domain.Participant{}, domain.PlayerResults{}, "", upErr("tournament_player", e)
	}
	p, r, e := playerparser.Parse(html, source, id, start)
	if e != nil {
		return p, r, "", &Error{ParseError, "tournament_player", e}
	}
	return p, r, source, nil
}

func (s *Service) SearchPlayers(ctx context.Context, q, fide, fed string, limit, offset int) ([]domain.IndexedPlayer, error) {
	q = strings.TrimSpace(q)
	if fide == "" && q != "" && allDigits(q) {
		fide, q = q, ""
	}
	if q == "" && fide == "" {
		return s.store.SearchPlayers(ctx, q, fide, fed, limit, offset)
	}
	firstName, lastName := splitPlayerQuery(q)
	key := "player-search:" + strings.Join([]string{firstName, lastName, fide, fed}, "|")
	value, err, _ := s.group.Do(key, func() (any, error) {
		html, _, e := s.upstream.SearchPlayers(ctx, firstName, lastName, fide, fed)
		if e != nil {
			return nil, &Error{Unavailable, "player_search", e}
		}
		cap := s.cfg.MaxSearchResults
		if cap <= 0 {
			cap = 500
		}
		entries, e := playersearchparser.Parse(html, cap)
		if e != nil {
			return nil, &Error{ParseError, "player_search", e}
		}
		return mergePlayerSearchEntries(entries, s.now().UTC()), nil
	})
	if err != nil {
		return nil, err
	}
	items := value.([]domain.IndexedPlayer)
	if offset >= len(items) {
		return []domain.IndexedPlayer{}, nil
	}
	end := min(offset+limit, len(items))
	return items[offset:end], nil
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}
func (s *Service) Player(ctx context.Context, key string) (domain.IndexedPlayer, error) {
	p, e := s.store.GetPlayer(ctx, key)
	if errors.Is(e, sql.ErrNoRows) {
		return p, &Error{NotFound, "player", e}
	}
	return p, e
}

func splitPlayerQuery(q string) (string, string) {
	q = strings.TrimSpace(q)
	if before, after, ok := strings.Cut(q, ","); ok {
		return strings.TrimSpace(after), strings.TrimSpace(before)
	}
	parts := strings.Fields(q)
	if len(parts) < 2 {
		return "", q
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

func mergePlayerSearchEntries(entries []domain.PlayerSearchEntry, fallbackTime time.Time) []domain.IndexedPlayer {
	fideByName := map[string]map[string]struct{}{}
	for _, entry := range entries {
		if entry.FIDEID == nil {
			continue
		}
		nameKey := playerNameKey(entry.Name, entry.Federation)
		if fideByName[nameKey] == nil {
			fideByName[nameKey] = map[string]struct{}{}
		}
		fideByName[nameKey][*entry.FIDEID] = struct{}{}
	}

	type playerGroup struct {
		player      domain.IndexedPlayer
		appearances map[string]domain.Appearance
		tournaments map[string]struct{}
	}
	groups := map[string]*playerGroup{}
	order := []string{}
	for _, entry := range entries {
		playerKey := basePlayerKey(entry, fideByName)
		group := groups[playerKey]
		if group == nil {
			confidence := "name_federation"
			var fideID *string
			if strings.HasPrefix(playerKey, "fide:") {
				confidence = "fide"
				id := strings.TrimPrefix(playerKey, "fide:")
				fideID = &id
			}
			group = &playerGroup{
				player: domain.IndexedPlayer{
					PlayerKey: playerKey, Name: entry.Name, FIDEID: fideID, Federation: entry.Federation,
					Club: entry.Club, IdentityConfidence: confidence,
				},
				appearances: map[string]domain.Appearance{},
				tournaments: map[string]struct{}{},
			}
			groups[playerKey] = group
			order = append(order, playerKey)
		}
		seenAt := fallbackTime
		if entry.EndsOn != nil {
			if parsed, err := time.Parse("2006-01-02", *entry.EndsOn); err == nil {
				seenAt = parsed.UTC()
			}
		}
		tournamentName := entry.TournamentName
		appearance := domain.Appearance{
			TournamentID: entry.TournamentID, TournamentName: &tournamentName, StartNumber: entry.StartNumber,
			DisplayName: entry.Name, Federation: entry.Federation, Club: entry.Club, EndsOn: entry.EndsOn, SeenAt: seenAt,
		}
		appearanceKey := entry.TournamentID + ":" + entry.StartNumber
		if existing, ok := group.appearances[appearanceKey]; !ok || appearance.SeenAt.After(existing.SeenAt) {
			group.appearances[appearanceKey] = appearance
		}
		group.tournaments[entry.TournamentID] = struct{}{}
		if group.player.LastSeenAt.IsZero() || seenAt.After(group.player.LastSeenAt) {
			group.player.Name = entry.Name
			group.player.Federation = entry.Federation
			group.player.Club = entry.Club
			group.player.LastSeenAt = seenAt
		}
	}

	out := make([]domain.IndexedPlayer, 0, len(order))
	for _, key := range order {
		group := groups[key]
		for _, appearance := range group.appearances {
			group.player.Appearances = append(group.player.Appearances, appearance)
		}
		sort.Slice(group.player.Appearances, func(i, j int) bool {
			return group.player.Appearances[i].SeenAt.After(group.player.Appearances[j].SeenAt)
		})
		group.player.TournamentCount = len(group.tournaments)
		out = append(out, group.player)
	}
	return out
}

func basePlayerKey(entry domain.PlayerSearchEntry, fideByName map[string]map[string]struct{}) string {
	if entry.FIDEID != nil {
		return "fide:" + *entry.FIDEID
	}
	ids := fideByName[playerNameKey(entry.Name, entry.Federation)]
	if len(ids) == 1 {
		for id := range ids {
			return "fide:" + id
		}
	}
	return base.PlayerKey(entry.Name, nil, entry.Federation)
}

func playerNameKey(name string, federation *string) string {
	fed := ""
	if federation != nil {
		fed = strings.ToUpper(*federation)
	}
	return strings.ToLower(strings.Join(strings.Fields(name), " ")) + "\x00" + fed
}

func (s *Service) cached(ctx context.Context, key string, ttl, maxStale time.Duration, refresh bool, dst any, fetch func(context.Context) (any, string, error)) (domain.Meta, error) {
	now := s.now().UTC()
	cached, cacheErr := s.store.Get(ctx, key)
	if cacheErr == nil && !refresh && now.Sub(cached.FetchedAt) <= ttl {
		if e := json.Unmarshal(cached.Payload, dst); e == nil {
			return meta(cached, now, "hit", false), nil
		}
	}
	v, err, _ := s.group.Do(key, func() (any, error) {
		value, source, e := fetch(ctx)
		if e != nil {
			return nil, e
		}
		b, e := json.Marshal(value)
		if e != nil {
			return nil, e
		}
		at := s.now().UTC()
		if e = s.store.Put(ctx, key, source, b, at); e != nil {
			return nil, e
		}
		return store.Cached{Payload: b, SourceURL: source, FetchedAt: at}, nil
	})
	if err == nil {
		fresh := v.(store.Cached)
		if e := json.Unmarshal(fresh.Payload, dst); e != nil {
			return domain.Meta{}, e
		}
		disposition := "miss"
		if cacheErr == nil || refresh {
			disposition = "refreshed"
		}
		return meta(fresh, now, disposition, false), nil
	}
	if cacheErr == nil && now.Sub(cached.FetchedAt) <= maxStale {
		if e := json.Unmarshal(cached.Payload, dst); e == nil {
			return meta(cached, now, "stale-fallback", true), nil
		}
	}
	return domain.Meta{}, err
}
func meta(c store.Cached, now time.Time, disposition string, stale bool) domain.Meta {
	age := now.Sub(c.FetchedAt)
	if age < 0 {
		age = 0
	}
	return domain.Meta{SourceURL: c.SourceURL, FetchedAt: c.FetchedAt, AgeSeconds: int64(age.Seconds()), Cache: disposition, Stale: stale}
}
func upErr(resource string, e error) error {
	var h *upstream.HTTPError
	if errors.As(e, &h) && h.Status == 404 {
		return &Error{NotFound, resource, e}
	}
	return &Error{Unavailable, resource, e}
}
