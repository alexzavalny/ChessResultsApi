package player

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
)

var metaAliases = map[string]string{"name": "name", "title": "title", "starting_rank": "starting_rank", "rank": "rank", "rating": "rating", "national_rating": "national_rating", "international_rating": "international_rating", "performance_rating": "performance_rating", "performance": "performance_rating", "points": "points", "federation": "federation", "club_city": "club", "club": "club", "fide_id": "fide_id", "fideid": "fide_id", "year_of_birth": "birth_year", "birth_year": "birth_year", "rating_change": "rating_change", "fide_rtg": "rating_change"}
var gameAliases = map[string]string{"rd": "round", "rnd": "round", "round": "round", "bo": "board", "board": "board", "sno": "opponent_start_number", "no": "opponent_start_number", "name": "opponent_name", "title": "opponent_title", "titel": "opponent_title", "rtg": "opponent_rating", "rtgi": "opponent_rating", "rating": "opponent_rating", "fed": "opponent_federation", "club_city": "opponent_club", "pts": "opponent_points", "res": "result", "result": "result"}

func Parse(html, source, tournamentID, startNumber string) (domain.Participant, domain.PlayerResults, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return domain.Participant{}, domain.PlayerResults{}, err
	}
	var dialog *goquery.Selection
	doc.Find(".defaultDialog").EachWithBreak(func(_ int, d *goquery.Selection) bool {
		if strings.EqualFold(base.Text(d.Find("h2").First().Text()), "Player info") {
			dialog = d
			return false
		}
		return true
	})
	if dialog == nil {
		return domain.Participant{}, domain.PlayerResults{}, fmt.Errorf("player info dialog not found")
	}
	tables := dialog.Find("table")
	if tables.Length() == 0 {
		return domain.Participant{}, domain.PlayerResults{}, fmt.Errorf("player metadata table missing")
	}
	p := domain.Participant{TournamentID: tournamentID, StartNumber: startNumber, Extra: map[string]string{}}
	tables.Eq(0).Find("tr").Each(func(_ int, r *goquery.Selection) {
		c := base.Cells(r)
		for i := 0; i+1 < len(c); i += 2 {
			rawKey, val := base.Key(base.Cell(c, i)), base.Cell(c, i+1)
			k := metaAliases[rawKey]
			switch k {
			case "name":
				p.Name = val
			case "title":
				p.Title = base.StringPtr(val)
			case "starting_rank":
				p.StartingRank = base.Int(val)
			case "rank":
				p.Rank = base.Int(val)
			case "rating":
				p.Rating = base.Int(val)
			case "national_rating":
				p.NationalRating = base.Int(val)
			case "international_rating":
				p.InternationalRating = base.Int(val)
			case "performance_rating":
				p.PerformanceRating = base.Int(val)
			case "points":
				p.Points = base.Float(val)
			case "federation":
				p.Federation = base.StringPtr(val)
			case "club":
				p.Club = base.StringPtr(val)
			case "fide_id":
				p.FIDEID = base.FIDE(val)
			case "birth_year":
				p.BirthYear = base.Int(val)
			case "rating_change":
				p.RatingChange = base.Float(val)
			default:
				if rawKey != "" && val != "" {
					p.Extra[rawKey] = val
				}
			}
		}
	})
	if p.Name == "" {
		return p, domain.PlayerResults{}, fmt.Errorf("player name missing")
	}
	p.PlayerKey = base.PlayerKey(p.Name, p.FIDEID, p.Federation)
	results := domain.PlayerResults{TournamentID: tournamentID, StartNumber: startNumber, Player: domain.PlayerSummary{PlayerKey: p.PlayerKey, Name: p.Name, FIDEID: p.FIDEID, Points: p.Points, Rank: p.Rank}, Games: []domain.Game{}}
	if tables.Length() > 1 {
		results.Games = parseGames(tables.Eq(1))
	}
	if p.RatingChange == nil {
		p.RatingChange = totalRatingChange(results.Games)
	}
	return p, results, nil
}

