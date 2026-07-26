package pairing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePairingsIncludingSpecialRows(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "pairings.html"))
	if err != nil {
		t.Fatal(err)
	}

	result, err := Parse(string(fixture), "1368916")
	if err != nil {
		t.Fatal(err)
	}
	if result.Round == nil || *result.Round != 9 || result.Heading != "Round 9 on 2026/07/12 at 15:00" {
		t.Fatalf("heading: %#v", result)
	}
	if len(result.Pairings) != 3 {
		t.Fatalf("pairings: %#v", result.Pairings)
	}

	first := result.Pairings[0]
	if first.Board == nil || *first.Board != 1 || first.White.StartNumber == nil || *first.White.StartNumber != "1" ||
		first.White.Name != "Shirov, Alexei" || first.White.Title == nil || *first.White.Title != "GM" ||
		first.White.Rating == nil || *first.White.Rating != 2616 || first.White.Federation == nil || *first.White.Federation != "ESP" ||
		first.White.Points == nil || *first.White.Points != 7.5 || first.Black == nil ||
		first.Black.StartNumber == nil || *first.Black.StartNumber != "2" || first.Black.Name != "Sivuk, Vitaly" ||
		first.Black.Points == nil || *first.Black.Points != 6.5 || first.Result == nil || *first.Result != "½ - ½" ||
		first.Status != "normal" {
		t.Fatalf("first pairing: %#v", first)
	}
	if result.Pairings[1].Status != "bye" || result.Pairings[1].Black != nil || result.Pairings[1].Result == nil || *result.Pairings[1].Result != "1" {
		t.Fatalf("bye: %#v", result.Pairings[1])
	}
	if result.Pairings[2].Status != "not_paired" || result.Pairings[2].Black != nil {
		t.Fatalf("not paired: %#v", result.Pairings[2])
	}
}
