package player

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlayerAndGames(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "player.html"))
	if e != nil {
		t.Fatal(e)
	}
	p, r, e := Parse(string(b), "https://s2.chess-results.com/tnr1359649.aspx?art=9&snr=61", "1359649", "61")
	if e != nil {
		t.Fatal(e)
	}
	if p.FIDEID == nil || *p.FIDEID != "11653949" || p.BirthYear == nil || *p.BirthYear != 2019 {
		t.Fatalf("player: %#v", p)
	}
	if len(r.Games) != 2 {
		t.Fatalf("games: %#v", r.Games)
	}
	if r.Games[0].Round == nil || *r.Games[0].Round != 1 || r.Games[1].Round == nil || *r.Games[1].Round != 2 || r.Games[0].ResultKind != "bye" || r.Games[1].Color == nil || *r.Games[1].Color != "white" || r.Games[1].OpponentStartNumber == nil || *r.Games[1].OpponentStartNumber != "23" {
		t.Fatalf("games: %#v", r.Games)
	}
}
