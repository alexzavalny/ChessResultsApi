PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;

CREATE TABLE IF NOT EXISTS cache_entries (
  cache_key TEXT PRIMARY KEY,
  payload_json BLOB NOT NULL,
  source_url TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  parser_version TEXT NOT NULL,
  content_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS players (
  player_key TEXT PRIMARY KEY,
  fide_id TEXT,
  canonical_name TEXT NOT NULL,
  normalized_name TEXT NOT NULL,
  federation TEXT,
  club TEXT,
  identity_confidence TEXT NOT NULL,
  first_seen_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS players_name_idx ON players(normalized_name);
CREATE INDEX IF NOT EXISTS players_fide_idx ON players(fide_id);
CREATE TABLE IF NOT EXISTS player_appearances (
  player_key TEXT NOT NULL,
  tournament_id TEXT NOT NULL,
  start_number TEXT NOT NULL,
  display_name TEXT NOT NULL,
  federation TEXT,
  club TEXT,
  rating INTEGER,
  seen_at INTEGER NOT NULL,
  PRIMARY KEY(player_key,tournament_id,start_number),
  FOREIGN KEY(player_key) REFERENCES players(player_key) ON DELETE CASCADE
);
