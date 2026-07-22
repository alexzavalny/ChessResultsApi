package event

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTournamentAndStandings(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "event.html"))
	if e != nil {
		t.Fatal(e)
	}
	event, standing, e := Parse(string(b), "https://s2.chess-results.com/tnr1359649.aspx?art=1&lan=1", "1359649")
	if e != nil {
		t.Fatal(e)
	}
	if event.Name != "Summer Open" || event.RoundCount == nil || *event.RoundCount != 7 || len(event.AvailablePairingRounds) != 1 {
		t.Fatalf("event: %#v", event)
	}
	if len(standing.Standings) != 1 {
		t.Fatalf("standings: %#v", standing)
	}
	s := standing.Standings[0]
	if s.StartNumber != "23" || s.Title == nil || *s.Title != "II" || s.Rating == nil || *s.Rating != 1468 || s.RatingChange == nil || *s.RatingChange != -8.2 || s.Points == nil || *s.Points != 4.5 || s.TieBreaks["tb1"] != "17.0" || s.Group == nil || *s.Group != "U18" {
		t.Fatalf("standing: %#v", s)
	}
}
