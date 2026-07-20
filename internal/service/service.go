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
	eventparser "github.com/alex/easy-chess-results-api/internal/parser/event"
	playerparser "github.com/alex/easy-chess-results-api/internal/parser/player"
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
	return s.store.SearchPlayers(ctx, q, fide, fed, limit, offset)
}
func (s *Service) Player(ctx context.Context, key string) (domain.IndexedPlayer, error) {
	p, e := s.store.GetPlayer(ctx, key)
	if errors.Is(e, sql.ErrNoRows) {
		return p, &Error{NotFound, "player", e}
	}
	return p, e
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
