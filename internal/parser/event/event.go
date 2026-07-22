package event

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
)

var aliases = map[string]string{"rk": "rank", "rank": "rank", "no": "start_number", "sno": "start_number", "snr": "start_number", "name": "name", "rtg": "rating", "rating": "rating", "pts": "points", "points": "points", "club_city": "club", "club": "club", "fed": "federation", "federation": "federation", "typ": "group", "type": "group", "title": "title", "titel": "title", "fideid": "fide_id", "fide_id": "fide_id"}
var knownMeta = map[string]string{"federation": "federation", "date": "date_text", "number_of_rounds": "round_count", "tournament_type": "tournament_type", "time_control": "time_control", "name_of_the_tournament": "name", "tournament_name": "name"}

func Parse(html, source, tournamentID string) (domain.Tournament, domain.Standings, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return domain.Tournament{}, domain.Standings{}, err
	}
	ranking, heading := findRanking(doc)
	if ranking == nil {
		return domain.Tournament{}, domain.Standings{}, fmt.Errorf("ranking table not found")
	}
	t := domain.Tournament{ID: tournamentID, SourceURL: source, Extra: map[string]string{}, AvailableStandingRounds: []int{}, AvailablePairingRounds: []int{}}
	parseMetadata(doc, ranking, &t)
	findRounds(doc, &t)
	s, err := parseStandings(ranking, heading, tournamentID)
	if err != nil {
		return t, s, err
	}
	return t, s, nil
}

func findRanking(doc *goquery.Document) (*goquery.Selection, string) {
	var best *goquery.Selection
	heading := ""
	doc.Find("table").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		r := t.Find("tr").First()
		hs := base.Headers(r)
		hasName, hasScore := false, false
		for _, h := range hs {
			k := base.Key(h)
			if k == "name" {
				hasName = true
			}
			if k == "pts" || k == "points" || k == "rtg" || k == "rating" {
				hasScore = true
			}
		}
		if hasName && hasScore {
			best = t
			heading = base.Text(t.Closest(".defaultDialog").Find("h2").First().Text())
			if heading == "" {
				heading = base.Text(t.PrevAllFiltered("h2").First().Text())
			}
			return false
		}
		return true
	})
	return best, heading
}

func parseMetadata(doc *goquery.Document, ranking *goquery.Selection, out *domain.Tournament) {
	bestScore := -999
	var best *goquery.Selection
	doc.Find(".defaultDialog").Each(func(_ int, d *goquery.Selection) {
		candidate := d.Find("table").First()
		if candidate.Length() > 0 && ranking.Length() > 0 && candidate.Get(0) == ranking.Get(0) {
			return
		}
		score := 0
		h := strings.ToLower(base.Text(d.Find("h2").First().Text()))
		if h != "" {
			score += 2
		}
		if strings.Contains(h, "player info") || strings.Contains(h, "pairing") {
			score -= 5
		}
		d.Find("table").First().Find("tr").Each(func(_ int, r *goquery.Selection) {
			c := base.Cells(r)
			if len(c) >= 2 {
				k := base.Key(base.Cell(c, 0))
				if _, ok := knownMeta[k]; ok {
					score += 3
				}
			}
		})
		if score > bestScore {
			bestScore = score
			best = d
		}
	})
	if best == nil || bestScore < 3 {
		return
	}
	best.Find("table").First().Find("tr").Each(func(_ int, r *goquery.Selection) {
		c := base.Cells(r)
		if len(c) < 2 {
			return
		}
		label, val := base.Text(base.Cell(c, 0)), base.Text(base.Cell(c, 1))
		if label == "" || val == "" {
			return
		}
		k := base.Key(label)
		switch knownMeta[k] {
		case "name":
			out.Name = val
		case "federation":
			out.Federation = base.StringPtr(val)
		case "date_text":
			out.DateText = base.StringPtr(val)
			parseDates(val, out)
		case "round_count":
			out.RoundCount = base.Int(val)
		case "tournament_type":
			out.TournamentType = base.StringPtr(val)
		case "time_control":
			out.TimeControl = base.StringPtr(val)
		default:
			out.Extra[k] = val
		}
	})
	last := base.Text(best.Find(".CRsmall").Text())
	if last != "" {
		out.Extra["last_update_text"] = last
	}
}
func parseDates(s string, t *domain.Tournament) {
	re := regexp.MustCompile(`\d{4}[./-]\d{2}[./-]\d{2}`)
	m := re.FindAllString(s, -1)
	if len(m) > 0 {
		x := strings.ReplaceAll(strings.ReplaceAll(m[0], "/", "-"), ".", "-")
		t.StartsOn = &x
	}
	if len(m) > 1 {
		x := strings.ReplaceAll(strings.ReplaceAll(m[1], "/", "-"), ".", "-")
		t.EndsOn = &x
	}
}
func findRounds(doc *goquery.Document, t *domain.Tournament) {
	sm, pm := map[int]bool{}, map[int]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		h, _ := a.Attr("href")
		u := base.Resolve(t.SourceURL, h)
		if strings.Contains(u, "art=1") || strings.Contains(strings.ToLower(base.Text(a.Text())), "rank") {
			if r := queryInt(u, "rd"); r > 0 {
				sm[r] = true
			}
		}
		if strings.Contains(u, "art=2") {
			if r := queryInt(u, "rd"); r > 0 {
				pm[r] = true
			}
		}
	})
	for r := range sm {
		t.AvailableStandingRounds = append(t.AvailableStandingRounds, r)
	}
	for r := range pm {
		t.AvailablePairingRounds = append(t.AvailablePairingRounds, r)
	}
	sort.Ints(t.AvailableStandingRounds)
	sort.Ints(t.AvailablePairingRounds)
}
func queryInt(s, k string) int {
	i := strings.Index(s, k+"=")
	if i < 0 {
		return 0
	}
	v := s[i+len(k)+1:]
	if j := strings.IndexByte(v, '&'); j >= 0 {
		v = v[:j]
	}
	n, _ := strconv.Atoi(v)
	return n
}

