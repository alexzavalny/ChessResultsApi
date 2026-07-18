package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

var (
	tournamentPath = regexp.MustCompile(`(?i)tnr(\d+)\.aspx`)
	roundHeading   = regexp.MustCompile(`(?i)(?:round|runde)\s*(\d+)`)
	nonDigits      = regexp.MustCompile(`\D+`)
)

func Text(s string) string { return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ") }
func Key(s string) string {
	s = strings.ToLower(Text(s))
	var b strings.Builder
	last := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			last = false
		} else if !last {
			b.WriteByte('_')
			last = true
		}
	}
	return strings.Trim(b.String(), "_")
}
func StringPtr(s string) *string {
	s = Text(s)
	if s == "" || s == "-" {
		return nil
	}
	return &s
}
func Int(s string) *int {
	s = strings.TrimSpace(s)
	n, e := strconv.Atoi(nonDigits.ReplaceAllString(s, ""))
	if e != nil || s == "" || s == "-" {
		return nil
	}
	return &n
}
func Float(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" || s == "-" {
		return nil
	}
	n, e := strconv.ParseFloat(s, 64)
	if e != nil {
		return nil
	}
	return &n
}
func FIDE(s string) *string {
	d := nonDigits.ReplaceAllString(s, "")
	if d == "" || d == "0" {
		return nil
	}
	return &d
}
func Date(s string) *string {
	s = Text(s)
	for _, layout := range []string{"2006-01-02", "2006/01/02", "02.01.2006", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			v := t.Format("2006-01-02")
			return &v
		}
	}
	return StringPtr(s)
}
func TournamentID(href string) (string, bool) {
	m := tournamentPath.FindStringSubmatch(href)
	return valueMatch(m)
}
func StartNumber(href string) (string, bool) {
	u, e := url.Parse(href)
	if e != nil {
		return "", false
	}
	n := u.Query().Get("snr")
	if n == "" {
		return "", false
	}
	if _, e = strconv.ParseUint(n, 10, 64); e != nil {
		return "", false
	}
	return n, true
}
func Resolve(base, href string) string {
	b, e := url.Parse(base)
	if e != nil {
		return href
	}
	h, e := url.Parse(href)
	if e != nil {
		return href
	}
	return b.ResolveReference(h).String()
}
func PlayerKey(name string, fide *string, fed *string) string {
	if fide != nil {
		return "fide:" + *fide
	}
	f := ""
	if fed != nil {
		f = strings.ToLower(*fed)
	}
	n := strings.ToLower(Text(name))
	sum := sha256.Sum256([]byte(n + "\x00" + f))
	return "local:" + slug(n) + ":" + hex.EncodeToString(sum[:4])
}
func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
func Headers(row *goquery.Selection) []string {
	out := []string{}
	row.ChildrenFiltered("th,td").Each(func(_ int, c *goquery.Selection) { out = append(out, Text(c.Text())) })
	return out
}
func HeaderMap(headers []string, aliases map[string]string) map[string]int {
	m := map[string]int{}
	for i, h := range headers {
		k := Key(h)
		if a, ok := aliases[k]; ok {
			k = a
		}
		if k != "" {
			m[k] = i
		}
	}
	return m
}
func Cells(row *goquery.Selection) []*goquery.Selection {
	var out []*goquery.Selection
	row.ChildrenFiltered("td,th").Each(func(_ int, c *goquery.Selection) { out = append(out, c) })
	return out
}
func Cell(c []*goquery.Selection, i int) string {
	if i < 0 || i >= len(c) {
		return ""
	}
	return Text(c[i].Text())
}
func At(m map[string]int, k string) int {
	if i, ok := m[k]; ok {
		return i
	}
	return -1
}
func RoundFromHeading(s string) *int {
	m := roundHeading.FindStringSubmatch(s)
	if v, ok := valueMatch(m); ok {
		return Int(v)
	}
	return nil
}
func valueMatch(m []string) (string, bool) {
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}
func RequireDoc(html string) (*goquery.Document, error) {
	d, e := goquery.NewDocumentFromReader(strings.NewReader(html))
	if e != nil {
		return nil, fmt.Errorf("parse html: %w", e)
	}
	return d, nil
}
