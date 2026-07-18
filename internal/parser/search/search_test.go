package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNestedSearchTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "search.html"))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := HiddenFields(string(b))
	if err != nil || hidden["__VIEWSTATE"] != "state" {
		t.Fatalf("hidden fields: %v %v", hidden, err)
	}
	got, err := Parse(string(b), "https://s2.chess-results.com/turniersuche.aspx?lan=1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "1416130" || got[0].Name != "Summer Open" || got[0].SourceURL != "https://s2.chess-results.com/tnr1416130.aspx?lan=1" {
		t.Fatalf("unexpected result: %#v", got)
	}
}
