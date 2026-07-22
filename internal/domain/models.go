package domain

import "time"

const ParserVersion = "1"

type Meta struct {
	SourceURL  string    `json:"source_url"`
	FetchedAt  time.Time `json:"fetched_at"`
	AgeSeconds int64     `json:"age_seconds"`
	Cache      string    `json:"cache"`
	Stale      bool      `json:"stale"`
}

type Envelope[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

type Tournament struct {
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	Federation              *string           `json:"federation"`
	StartsOn                *string           `json:"starts_on"`
	EndsOn                  *string           `json:"ends_on"`
	DateText                *string           `json:"date_text"`
	RoundCount              *int              `json:"round_count"`
	TournamentType          *string           `json:"tournament_type"`
	TimeControl             *string           `json:"time_control"`
	LastUpstreamUpdate      *string           `json:"last_upstream_update"`
	AvailableStandingRounds []int             `json:"available_standing_rounds"`
	AvailablePairingRounds  []int             `json:"available_pairing_rounds"`
	SourceURL               string            `json:"source_url,omitempty"`
	Extra                   map[string]string `json:"extra"`
}

type TournamentSearchResult struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Federation  *string `json:"federation"`
	StartsOn    *string `json:"starts_on"`
	EndsOn      *string `json:"ends_on"`
	TimeControl *string `json:"time_control"`
	SourceURL   string  `json:"source_url"`
}

type SearchResponse struct {
	Data  []TournamentSearchResult `json:"data"`
	Count int                      `json:"count"`
	Meta  Meta                     `json:"meta"`
}

type Standing struct {
	Rank              *int              `json:"rank"`
	StartNumber       string            `json:"start_number"`
	PlayerKey         string            `json:"player_key"`
	Name              string            `json:"name"`
	Title             *string           `json:"title"`
	Rating            *int              `json:"rating"`
	Federation        *string           `json:"federation"`
	Club              *string           `json:"club"`
	Points            *float64          `json:"points"`
	PointsText        string            `json:"points_text"`
	TieBreaks         map[string]string `json:"tie_breaks"`
	Group             *string           `json:"group"`
	PlayerResultsPath string            `json:"player_results_path"`
	Extra             map[string]string `json:"extra"`
}

type Standings struct {
	TournamentID string     `json:"tournament_id"`
	Round        *int       `json:"round"`
	Heading      string     `json:"heading"`
	Standings    []Standing `json:"standings"`
}

type Participant struct {
	TournamentID        string            `json:"tournament_id"`
	StartNumber         string            `json:"start_number"`
	PlayerKey           string            `json:"player_key"`
	Name                string            `json:"name"`
	Title               *string           `json:"title"`
	StartingRank        *int              `json:"starting_rank"`
	Rating              *int              `json:"rating"`
	NationalRating      *int              `json:"national_rating"`
	InternationalRating *int              `json:"international_rating"`
	PerformanceRating   *int              `json:"performance_rating"`
	Points              *float64          `json:"points"`
	Rank                *int              `json:"rank"`
	Federation          *string           `json:"federation"`
	Club                *string           `json:"club"`
	FIDEID              *string           `json:"fide_id"`
	BirthYear           *int              `json:"birth_year"`
	RatingChange        *float64          `json:"rating_change"`
	Extra               map[string]string `json:"extra"`
}

type Game struct {
	Round               *int     `json:"round"`
	Board               *int     `json:"board"`
	Color               *string  `json:"color"`
	OpponentStartNumber *string  `json:"opponent_start_number"`
	OpponentName        string   `json:"opponent_name"`
	OpponentTitle       *string  `json:"opponent_title"`
	OpponentRating      *int     `json:"opponent_rating"`
	OpponentFederation  *string  `json:"opponent_federation"`
	OpponentClub        *string  `json:"opponent_club"`
	OpponentPoints      *float64 `json:"opponent_points"`
	RatingChange        *float64 `json:"rating_change"`
	Result              *string  `json:"result"`
	ResultKind          string   `json:"result_kind"`
	SourceResultText    string   `json:"source_result_text"`
}

type PlayerSummary struct {
	PlayerKey string   `json:"player_key"`
	Name      string   `json:"name"`
	FIDEID    *string  `json:"fide_id"`
	Points    *float64 `json:"points"`
	Rank      *int     `json:"rank"`
}

type PlayerResults struct {
	TournamentID string        `json:"tournament_id"`
	StartNumber  string        `json:"start_number"`
	Player       PlayerSummary `json:"player"`
	Games        []Game        `json:"games"`
}

type IndexedPlayer struct {
	PlayerKey          string       `json:"player_key"`
	Name               string       `json:"name"`
	FIDEID             *string      `json:"fide_id"`
	Federation         *string      `json:"federation"`
	Club               *string      `json:"club"`
	IdentityConfidence string       `json:"identity_confidence"`
	TournamentCount    int          `json:"tournament_count"`
	LastSeenAt         time.Time    `json:"last_seen_at"`
	Appearances        []Appearance `json:"appearances,omitempty"`
}

type Appearance struct {
	TournamentID string    `json:"tournament_id"`
	StartNumber  string    `json:"start_number"`
	DisplayName  string    `json:"display_name"`
	Federation   *string   `json:"federation"`
	Club         *string   `json:"club"`
	Rating       *int      `json:"rating"`
	SeenAt       time.Time `json:"seen_at"`
}
