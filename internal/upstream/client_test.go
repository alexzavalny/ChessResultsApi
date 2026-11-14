package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchUsesFreshFormState(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "test"})
			_, _ = w.Write([]byte(`<!doctype html><html><form><input type="hidden" name="__VIEWSTATE" value="fresh"></form></html>`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		cookie, _ := r.Cookie("session")
		if cookie == nil || r.Form.Get("__VIEWSTATE") != "fresh" || r.Form.Get("ctl00$P1$combo_land") != "LAT" || r.Form.Get("ctl00$P1$txt_von_tag") != "2026-01-01" {
			t.Errorf("unexpected search transaction: cookie=%v form=%v", cookie, r.Form)
		}
		_, _ = w.Write([]byte(`<!doctype html><html><body>results</body></html>`))
	}))
	defer server.Close()
	c, err := New(server.URL, "1", 2*time.Second, 2, time.Millisecond, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.Search(context.Background(), "LAT", "2026-01-01", "2026-12-31"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestPlayerSearchUsesChessResultsPlayerDatabaseForm(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "player-test"})
			_, _ = w.Write([]byte(`<!doctype html><html><form><input type="hidden" name="__VIEWSTATE" value="fresh-player"></form></html>`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		cookie, _ := r.Cookie("session")
		if cookie == nil ||
			r.Form.Get("__VIEWSTATE") != "fresh-player" ||
			r.Form.Get("ctl00$P1$txt_nachname") != "Jefimovs" ||
			r.Form.Get("ctl00$P1$txt_vorname") != "Aleksejs" ||
			r.Form.Get("ctl00$P1$txt_FED") != "LAT" ||
			r.Form.Get("ctl00$P1$combo_anzahl_zeilen") != "1" {
			t.Errorf("unexpected player search transaction: cookie=%v form=%v", cookie, r.Form)
		}
		_, _ = w.Write([]byte(`<!doctype html><html><body>results</body></html>`))
	}))
	defer server.Close()
	c, err := New(server.URL, "1", 2*time.Second, 2, time.Millisecond, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.SearchPlayers(context.Background(), "Aleksejs", "Jefimovs", "", "LAT"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestFallsBackToNextMirror(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	requests := 0
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer live.Close()

	c, err := New(deadURL+","+live.URL, "1", 2*time.Second, 2, time.Millisecond, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.GetTournament(context.Background(), "1", 1, "", ""); err != nil {
		t.Fatalf("first request: %v", err)
	}
	if requests != 1 {
		t.Fatalf("got %d requests on the live mirror, want 1", requests)
	}
	// The mirror that answered is now preferred, so no second dial to the dead one.
	if _, _, err := c.GetTournament(context.Background(), "1", 1, "", ""); err != nil {
		t.Fatalf("second request: %v", err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests on the live mirror, want 2", requests)
	}
}
