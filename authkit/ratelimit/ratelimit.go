// SPDX-License-Identifier: Apache-2.0

// Package ratelimit limits failed login attempts per client IP.
package ratelimit

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// DefaultLimit and DefaultWindow cap login attempts per client IP.
const (
	DefaultLimit  = 10
	DefaultWindow = time.Minute
)

// Config parameterizes the limiter.
type Config struct {
	// Limit caps failed attempts per client IP per Window. Zero applies DefaultLimit.
	Limit int
	// Window is the counting window. Zero applies DefaultWindow.
	Window time.Duration
	// TrustedProxies lists the CIDR ranges of reverse proxies permitted to
	// set X-Forwarded-For.
	TrustedProxies []string
}

// limitAndWindow returns the configured budget with defaults applied.
func (cfg Config) limitAndWindow() (int, time.Duration) {
	limit := cfg.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	window := cfg.Window
	if window == 0 {
		window = DefaultWindow
	}
	return limit, window
}

// Limiter counts login failures per key outside the HTTP middleware.
type Limiter struct {
	limiter *httprate.RateLimiter
	limit   int
	window  time.Duration
}

// NewLimiter returns a key based limiter enforcing cfg's budget.
func NewLimiter(cfg Config) *Limiter {
	_, window := cfg.limitAndWindow()
	return newLimiterUsing(cfg, httprate.NewLocalLimitCounter(window))
}

// newLimiterUsing returns a key based limiter tracking failures in counter.
func newLimiterUsing(cfg Config, counter httprate.LimitCounter) *Limiter {
	limit, window := cfg.limitAndWindow()
	return &Limiter{
		limiter: httprate.NewRateLimiter(limit, window, httprate.WithLimitCounter(counter)),
		limit:   limit,
		window:  window,
	}
}

// Check reports whether key may attempt a login, and the wait when it may not.
func (l *Limiter) Check(key string) (bool, time.Duration, error) {
	_, rate, err := l.limiter.Status(key)
	if err != nil {
		return false, 0, err
	}
	if int(math.Round(rate)) >= l.limit {
		return false, 2 * l.window, nil
	}
	return true, 0, nil
}

// RecordFailure counts one failed login for key.
func (l *Limiter) RecordFailure(key string) error {
	bucket := time.Now().UTC().Truncate(l.window)
	return l.limiter.Counter().IncrementBy(key, bucket, 1)
}

// Middleware returns middleware that limits failed login attempts per client IP.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	_, window := cfg.limitAndWindow()
	return middlewareUsing(cfg, httprate.NewLocalLimitCounter(window))
}

// middlewareUsing returns middleware that limits failed login attempts
// per client IP, tracking them in counter.
func middlewareUsing(cfg Config, counter httprate.LimitCounter) func(http.Handler) http.Handler {
	limiter := newLimiterUsing(cfg, counter)
	resolve := clientIPResolver(cfg.TrustedProxies)
	limitStage := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyByRemoteIP(r)
			allowed, retryAfter, err := limiter.Check(key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
				return
			}
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(recorder, r)
			if recorder.Status() == http.StatusUnauthorized {
				_ = limiter.RecordFailure(key)
			}
		})
	}
	return func(next http.Handler) http.Handler {
		return resolve(limitStage(next))
	}
}

// ResolveClientIP returns middleware recording the request's client IP under the trusted proxy rules.
func ResolveClientIP(trustedProxies []string) func(http.Handler) http.Handler {
	return clientIPResolver(trustedProxies)
}

// ClientIP returns the request's canonical client IP for rate limiting.
func ClientIP(r *http.Request) string {
	return keyByRemoteIP(r)
}

// clientIPResolver returns middleware that records a request's client IP for rate limiting.
func clientIPResolver(trustedProxies []string) func(http.Handler) http.Handler {
	if len(trustedProxies) == 0 {
		return middleware.ClientIPFromRemoteAddr
	}
	return middleware.ClientIPFromXFF(trustedProxies...)
}

// keyByRemoteIP returns a request's canonical client IP for rate limiting.
func keyByRemoteIP(r *http.Request) string {
	ip := middleware.GetClientIP(r.Context())
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip = host
	}
	return httprate.CanonicalizeIP(ip)
}

// ParseTrustedProxies parses a comma-separated list into trusted-proxy CIDR ranges.
func ParseTrustedProxies(raw string) ([]string, error) {
	var prefixes []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := netip.ParsePrefix(part); err != nil {
			return nil, fmt.Errorf("ratelimit: invalid CIDR %q: %w", part, err)
		}
		prefixes = append(prefixes, part)
	}
	return prefixes, nil
}

// writeError writes a JSON error response with the given status code and message.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, message)
}
