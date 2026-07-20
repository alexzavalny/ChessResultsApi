package service

import (
	"testing"

	"github.com/alex/easy-chess-results-api/internal/domain"
)

func TestTournamentSearchResultsSortByStartDateAscending(t *testing.T) {
	items := []domain.TournamentSearchResult{
		{ID: "late", StartsOn: stringPointer("2026-08-10")},
		{ID: "unknown"},
		{ID: "early", StartsOn: stringPointer("2026-07-14")},
	}

	sortTournamentSearchResults(items)

	if items[0].ID != "early" || items[1].ID != "late" || items[2].ID != "unknown" {
		t.Fatalf("unexpected order: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
	}
}

func stringPointer(value string) *string { return &value }