func parseStandings(table *goquery.Selection, heading, id string) (domain.Standings, error) {
	rows := table.Find("tr")
	if rows.Length() < 2 {
		return domain.Standings{}, fmt.Errorf("ranking table has no rows")
	}
	headerIndex := -1
	var headers []string
	rows.EachWithBreak(func(i int, row *goquery.Selection) bool {
		candidate := base.HeaderMap(base.Headers(row), aliases)
		if base.At(candidate, "name") >= 0 && (base.At(candidate, "points") >= 0 || base.At(candidate, "rating") >= 0) {
			headerIndex, headers = i, base.Headers(row)
			return false
		}
		return true
	})
	if headerIndex < 0 {
		return domain.Standings{}, fmt.Errorf("ranking header row missing")
	}
	// Header normalization strips punctuation, so Chess-Results' "rtg+/-"
	// would otherwise collide with the distinct "Rtg" column.
	for i, header := range headers {
		compact := strings.ToLower(strings.ReplaceAll(base.Text(header), " ", ""))
		if strings.Contains(compact, "rtg") && strings.Contains(compact, "+/-") {
			headers[i] = "rating_change"
		}
	}
	// Chess-Results commonly leaves the title column header blank. Only assign
	// it when sampled values look like chess titles, so flag/decorative columns
	// remain absent from the domain model.
	for i, h := range headers {
		if base.Key(h) != "" {
			continue
		}
		looksLikeTitle := false
		rows.Slice(headerIndex+1, goquery.ToEnd).EachWithBreak(func(_ int, row *goquery.Selection) bool {
			v := strings.ToUpper(base.Cell(base.Cells(row), i))
			switch v {
			case "GM", "IM", "FM", "CM", "WGM", "WIM", "WFM", "WCM", "I", "II", "III", "IV":
				looksLikeTitle = true
				return false
			}
			return true
		})
		if looksLikeTitle {
			headers[i] = "title"
			break
		}
	}
	hm := base.HeaderMap(headers, aliases)
	if base.At(hm, "name") < 0 {
		return domain.Standings{}, fmt.Errorf("ranking name column missing")
	}
	out := domain.Standings{TournamentID: id, Round: base.RoundFromHeading(heading), Heading: heading, Standings: []domain.Standing{}}
	rows.Slice(headerIndex+1, goquery.ToEnd).Each(func(_ int, r *goquery.Selection) {
		c := base.Cells(r)
		name := base.Cell(c, base.At(hm, "name"))
		if name == "" {
			return
		}
		fed := base.StringPtr(base.Cell(c, base.At(hm, "federation")))
		fide := base.FIDE(base.Cell(c, base.At(hm, "fide_id")))
		sn := base.Cell(c, base.At(hm, "start_number"))
		if a := c[base.At(hm, "name")].Find("a[href]").First(); a.Length() > 0 {
			h, _ := a.Attr("href")
			if x, ok := base.StartNumber(h); ok {
				sn = x
			}
		}
		st := domain.Standing{Rank: base.Int(base.Cell(c, base.At(hm, "rank"))), StartNumber: sn, Name: name, Federation: fed, PlayerKey: base.PlayerKey(name, fide, fed), Title: base.StringPtr(base.Cell(c, base.At(hm, "title"))), Rating: base.Int(base.Cell(c, base.At(hm, "rating"))), RatingChange: base.Float(base.Cell(c, base.At(hm, "rating_change"))), Club: base.StringPtr(base.Cell(c, base.At(hm, "club"))), Points: base.Float(base.Cell(c, base.At(hm, "points"))), PointsText: base.Cell(c, base.At(hm, "points")), Group: base.StringPtr(base.Cell(c, base.At(hm, "group"))), TieBreaks: map[string]string{}, Extra: map[string]string{}, PlayerResultsPath: "/api/v1/tournaments/" + id + "/players/" + sn + "/results"}
		for i, h := range headers {
			k := base.Key(h)
			canon := k
			if a, ok := aliases[k]; ok {
				canon = a
			}
			if strings.HasPrefix(k, "tb") {
				st.TieBreaks[k] = base.Cell(c, i)
			} else if _, known := map[string]bool{"rank": true, "start_number": true, "name": true, "federation": true, "fide_id": true, "title": true, "rating": true, "rating_change": true, "club": true, "points": true, "group": true}[canon]; !known && k != "" {
				st.Extra[k] = base.Cell(c, i)
			}
		}
		out.Standings = append(out.Standings, st)
	})
	return out, nil
}
