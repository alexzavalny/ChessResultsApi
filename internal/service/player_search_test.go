package service

import (
	"testing"
	"time"

	"github.com/alex/easy-chess-results-api/internal/domain"
)

func TestMergePlayerSearchEntriesKeepsAleksejsAndMergesSergejsByFIDE(t *testing.T) {
	lat, riga, mifans, fide := "LAT", "Rīga", "Mifan Chess/Dmitrijs", "11656808"
	may, april, july := "2026-05-31", "2026-04-20", "2026-07-24"
	entries := []domain.PlayerSearchEntry{
		{Name: "Jefimovs, Aleksejs", Federation: &lat, Club: &mifans, TournamentID: "1393979", TournamentName: "Junioru Kauss", StartNumber: "19", EndsOn: &may},
		{Name: "Jefimovs, Sergejs", FIDEID: &fide, Federation: &lat, Club: &riga, TournamentID: "1431442", TournamentName: "Blitz", StartNumber: "26", EndsOn: &july},
		{Name: "Jefimovs, Sergejs", Federation: &lat, Club: &riga, TournamentID: "1384102", TournamentName: "Rapid", StartNumber: "23", EndsOn: &april},
	}

	got := mergePlayerSearchEntries(entries, time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC))
	if len(got) != 2 {
		t.Fatalf("got %d players, want 2: %#v", len(got), got)
	}
	if got[0].Name != "Jefimovs, Aleksejs" || got[0].PlayerKey == got[1].PlayerKey {
		t.Fatalf("Aleksejs identity was lost: %#v", got)
	}
	sergejs := got[1]
	if sergejs.PlayerKey != "fide:11656808" || sergejs.FIDEID == nil || *sergejs.FIDEID != fide {
		t.Fatalf("unexpected Sergejs identity: %#v", sergejs)
	}
	if len(sergejs.Appearances) != 2 || sergejs.TournamentCount != 2 {
		t.Fatalf("Sergejs appearances were not merged: %#v", sergejs)
	}
	if sergejs.Appearances[0].TournamentID != "1431442" || sergejs.Appearances[0].StartNumber != "26" {
		t.Fatalf("fresh Sergejs tournament target was not preserved: %#v", sergejs.Appearances[0])
	}
}

func TestSplitPlayerQuery(t *testing.T) {
	tests := []struct {
		query, first, last string
	}{
		{"Jefimovs", "", "Jefimovs"},
		{"Aleksejs Jefimovs", "Aleksejs", "Jefimovs"},
		{"Jefimovs, Aleksejs", "Aleksejs", "Jefimovs"},
	}
	for _, tc := range tests {
		first, last := splitPlayerQuery(tc.query)
		if first != tc.first || last != tc.last {
			t.Errorf("splitPlayerQuery(%q) = %q, %q; want %q, %q", tc.query, first, last, tc.first, tc.last)
		}
	}
}
