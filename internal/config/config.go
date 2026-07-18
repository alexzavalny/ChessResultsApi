package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr              string
	DatabasePath            string
	DefaultFederation       string
	UpstreamLanguage        string
	UpstreamBaseURL         string
	UpstreamTimeout         time.Duration
	UpstreamMaxConcurrency  int
	UpstreamMinInterval     time.Duration
	UpstreamMaxBodyBytes    int64
	SearchTTL               time.Duration
	ActiveStandingsTTL      time.Duration
	CompletedTTL            time.Duration
	MaxStaleActive          time.Duration
	MaxStaleCompleted       time.Duration
	MaxSearchIntervalDays   int
	MaxSearchResults        int
	CacheControlMaxAge      int
	LogLevel                string
	LogFormat               string
	APIKeys                 map[string]struct{}
	AllowInsecurePublicBind bool
}

func Load() (Config, error) {
	c := Config{
		ListenAddr: env("LISTEN_ADDR", "127.0.0.1:8080"), DatabasePath: env("DATABASE_PATH", "./data/api.sqlite"),
		DefaultFederation: strings.ToUpper(env("DEFAULT_COUNTRY", env("DEFAULT_FEDERATION", "LAT"))), UpstreamLanguage: env("UPSTREAM_LANGUAGE", "1"),
		UpstreamBaseURL: env("UPSTREAM_BASE_URL", "https://s2.chess-results.com"), LogLevel: env("LOG_LEVEL", "info"), LogFormat: env("LOG_FORMAT", "json"),
		MaxSearchIntervalDays: envInt("MAX_SEARCH_INTERVAL_DAYS", 366), MaxSearchResults: envInt("MAX_SEARCH_RESULTS", 500),
		CacheControlMaxAge: envInt("CACHE_CONTROL_MAX_AGE", 30), APIKeys: parseKeys(os.Getenv("API_KEYS")),
		AllowInsecurePublicBind: envBool("ALLOW_INSECURE_PUBLIC_BIND", false),
	}
	var err error
	for _, x := range []struct {
		dst      *time.Duration
		key, def string
	}{
		{&c.UpstreamTimeout, "UPSTREAM_TIMEOUT", "15s"}, {&c.UpstreamMinInterval, "UPSTREAM_MIN_INTERVAL", "750ms"},
		{&c.SearchTTL, "CACHE_SEARCH_TTL", "10m"}, {&c.ActiveStandingsTTL, "CACHE_ACTIVE_STANDINGS_TTL", "2m"},
		{&c.CompletedTTL, "CACHE_COMPLETED_TTL", "24h"}, {&c.MaxStaleActive, "MAX_STALE_ACTIVE", "24h"},
		{&c.MaxStaleCompleted, "MAX_STALE_COMPLETED", "720h"},
	} {
		*x.dst, err = time.ParseDuration(env(x.key, x.def))
		if err != nil {
			return c, fmt.Errorf("%s: %w", x.key, err)
		}
		if *x.dst <= 0 {
			return c, fmt.Errorf("%s must be positive", x.key)
		}
	}
	c.UpstreamMaxConcurrency = envInt("UPSTREAM_MAX_CONCURRENCY", 2)
	c.UpstreamMaxBodyBytes = envInt64("UPSTREAM_MAX_BODY_BYTES", 5242880)
	if c.UpstreamMaxConcurrency <= 0 || c.UpstreamMaxBodyBytes <= 0 || c.MaxSearchIntervalDays <= 0 || c.MaxSearchResults <= 0 {
		return c, errors.New("safety limits must be positive")
	}
	if c.DefaultFederation != "-" && (len(c.DefaultFederation) != 3 || !letters(c.DefaultFederation)) {
		return c, errors.New("DEFAULT_COUNTRY must be a three-letter code or -")
	}
	host, _, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return c, fmt.Errorf("LISTEN_ADDR: %w", err)
	}
	if !isPrivateHost(host) && len(c.APIKeys) == 0 && !c.AllowInsecurePublicBind {
		return c, errors.New("public bind requires API_KEYS or ALLOW_INSECURE_PUBLIC_BIND=true")
	}
	if err := os.MkdirAll(filepath.Dir(c.DatabasePath), 0o750); err != nil {
		return c, fmt.Errorf("database directory: %w", err)
	}
	return c, nil
}

func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	v, e := strconv.Atoi(env(k, strconv.Itoa(d)))
	if e != nil {
		return 0
	}
	return v
}
func envInt64(k string, d int64) int64 {
	v, e := strconv.ParseInt(env(k, strconv.FormatInt(d, 10)), 10, 64)
	if e != nil {
		return 0
	}
	return v
}
func envBool(k string, d bool) bool {
	v, e := strconv.ParseBool(env(k, strconv.FormatBool(d)))
	if e != nil {
		return d
	}
	return v
}
func parseKeys(s string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, k := range strings.Split(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			m[k] = struct{}{}
		}
	}
	return m
}
func letters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
func isPrivateHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