func parseGames(table *goquery.Selection) []domain.Game {
	rows := table.Find("tr")
	if rows.Length() < 2 {
		return []domain.Game{}
	}
	headers := base.Headers(rows.First())
	for i, header := range headers {
		compact := strings.ToLower(strings.ReplaceAll(base.Text(header), " ", ""))
		if strings.Contains(compact, "rtg") && strings.Contains(compact, "+/-") {
			headers[i] = "rating_change"
		}
	}
	for i, h := range headers {
		if base.Key(h) != "" {
			continue
		}
		rows.Slice(1, goquery.ToEnd).EachWithBreak(func(_ int, row *goquery.Selection) bool {
			v := strings.ToUpper(base.Cell(base.Cells(row), i))
			switch v {
			case "GM", "IM", "FM", "CM", "WGM", "WIM", "WFM", "WCM", "I", "II", "III", "IV":
				headers[i] = "title"
				return false
			}
			return true
		})
		if headers[i] == "title" {
			break
		}
	}
	hm := base.HeaderMap(headers, gameAliases)
	out := []domain.Game{}
	rows.Slice(1, goquery.ToEnd).Each(func(_ int, r *goquery.Selection) {
		c := base.Cells(r)
		if len(c) == 0 {
			return
		}
		nameIdx := base.At(hm, "opponent_name")
		resultIdx := base.At(hm, "result")
		if nameIdx < 0 {
			if len(c) >= 9 {
				nameIdx = 3
			} else {
				return
			}
		}
		if resultIdx < 0 {
			resultIdx = len(c) - 2
		}
		name := base.Cell(c, nameIdx)
		raw := base.Cell(c, resultIdx)
		if name == "" && raw == "" {
			return
		}
		g := domain.Game{Round: base.Int(base.Cell(c, base.At(hm, "round"))), Board: base.Int(base.Cell(c, base.At(hm, "board"))), OpponentName: name, OpponentTitle: base.StringPtr(base.Cell(c, base.At(hm, "opponent_title"))), OpponentRating: base.Int(base.Cell(c, base.At(hm, "opponent_rating"))), OpponentFederation: base.StringPtr(base.Cell(c, base.At(hm, "opponent_federation"))), OpponentClub: base.StringPtr(base.Cell(c, base.At(hm, "opponent_club"))), OpponentPoints: base.Float(base.Cell(c, base.At(hm, "opponent_points"))), RatingChange: base.Float(base.Cell(c, base.At(hm, "rating_change"))), SourceResultText: raw}
		if a := c[nameIdx].Find("a[href]").First(); a.Length() > 0 {
			h, _ := a.Attr("href")
			if sn, ok := base.StartNumber(h); ok {
				g.OpponentStartNumber = &sn
			}
		}
		if r.Find(".FarbewT").Length() > 0 {
			x := "white"
			g.Color = &x
		} else if r.Find(".FarbesT").Length() > 0 {
			x := "black"
			g.Color = &x
		}
		g.Result, g.ResultKind = normalizeResult(raw, name)
		out = append(out, g)
	})
	return out
}

func totalRatingChange(games []domain.Game) *float64 {
	var total float64
	found := false
	for _, game := range games {
		if game.RatingChange != nil {
			total += *game.RatingChange
			found = true
		}
	}
	if !found {
		return nil
	}
	return &total
}

func normalizeResult(raw, name string) (*string, string) {
	s := strings.ToLower(base.Text(raw))
	n := strings.ToLower(base.Text(name))
	bye := strings.Contains(n, "bye") || strings.Contains(n, "spielfrei") || strings.Contains(s, "bye")
	switch s {
	case "1", "1.0", "1,0":
		x := "1"
		if bye {
			return &x, "bye"
		}
		return &x, "win"
	case "½", "0.5", "0,5", "1/2":
		x := "0.5"
		return &x, "draw"
	case "0", "0.0", "0,0":
		x := "0"
		return &x, "loss"
	case "+", "1f":
		x := "+"
		return &x, "forfeit_win"
	case "-", "0f":
		x := "-"
		if bye {
			return &x, "bye"
		}
		return &x, "forfeit_loss"
	case "", "*":
		return nil, "unplayed"
	default:
		if bye {
			x := "1"
			return &x, "bye"
		}
		return nil, "unknown"
	}
}
