package pairing

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/alex/easy-chess-results-api/internal/domain"
	base "github.com/alex/easy-chess-results-api/internal/parser"
)

func Parse(html, tournamentID string) (domain.Pairings, error) {
	doc, err := base.RequireDoc(html)
	if err != nil {
		return domain.Pairings{}, err
	}

	table, heading := findTable(doc)
	if table == nil {
		return domain.Pairings{}, fmt.Errorf("pairings table not found")
	}

	rows := table.Find("tr")
	headerIndex := -1
	var columns pairingColumns
	rows.EachWithBreak(func(i int, row *goquery.Selection) bool {
		candidate, ok := columnsFromHeaders(base.Headers(row))
		if ok {
			headerIndex = i
			columns = candidate
			return false
		}
		return true
	})
	if headerIndex < 0 {
		return domain.Pairings{}, fmt.Errorf("pairings header row missing")
	}

	out := domain.Pairings{
		TournamentID: tournamentID,
		Round:        base.RoundFromHeading(heading),
		Heading:      heading,
		Pairings:     []domain.Pairing{},
	}
	rows.Slice(headerIndex+1, goquery.ToEnd).Each(func(_ int, row *goquery.Selection) {
		cells := base.Cells(row)
		whiteName := base.Cell(cells, columns.whiteName)
		if whiteName == "" {
			return
		}

		white := player(cells, columns.whiteName, columns.whiteNumber, columns.whiteFederation, columns.whiteTitle, columns.whiteRating, columns.whitePoints)
		blackName := base.Cell(cells, columns.blackName)
		status := "normal"
		var black *domain.PairingPlayer
		switch strings.ToLower(blackName) {
		case "bye":
			status = "bye"
		case "not paired", "not_paired":
			status = "not_paired"
		case "":
			status = "unpaired"
		default:
			value := player(cells, columns.blackName, columns.blackNumber, columns.blackFederation, columns.blackTitle, columns.blackRating, columns.blackPoints)
			black = &value
		}

		out.Pairings = append(out.Pairings, domain.Pairing{
			Board:  base.Int(base.Cell(cells, columns.board)),
			White:  white,
			Black:  black,
			Result: base.StringPtr(base.Cell(cells, columns.result)),
			Status: status,
		})
	})
	if len(out.Pairings) == 0 {
		return out, fmt.Errorf("pairings table has no rows")
	}
	return out, nil
}

type pairingColumns struct {
	board, whiteNumber, whiteFederation, whiteTitle, whiteName, whiteRating, whitePoints int
	result                                                                               int
	blackPoints, blackTitle, blackName, blackRating, blackFederation, blackNumber        int
}

func columnsFromHeaders(headers []string) (pairingColumns, bool) {
	columns := pairingColumns{
		board: -1, whiteNumber: -1, whiteFederation: -1, whiteTitle: -1, whiteName: -1, whiteRating: -1, whitePoints: -1,
		result:      -1,
		blackPoints: -1, blackTitle: -1, blackName: -1, blackRating: -1, blackFederation: -1, blackNumber: -1,
	}
	numberColumns := []int{}
	ratingColumns := []int{}
	pointsColumns := []int{}
	for i, header := range headers {
		switch base.Key(header) {
		case "bo", "bd", "board":
			columns.board = i
		case "no", "number":
			numberColumns = append(numberColumns, i)
		case "rtg", "rating":
			ratingColumns = append(ratingColumns, i)
		case "pts", "points":
			pointsColumns = append(pointsColumns, i)
		case "white":
			columns.whiteName = i
		case "black":
			columns.blackName = i
		case "result":
			columns.result = i
		}
	}
	if columns.board < 0 || columns.whiteName < 2 || columns.blackName < 2 || columns.result < 0 {
		return columns, false
	}

	for _, index := range numberColumns {
		if index < columns.whiteName && index > columns.whiteNumber {
			columns.whiteNumber = index
		}
		if index > columns.blackName && (columns.blackNumber < 0 || index < columns.blackNumber) {
			columns.blackNumber = index
		}
	}
	for _, index := range ratingColumns {
		if index > columns.whiteName && index < columns.result {
			columns.whiteRating = index
		}
		if index > columns.blackName && (columns.blackRating < 0 || index < columns.blackRating) {
			columns.blackRating = index
		}
	}
	for _, index := range pointsColumns {
		if index > columns.whiteName && index < columns.result {
			columns.whitePoints = index
		}
		if index > columns.result && index < columns.blackName && columns.blackPoints < 0 {
			columns.blackPoints = index
		}
	}

	// With flag=30 Chess-Results inserts a federation-flag cell between No.
	// and title for each player. Without it, the federation is carried by the
	// player-name cell's CSS class. Compact tables can also omit No. and Rtg;
	// their player links still carry the tournament start number.
	if columns.whiteNumber >= 0 && columns.whiteName-columns.whiteNumber >= 3 {
		columns.whiteFederation = columns.whiteName - 2
	}
	columns.whiteTitle = columns.whiteName - 1
	columns.blackTitle = columns.blackName - 1
	if columns.blackNumber >= 0 && columns.blackNumber-columns.blackName >= 3 {
		columns.blackFederation = columns.blackName + 2
	}
	if columns.whitePoints < 0 || columns.blackPoints < 0 ||
		columns.whitePoints >= columns.result || columns.blackPoints <= columns.result ||
		columns.blackPoints >= columns.blackName || columns.blackName >= len(headers) {
		return columns, false
	}
	return columns, true
}

