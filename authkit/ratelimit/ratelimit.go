// SPDX-License-Identifier: Apache-2.0

// Package ratelimit limits failed login attempts per client IP.
//
// The middleware counts only responses with status 401, holds successful
// logins free of charge, and fails closed when its counter errors. Wrap
// your login handler with Middleware and mount the result on your router.
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

// Middleware returns middleware that limits failed login attempts per client IP.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	_, window := cfg.limitAndWindow()
	return middlewareUsing(cfg, httprate.NewLocalLimitCounter(window))
}

// middlewareUsing returns middleware that limits failed login attempts
// per client IP, tracking them in counter.
func middlewareUsing(cfg Config, counter httprate.LimitCounter) func(http.Handler) http.Handler {
	limit, window := cfg.limitAndWindow()
	limiter := httprate.NewRateLimiter(limit, window,
		httprate.WithLimitCounter(counter),
	)
	resolve := clientIPResolver(cfg.TrustedProxies)
	limitStage := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyByRemoteIP(r)
			_, rate, err := limiter.Status(key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if int(math.Round(rate)) >= limit {
				w.Header().Set("Retry-After", strconv.Itoa(int((2 * window).Seconds())))
				writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
				return
			}
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(recorder, r)
			if recorder.Status() == http.StatusUnauthorized {
				bucket := time.Now().UTC().Truncate(window)
				_ = limiter.Counter().IncrementBy(key, bucket, 1)
			}
		})
	}
	return func(next http.Handler) http.Handler {
		return resolve(limitStage(next))
	}
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
