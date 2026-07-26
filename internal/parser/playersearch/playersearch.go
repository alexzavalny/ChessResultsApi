package playersearch

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
)

var aliases = map[string]string{
	"name":       "name",
	"fideid":     "fide_id",
	"fide_id":    "fide_id",
	"club_city":  "club",
	"club":       "club",
	"fed":        "federation",
	"federation": "federation",
	"tournament": "tournament",
	"end_date":   "ends_on",
}

func Parse(html string, cap int) ([]domain.PlayerSearchEntry, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return nil, err
	}

	var resultRows *goquery.Selection
	var headers []string
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		rows := directRows(table)
		rows.EachWithBreak(func(_ int, row *goquery.Selection) bool {
			candidate := base.Headers(row)
			hm := base.HeaderMap(candidate, aliases)
			if base.At(hm, "name") >= 0 && base.At(hm, "fide_id") >= 0 && base.At(hm, "tournament") >= 0 {
				headers = candidate
				resultRows = rows
				return false
			}
			return true
		})
		return resultRows == nil
	})
	if resultRows == nil {
		if isPlayerDatabasePage(doc) {
			return []domain.PlayerSearchEntry{}, nil
		}
		return nil, fmt.Errorf("player search result table not found")
	}

	hm := base.HeaderMap(headers, aliases)
	out := []domain.PlayerSearchEntry{}
	resultRows.Each(func(_ int, row *goquery.Selection) {
		if len(out) >= cap {
			return
		}
		cells := base.Cells(row)
		nameIndex, tournamentIndex := base.At(hm, "name"), base.At(hm, "tournament")
		if nameIndex < 0 || tournamentIndex < 0 || nameIndex >= len(cells) || tournamentIndex >= len(cells) {
			return
		}
		playerLink := cells[nameIndex].Find("a[href]").First()
		playerHref, ok := playerLink.Attr("href")
		if !ok {
			return
		}
		startNumber, ok := base.StartNumber(playerHref)
		if !ok {
			return
		}
		tournamentLink := cells[tournamentIndex].Find("a[href]").First()
		tournamentHref, ok := tournamentLink.Attr("href")
		if !ok {
			return
		}
		tournamentID, ok := base.TournamentID(tournamentHref)
		if !ok {
			return
		}
		name := base.Cell(cells, nameIndex)
		tournamentName := base.Cell(cells, tournamentIndex)
		if name == "" || tournamentName == "" {
			return
		}
		out = append(out, domain.PlayerSearchEntry{
			Name:           name,
			FIDEID:         base.FIDE(base.Cell(cells, base.At(hm, "fide_id"))),
			Federation:     base.StringPtr(base.Cell(cells, base.At(hm, "federation"))),
			Club:           base.StringPtr(base.Cell(cells, base.At(hm, "club"))),
			TournamentID:   tournamentID,
			TournamentName: tournamentName,
			StartNumber:    startNumber,
			EndsOn:         base.Date(base.Cell(cells, base.At(hm, "ends_on"))),
		})
	})
	if len(out) == 0 && isPlayerDatabasePage(doc) {
		return out, nil
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("player search result rows not found")
	}
	return out, nil
}

func isPlayerDatabasePage(doc *goquery.Document) bool {
	return strings.Contains(strings.ToLower(base.Text(doc.Text())), "player-database of chess-results")
}

func directRows(table *goquery.Selection) *goquery.Selection {
	rows := table.ChildrenFiltered("tbody").ChildrenFiltered("tr")
	if rows.Length() == 0 {
		rows = table.ChildrenFiltered("tr")
	}
	return rows
}
