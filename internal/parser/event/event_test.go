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

func TestParseNameFromHeadingAndBlitzTimeControl(t *testing.T) {
	page := `<!doctype html><html><body>
<div class="defaultDialog"><h3 class="CRmsg">Info</h3><h2>Mikhail Tal Memorial 2026 Rapid  </h2>
<table>
<tr><td>Organizer(s)</td><td>Latvijas Šaha federācija</td></tr>
<tr><td>Federation</td><td>Latvia ( LAT )</td></tr>
<tr><td>Time control (Blitz)</td><td>3 min + 2 sec</td></tr>
</table><div class="CRsmall">Last update 07.11.2026 21:31:19</div></div>
<div class="defaultDialog"><h2>Final Ranking after 8 Rounds</h2><table class="CRs1">
<tr><th>Rk.</th><th>No.</th><th>Name</th><th>Rtg</th><th>Pts.</th></tr>
<tr><td>1</td><td>3</td><td>Quparadze, Giga</td><td>2600</td><td>7,0</td></tr>
</table></div></body></html>`
	event, _, err := Parse(page, "https://s1.chess-results.com/tnr1537042.aspx?art=1&lan=1", "1537042")
	if err != nil {
		t.Fatal(err)
	}
	if event.Name != "Mikhail Tal Memorial 2026 Rapid" {
		t.Fatalf("name: %q", event.Name)
	}
	if event.TimeControl == nil || *event.TimeControl != "3 min + 2 sec" {
		t.Fatalf("time control: %#v", event.TimeControl)
	}
	if event.Federation == nil || *event.Federation != "Latvia ( LAT )" {
		t.Fatalf("federation: %#v", event.Federation)
	}
}

func TestParseNameWhenMetadataIsHidden(t *testing.T) {
	page := `<!doctype html><html><body>
<div class="defaultDialogMsg"><h3>Note: details are shown after clicking the button</h3></div>
<div class="defaultDialog"><h2>Mikhail Tal Memorial 2026 Rapid  </h2>
<table><tr><td class="CR0"><p class="CRsmall">Last update 07.11.2026 21:31:19, Creator: savieniba</p></td></tr></table></div>
<div class="defaultDialog"><h2>Final Ranking after 8 Rounds</h2><table class="CRs1">
<tr><th>Rk.</th><th>No.</th><th>Name</th><th>Rtg</th><th>Pts.</th></tr>
<tr><td>1</td><td>3</td><td>Quparadze, Giga</td><td>2600</td><td>7,0</td></tr>
</table></div></body></html>`
	event, _, err := Parse(page, "https://s1.chess-results.com/tnr1537042.aspx?art=1&lan=1", "1537042")
	if err != nil {
		t.Fatal(err)
	}
	if event.Name != "Mikhail Tal Memorial 2026 Rapid" {
		t.Fatalf("name: %q", event.Name)
	}
}
