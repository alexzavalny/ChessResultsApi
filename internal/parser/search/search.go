package search

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
)

var aliases = map[string]string{"tournament": "name", "fed": "federation", "federation": "federation", "from": "starts_on", "start": "starts_on", "von": "starts_on", "to": "ends_on", "end": "ends_on", "bis": "ends_on", "time_control": "time_control", "bedenkzeit": "time_control"}

func HiddenFields(html string) (map[string]string, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	doc.Find(`input[type="hidden"][name]`).Each(func(_ int, s *goquery.Selection) { n, _ := s.Attr("name"); v, _ := s.Attr("value"); out[n] = v })
	if len(out) == 0 {
		return nil, fmt.Errorf("search form has no hidden fields")
	}
	return out, nil
}

func Parse(html, source string, cap int) ([]domain.TournamentSearchResult, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return nil, err
	}
	var table *goquery.Selection
	doc.Find("table").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		first := t.ChildrenFiltered("thead").Find("tr").First()
		if first.Length() == 0 {
			first = t.ChildrenFiltered("tbody").ChildrenFiltered("tr").First()
		}
		if first.Length() == 0 {
			first = t.ChildrenFiltered("tr").First()
		}
		h := base.Headers(first)
		hasName := false
		for _, x := range h {
			if strings.EqualFold(base.Text(x), "Tournament") {
				hasName = true
			}
		}
		if !hasName {
			return true
		}
		direct := false
		t.ChildrenFiltered("tbody").ChildrenFiltered("tr").EachWithBreak(func(_ int, r *goquery.Selection) bool {
			r.ChildrenFiltered("td").Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
				href, _ := a.Attr("href")
				if _, ok := base.TournamentID(href); ok {
					direct = true
					return false
				}
				return true
			})
			return !direct
		})
		if direct {
			table = t
			return false
		}
		return true
	})
	if table == nil {
		return nil, fmt.Errorf("tournament result table not found")
	}
	rows := table.ChildrenFiltered("tbody").ChildrenFiltered("tr")
	if rows.Length() == 0 {
		rows = table.ChildrenFiltered("tr")
	}
	var headers []string
	var hm map[string]int
	out := []domain.TournamentSearchResult{}
	rows.Each(func(_ int, r *goquery.Selection) {
		cells := base.Cells(r)
		if len(cells) == 0 {
			return
		}
		if headers == nil {
			maybe := base.Headers(r)
			for _, h := range maybe {
				if strings.EqualFold(h, "Tournament") {
					headers = maybe
					hm = base.HeaderMap(headers, aliases)
					return
				}
			}
			return
		}
		nameIdx := base.At(hm, "name")
		if nameIdx < 0 || nameIdx >= len(cells) {
			return
		}
		a := cells[nameIdx].Find("a[href]").First()
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		id, ok := base.TournamentID(href)
		if !ok {
			return
		}
		item := domain.TournamentSearchResult{ID: id, Name: base.Text(cells[nameIdx].Text()), SourceURL: base.Resolve(source, href)}
		item.Federation = base.StringPtr(base.Cell(cells, base.At(hm, "federation")))
		item.StartsOn = base.Date(base.Cell(cells, base.At(hm, "starts_on")))
		item.EndsOn = base.Date(base.Cell(cells, base.At(hm, "ends_on")))
		item.TimeControl = base.StringPtr(base.Cell(cells, base.At(hm, "time_control")))
		if len(out) < cap {
			out = append(out, item)
		}
	})
	return out, nil
}
