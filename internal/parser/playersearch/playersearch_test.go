package playersearch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNestedPlayerSearchTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "player_search.html"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(string(b), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[0].Name != "Jefimovs, Aleksejs" || got[0].TournamentID != "1393979" || got[0].StartNumber != "19" {
		t.Fatalf("unexpected Aleksejs entry: %#v", got[0])
	}
	if got[0].FIDEID != nil {
		t.Fatalf("Aleksejs FIDE ID = %v, want nil", got[0].FIDEID)
	}
	if got[1].FIDEID == nil || *got[1].FIDEID != "11656808" || got[1].StartNumber != "26" {
		t.Fatalf("unexpected Sergejs entry: %#v", got[1])
	}
	if got[1].EndsOn == nil || *got[1].EndsOn != "2026-07-24" {
		t.Fatalf("Sergejs end date = %v", got[1].EndsOn)
	}
}

func TestParsePlayerSearchWithoutMatchesReturnsEmptyList(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "player_search_empty.html"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := Parse(string(b), 100)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("got nil entries, want an empty list")
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0", len(got))
	}
}

func TestParseUnknownPlayerSearchLayoutStillFails(t *testing.T) {
	_, err := Parse("<!doctype html><html><body><p>Unexpected page</p></body></html>", 100)
	if err == nil {
		t.Fatal("expected an unknown layout error")
	}
}
