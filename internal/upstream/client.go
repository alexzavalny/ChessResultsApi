package upstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	searchparser "github.com/alex/easy-chess-results-api/internal/parser/search"
)

type HTTPError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("upstream status %d", e.Status) }

type Client struct {
	base        *url.URL
	language    string
	maxBody     int64
	http        *http.Client
	sem         chan struct{}
	minInterval time.Duration
	mu          sync.Mutex
	last        map[string]time.Time
}

func New(baseURL, language string, timeout time.Duration, concurrency int, minInterval time.Duration, maxBody int64) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid upstream base URL")
	}
	jar, _ := cookiejar.New(nil)
	c := &Client{base: base, language: language, maxBody: maxBody, sem: make(chan struct{}, concurrency), minInterval: minInterval, last: map[string]time.Time{}}
	tr := &http.Transport{DialContext: c.dialContext, ForceAttemptHTTP2: true, MaxIdleConns: 8, MaxIdleConnsPerHost: 4, ResponseHeaderTimeout: 8 * time.Second, TLSHandshakeTimeout: 5 * time.Second}
	hc := &http.Client{Timeout: timeout, Transport: tr, Jar: jar}
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !c.allowedHost(req.URL.Hostname()) {
			return errors.New("redirect target not allowed")
		}
		return nil
	}
	c.http = hc
	return c, nil
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	baseHost := strings.ToLower(c.base.Hostname())
	if baseHost == "localhost" || net.ParseIP(baseHost) != nil {
		return dialer.DialContext(ctx, network, address)
	}
	if !c.allowedHost(host) {
		return nil, errors.New("upstream host not allowed")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve upstream host: %w", err)
	}
	for _, resolved := range addresses {
		ip := resolved.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return nil, errors.New("upstream host resolved to a non-public address")
		}
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func (c *Client) allowedHost(h string) bool {
	h = strings.ToLower(h)
	base := strings.ToLower(c.base.Hostname())
	if h == base {
		return true
	}
	if base == "localhost" || net.ParseIP(base) != nil {
		return h == base
	}
	if h == "chess-results.com" {
		return true
	}
	if !strings.HasSuffix(h, ".chess-results.com") {
		return false
	}
	prefix := strings.TrimSuffix(h, ".chess-results.com")
	if !strings.HasPrefix(prefix, "s") || len(prefix) < 2 {
		return false
	}
	_, e := strconv.Atoi(prefix[1:])
	return e == nil
}
func (c *Client) url(path string, q url.Values) string {
	u := *c.base
	u.Path = path
	q.Set("lan", c.language)
	u.RawQuery = q.Encode()
	return u.String()
}
func (c *Client) TournamentURL(id string, art int, round, start string) string {
	q := url.Values{"art": {strconv.Itoa(art)}}
	if round != "" {
		q.Set("rd", round)
	}
	if start != "" {
		q.Set("snr", start)
	}
	return c.url("/tnr"+id+".aspx", q)
}

func (c *Client) GetTournament(ctx context.Context, id string, art int, round, start string) (string, string, error) {
	return c.get(ctx, c.TournamentURL(id, art, round, start))
}
func (c *Client) Search(ctx context.Context, federation, from, to string) (string, string, error) {
	searchURL := c.url("/turniersuche.aspx", url.Values{})
	first, _, err := c.get(ctx, searchURL)
	if err != nil {
		return "", "", err
	}
	fields, err := searchparser.HiddenFields(first)
	if err != nil {
		return "", "", err
	}
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	form.Set("ctl00$P1$combo_art", "5")
	form.Set("ctl00$P1$combo_sort", "3")
	form.Set("ctl00$P1$combo_land", federation)
	form.Set("ctl00$P1$combo_bedenkzeit", "0")
	form.Set("ctl00$P1$combo_anzahl_zeilen", "0")
	if from != "" {
		form.Set("ctl00$P1$txt_von_tag", from)
	}
	if to != "" {
		form.Set("ctl00$P1$txt_bis_tag", to)
	}
	form.Set("ctl00$P1$cb_suchen", "Search")
	body, final, err := c.do(ctx, http.MethodPost, searchURL, []byte(form.Encode()), "application/x-www-form-urlencoded")
	return body, final, err
}
func (c *Client) get(ctx context.Context, u string) (string, string, error) {
	return c.do(ctx, http.MethodGet, u, nil, "")
}
func (c *Client) do(ctx context.Context, method, u string, body []byte, contentType string) (string, string, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(time.Duration(200*(1<<attempt))*time.Millisecond + time.Duration(rand.IntN(150))*time.Millisecond):
			}
		}
		text, final, status, retry, err := c.once(ctx, method, u, body, contentType)
		if err == nil {
			return text, final, nil
		}
		last = err
		if status != 0 && status != 429 && status != 502 && status != 503 && status != 504 {
			return "", "", err
		}
		if retry > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(retry):
			}
		}
	}
	return "", "", last
}
func (c *Client) once(ctx context.Context, method, u string, body []byte, contentType string) (string, string, int, time.Duration, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return "", "", 0, 0, ctx.Err()
	}
	parsed, _ := url.Parse(u)
	c.throttle(ctx, parsed.Hostname())
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return "", "", 0, 0, err
	}
	req.Header.Set("User-Agent", "EasyChessResultsAPI/1.0 (+https://github.com/alex/easy-chess-results-api)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", 0, 0, err
	}
	defer resp.Body.Close()
	retry := parseRetry(resp.Header.Get("Retry-After"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", resp.StatusCode, retry, &HTTPError{Status: resp.StatusCode, RetryAfter: retry}
	}
	limited := io.LimitReader(resp.Body, c.maxBody+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return "", "", resp.StatusCode, 0, err
	}
	if int64(len(b)) > c.maxBody {
		return "", "", resp.StatusCode, 0, errors.New("upstream response exceeds size limit")
	}
	trim := strings.TrimSpace(string(b))
	if !strings.HasPrefix(strings.ToLower(trim), "<!doctype html") && !strings.HasPrefix(strings.ToLower(trim), "<html") {
		return "", "", resp.StatusCode, 0, errors.New("upstream response is not HTML")
	}
	return string(b), resp.Request.URL.String(), resp.StatusCode, 0, nil
}
func (c *Client) throttle(ctx context.Context, host string) {
	c.mu.Lock()
	wait := c.minInterval - time.Since(c.last[host])
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
	c.mu.Lock()
	c.last[host] = time.Now()
	c.mu.Unlock()
}
func parseRetry(s string) time.Duration {
	if n, e := strconv.Atoi(strings.TrimSpace(s)); e == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, e := http.ParseTime(s); e == nil && time.Until(t) > 0 {
		return time.Until(t)
	}
	return 0
}