func findTable(doc *goquery.Document) (*goquery.Selection, string) {
	var found *goquery.Selection
	heading := ""
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		table.Find("tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
			if _, ok := columnsFromHeaders(base.Headers(row)); !ok {
				return true
			}
			found = table
			dialog := table.Closest(".defaultDialog")
			heading = base.Text(dialog.Find("h3").First().Text())
			if heading == "" {
				heading = base.Text(dialog.Find("h2").First().Text())
			}
			return false
		})
		return found == nil
	})
	return found, heading
}

func player(cells []*goquery.Selection, name, number, federation, title, rating, points int) domain.PairingPlayer {
	pointsText := base.Cell(cells, points)
	return domain.PairingPlayer{
		StartNumber: startNumberFromCells(cells, number, name),
		Name:        base.Cell(cells, name),
		Title:       base.StringPtr(base.Cell(cells, title)),
		Rating:      base.Int(base.Cell(cells, rating)),
		Federation:  federationFromCells(cells, federation, name),
		Points:      score(pointsText),
		PointsText:  pointsText,
	}
}

func startNumberFromCells(cells []*goquery.Selection, numberIndex, nameIndex int) *string {
	if startNumber := base.StringPtr(base.Cell(cells, numberIndex)); startNumber != nil {
		return startNumber
	}
	if nameIndex < 0 || nameIndex >= len(cells) {
		return nil
	}

	var startNumber string
	cells[nameIndex].Find("a[href]").EachWithBreak(func(_ int, link *goquery.Selection) bool {
		href, _ := link.Attr("href")
		value, ok := base.StartNumber(href)
		if ok {
			startNumber = value
			return false
		}
		return true
	})
	return base.StringPtr(startNumber)
}

func federationFromCells(cells []*goquery.Selection, flagIndex, nameIndex int) *string {
	if flagIndex >= 0 && flagIndex < len(cells) {
		if federation := federationFromFlag(cells[flagIndex]); federation != "" {
			return base.StringPtr(federation)
		}
	}

	if nameIndex >= 0 && nameIndex < len(cells) {
		classes, _ := cells[nameIndex].Attr("class")
		for _, class := range strings.Fields(classes) {
			if isFederationCode(class) {
				return base.StringPtr(strings.ToUpper(class))
			}
		}
	}
	return nil
}

func federationFromFlag(cell *goquery.Selection) string {
	federation := ""
	cell.Find("[class]").EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		classes, _ := selection.Attr("class")
		for _, class := range strings.Fields(classes) {
			if strings.HasPrefix(class, "tn_") && len(class) > 3 {
				federation = strings.ToUpper(strings.TrimPrefix(class, "tn_"))
				return false
			}
		}
		return true
	})
	return federation
}

func isFederationCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func score(value string) *float64 {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	switch value {
	case "½":
		value = "0.5"
	default:
		value = strings.ReplaceAll(value, "½", ".5")
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &number
}
