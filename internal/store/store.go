package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alex/easy-chess-results-api/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ db *sql.DB }
type Cached struct {
	Payload   []byte
	SourceURL string
	FetchedAt time.Time
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	sqlBytes, err := migrations.ReadFile("migrations/001_init.sql")
	if err == nil {
		_, err = db.Exec(string(sqlBytes))
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS readiness_check(v INTEGER)")
	return err
}
func (s *Store) Get(ctx context.Context, key string) (Cached, error) {
	var c Cached
	var ts int64
	err := s.db.QueryRowContext(ctx, "SELECT payload_json,source_url,fetched_at FROM cache_entries WHERE cache_key=?", key).Scan(&c.Payload, &c.SourceURL, &ts)
	c.FetchedAt = time.Unix(ts, 0).UTC()
	return c, err
}
func (s *Store) Put(ctx context.Context, key, source string, payload []byte, at time.Time) error {
	sum := sha256.Sum256(payload)
	_, err := s.db.ExecContext(ctx, `INSERT INTO cache_entries(cache_key,payload_json,source_url,fetched_at,parser_version,content_hash) VALUES(?,?,?,?,?,?) ON CONFLICT(cache_key) DO UPDATE SET payload_json=excluded.payload_json,source_url=excluded.source_url,fetched_at=excluded.fetched_at,parser_version=excluded.parser_version,content_hash=excluded.content_hash`, key, payload, source, at.Unix(), domain.ParserVersion, hex.EncodeToString(sum[:]))
	return err
}

func (s *Store) IndexStandings(ctx context.Context, tournamentID string, items []domain.Standing, seen time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range items {
		confidence := "name_federation"
		var fide any
		if strings.HasPrefix(p.PlayerKey, "fide:") {
			confidence = "fide"
			fide = strings.TrimPrefix(p.PlayerKey, "fide:")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO players(player_key,fide_id,canonical_name,normalized_name,federation,club,identity_confidence,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(player_key) DO UPDATE SET canonical_name=excluded.canonical_name,federation=COALESCE(excluded.federation,players.federation),club=COALESCE(excluded.club,players.club),last_seen_at=excluded.last_seen_at`, p.PlayerKey, fide, p.Name, normalize(p.Name), p.Federation, p.Club, confidence, seen.Unix(), seen.Unix())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO player_appearances(player_key,tournament_id,start_number,display_name,federation,club,rating,seen_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(player_key,tournament_id,start_number) DO UPDATE SET display_name=excluded.display_name,federation=excluded.federation,club=excluded.club,rating=excluded.rating,seen_at=excluded.seen_at`, p.PlayerKey, tournamentID, p.StartNumber, p.Name, p.Federation, p.Club, p.Rating, seen.Unix())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) IndexParticipant(ctx context.Context, p domain.Participant, seen time.Time) error {
	st := domain.Standing{PlayerKey: p.PlayerKey, Name: p.Name, Federation: p.Federation, Club: p.Club, Rating: p.Rating, StartNumber: p.StartNumber}
	return s.IndexStandings(ctx, p.TournamentID, []domain.Standing{st}, seen)
}
func (s *Store) SearchPlayers(ctx context.Context, q, fide, fed string, limit, offset int) ([]domain.IndexedPlayer, error) {
	args := []any{}
	where := []string{"1=1"}
	if fide != "" {
		where = append(where, "p.fide_id=?")
		args = append(args, fide)
	} else if q != "" {
		where = append(where, "p.normalized_name LIKE ?")
		args = append(args, "%"+normalize(q)+"%")
	}
	if fed != "" {
		where = append(where, "p.federation=?")
		args = append(args, strings.ToUpper(fed))
	}
	args = append(args, normalize(q), normalize(q)+"%", limit, offset)
	query := `SELECT p.player_key,p.canonical_name,p.fide_id,p.federation,p.club,p.identity_confidence,COUNT(DISTINCT a.tournament_id),p.last_seen_at FROM players p LEFT JOIN player_appearances a ON a.player_key=p.player_key WHERE ` + strings.Join(where, " AND ") + ` GROUP BY p.player_key ORDER BY CASE WHEN p.normalized_name=? THEN 0 WHEN p.normalized_name LIKE ? THEN 1 ELSE 2 END,p.canonical_name LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.IndexedPlayer{}
	for rows.Next() {
		var p domain.IndexedPlayer
		var fideID, federation, club sql.NullString
		var ts int64
		if err = rows.Scan(&p.PlayerKey, &p.Name, &fideID, &federation, &club, &p.IdentityConfidence, &p.TournamentCount, &ts); err != nil {
			return nil, err
		}
		p.FIDEID = nullString(fideID)
		p.Federation = nullString(federation)
		p.Club = nullString(club)
		p.LastSeenAt = time.Unix(ts, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) GetPlayer(ctx context.Context, key string) (domain.IndexedPlayer, error) {
	var p domain.IndexedPlayer
	var fide, fed, club sql.NullString
	var ts int64
	err := s.db.QueryRowContext(ctx, `SELECT player_key,canonical_name,fide_id,federation,club,identity_confidence,last_seen_at FROM players WHERE player_key=?`, key).Scan(&p.PlayerKey, &p.Name, &fide, &fed, &club, &p.IdentityConfidence, &ts)
	if err != nil {
		return p, err
	}
	p.FIDEID = nullString(fide)
	p.Federation = nullString(fed)
	p.Club = nullString(club)
	p.LastSeenAt = time.Unix(ts, 0).UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT tournament_id,start_number,display_name,federation,club,rating,seen_at FROM player_appearances WHERE player_key=? ORDER BY seen_at DESC`, key)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var a domain.Appearance
		var f, c sql.NullString
		var rating sql.NullInt64
		var seen int64
		if err = rows.Scan(&a.TournamentID, &a.StartNumber, &a.DisplayName, &f, &c, &rating, &seen); err != nil {
			return p, err
		}
		a.Federation = nullString(f)
		a.Club = nullString(c)
		if rating.Valid {
			x := int(rating.Int64)
			a.Rating = &x
		}
		a.SeenAt = time.Unix(seen, 0).UTC()
		p.Appearances = append(p.Appearances, a)
	}
	p.TournamentCount = len(p.Appearances)
	return p, rows.Err()
}
func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func nullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

var ErrNotFound = errors.New("not found")
