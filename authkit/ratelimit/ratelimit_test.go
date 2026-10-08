// SPDX-License-Identifier: Apache-2.0

package ratelimit_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/gouncer/authkit/ratelimit"
)

const goodBody = `{"password":"correct"}`

const wrongBody = `{"password":"wrong"}`

// fakeLogin answers 200 for the good credential body and 401 otherwise.
func fakeLogin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) == goodBody {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
}

// newLimitedServer wraps the fake login handler with the limiter under cfg.
func newLimitedServer(cfg ratelimit.Config) http.Handler {
	return ratelimit.Middleware(cfg)(fakeLogin())
}

// loginVia posts body from the given connecting address, optionally with X-Forwarded-For.
func loginVia(t *testing.T, handler http.Handler, remoteAddr, forwardedFor, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// loginFrom posts body from the given connecting address.
func loginFrom(t *testing.T, handler http.Handler, ip, body string) *httptest.ResponseRecorder {
	t.Helper()
	return loginVia(t, handler, ip, "", body)
}

func TestMiddlewareBlocksRepeatedFailures(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{})

	if code := loginFrom(t, handler, "198.51.100.7:40000", wrongBody).Code; code == http.StatusTooManyRequests {
		t.Fatal("the first login attempt was rate limited")
	}

	var last *httptest.ResponseRecorder
	for range 50 {
		last = loginFrom(t, handler, "198.51.100.7:40000", wrongBody)
		if last.Code == http.StatusTooManyRequests {
			break
		}
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("repeated attempts from one IP were never rate limited, last status = %d", last.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(last.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body %q is not JSON: %v", last.Body.String(), err)
	}
	if body.Error == "" {
		t.Error("rate-limit response carries no JSON error message")
	}
	if got := last.Header().Get("Retry-After"); got != "120" {
		t.Errorf("Retry-After = %q, want %q", got, "120")
	}
	for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if value := last.Header().Get(header); value != "" {
			t.Errorf("rate-limit response leaks %s = %q", header, value)
		}
	}
}

func TestMiddlewareIsPerIP(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{})

	for range 50 {
		loginFrom(t, handler, "198.51.100.8:40000", wrongBody)
	}

	fresh := loginFrom(t, handler, "203.0.113.9:40000", goodBody)

	if fresh.Code != http.StatusOK {
		t.Fatalf("a login from an untouched IP got status %d, want %d", fresh.Code, http.StatusOK)
	}
}

func TestMiddlewareDoesNotCountSuccesses(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{})

	for i := range 30 {
		if code := loginFrom(t, handler, "198.51.100.40:5000", goodBody).Code; code != http.StatusOK {
			t.Fatalf("successful login %d got status %d, want %d", i+1, code, http.StatusOK)
		}
	}
}

func TestMiddlewareHonorsAConfiguredLimit(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{Limit: 2})

	first := loginFrom(t, handler, "198.51.100.50:4000", wrongBody)
	second := loginFrom(t, handler, "198.51.100.50:4000", wrongBody)
	third := loginFrom(t, handler, "198.51.100.50:4000", wrongBody)

	if first.Code != http.StatusUnauthorized || second.Code != http.StatusUnauthorized {
		t.Fatalf("statuses = %d, %d, want two %d failures inside the budget",
			first.Code, second.Code, http.StatusUnauthorized)
	}
	if third.Code != http.StatusTooManyRequests {
		t.Errorf("third attempt status = %d, want %d under a limit of 2", third.Code, http.StatusTooManyRequests)
	}
}

func TestMiddlewareKeysForwardedClientBehindTrustedProxy(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{TrustedProxies: []string{"192.0.2.0/24"}})
	const proxy = "192.0.2.1:9000"

	var blocked bool
	for range 50 {
		if loginVia(t, handler, proxy, "203.0.113.10", wrongBody).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("one forwarded client was never rate limited")
	}

	other := loginVia(t, handler, proxy, "203.0.113.20", goodBody)
	if other.Code == http.StatusTooManyRequests {
		t.Fatal("a different forwarded client was locked out by another client's attempts")
	}
	if other.Code != http.StatusOK {
		t.Fatalf("the second forwarded client got status %d, want %d", other.Code, http.StatusOK)
	}
}

func TestMiddlewareResistsForwardedForSpoofingBehindProxy(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{TrustedProxies: []string{"10.0.0.0/8"}})
	const proxy = "10.0.0.2:5000"
	const realClientAppendedByProxy = "203.0.113.50"

	var blocked bool
	for i := range 50 {
		spoofedHead := fmt.Sprintf("%d.%d.%d.%d", i%250+1, i%100+1, i%50+1, i%200+1)
		forwardedFor := spoofedHead + ", " + realClientAppendedByProxy
		if loginVia(t, handler, proxy, forwardedFor, wrongBody).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("rotating a spoofed X-Forwarded-For head bypassed the per-client rate limit")
	}
}

func TestMiddlewareFallsBackToRemoteAddrWithoutForwardedFor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		blockedAddr string
		freshAddr   string
	}{
		"host and port": {blockedAddr: "192.0.2.10:4000", freshAddr: "192.0.2.20:4000"},
		"bare ip":       {blockedAddr: "192.0.2.30", freshAddr: "192.0.2.40"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler := newLimitedServer(ratelimit.Config{TrustedProxies: []string{"192.0.2.0/24"}})

			var blocked bool
			for range 50 {
				if loginFrom(t, handler, tc.blockedAddr, wrongBody).Code == http.StatusTooManyRequests {
					blocked = true
					break
				}
			}
			if !blocked {
				t.Fatal("attempts without X-Forwarded-For were never rate limited by connecting address")
			}

			fresh := loginFrom(t, handler, tc.freshAddr, goodBody)
			if fresh.Code != http.StatusOK {
				t.Fatalf("a login from a different connecting address got status %d, want %d", fresh.Code, http.StatusOK)
			}
		})
	}
}

func TestMiddlewareIgnoresForwardedForWithoutTrustedProxy(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{})
	const proxy = "198.51.100.30:7000"

	var blocked bool
	for i := range 50 {
		spoofed := fmt.Sprintf("203.0.113.%d", i%200+1)
		if loginVia(t, handler, proxy, spoofed, wrongBody).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("rotating X-Forwarded-For bypassed the rate limit with no trusted proxy configured")
	}
}

// clientIPOf resolves one request through ResolveClientIP and answers the key ClientIP reads back.
func clientIPOf(t *testing.T, trustedProxies []string, remoteAddr string, forwardedFor ...string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = remoteAddr
	for _, value := range forwardedFor {
		request.Header.Add("X-Forwarded-For", value)
	}
	var key string
	ratelimit.ResolveClientIP(trustedProxies)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		key = ratelimit.ClientIP(r)
	})).ServeHTTP(httptest.NewRecorder(), request)
	return key
}

func TestClientIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	key := clientIPOf(t, []string{"10.42.0.0/16"}, "203.0.113.9:40000", "198.51.100.1")

	if key != "203.0.113.9" {
		t.Errorf("ClientIP() = %q, want the untrusted peer %q, never its forged header", key, "203.0.113.9")
	}
}

func TestMiddlewareLimitsUntrustedPeerRotatingForwardedHeader(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{TrustedProxies: []string{"10.42.0.0/16"}})
	const untrustedPeer = "203.0.113.9:40000"

	var blocked bool
	for i := range 50 {
		forged := fmt.Sprintf("198.51.100.%d", i+1)
		if loginVia(t, handler, untrustedPeer, forged, wrongBody).Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("an untrusted peer rotating X-Forwarded-For was never rate limited")
	}
}

func TestClientIPResolutionTable(t *testing.T) {
	t.Parallel()

	trusted := []string{"10.42.0.0/16"}
	const (
		untrustedPeer     = "203.0.113.9:40000"
		trustedPeer       = "10.42.0.7:40000"
		mappedTrustedPeer = "[::ffff:10.42.0.7]:40000"
	)
	tests := map[string]struct {
		remoteAddr   string
		forwardedFor []string
		want         string
	}{
		"an untrusted peer forging one entry":    {untrustedPeer, []string{"198.51.100.1"}, "203.0.113.9"},
		"an untrusted peer forging past garbage": {untrustedPeer, []string{"garbage, 198.51.100.2"}, "203.0.113.9"},
		"an untrusted peer sending two headers":  {untrustedPeer, []string{"198.51.100.3", "198.51.100.4"}, "203.0.113.9"},
		"an untrusted peer forging a mapped IP":  {untrustedPeer, []string{"::ffff:198.51.100.5"}, "203.0.113.9"},
		"an untrusted bare peer forging":         {"203.0.113.9", []string{"198.51.100.6"}, "203.0.113.9"},
		"an untrusted IPv6 peer forging":         {"[2001:db8:1:2::9]:40000", []string{"198.51.100.7"}, "2001:db8:1:2::"},
		"an untrusted zoned peer forging":        {"[fe80::1%eth0]:40000", []string{"198.51.100.8"}, "fe80::1%eth0"},
		"an unparseable peer forging":            {"@", []string{"198.51.100.14"}, "@"},
		"an untrusted peer with no header":       {untrustedPeer, nil, "203.0.113.9"},
		"a trusted peer appending the client":    {trustedPeer, []string{"198.51.100.9"}, "198.51.100.9"},
		"a trusted peer behind a forged head":    {trustedPeer, []string{"198.51.100.10, 198.51.100.11"}, "198.51.100.11"},
		"a mapped trusted peer with a client":    {mappedTrustedPeer, []string{"198.51.100.12"}, "198.51.100.12"},
		"a trusted peer with no header":          {trustedPeer, nil, "10.42.0.7"},
		"a trusted peer forwarding only proxies": {trustedPeer, []string{"10.42.0.8"}, "10.42.0.7"},
		"a trusted peer with garbage rightmost":  {trustedPeer, []string{"198.51.100.13, garbage"}, "10.42.0.7"},
		"a mapped trusted peer with no header":   {mappedTrustedPeer, nil, "10.42.0.7"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			key := clientIPOf(t, trusted, tc.remoteAddr, tc.forwardedFor...)

			if key != tc.want {
				t.Errorf("ClientIP() = %q, want %q", key, tc.want)
			}
		})
	}
}

func TestMiddlewareHandlesAddressWithoutPort(t *testing.T) {
	t.Parallel()

	handler := newLimitedServer(ratelimit.Config{})

	recorder := loginFrom(t, handler, "198.51.100.10", goodBody)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for a RemoteAddr without a port", recorder.Code, http.StatusOK)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw     string
		want    []string
		wantErr bool
	}{
		"empty":            {raw: "", want: nil},
		"whitespace only":  {raw: "  ,  ", want: nil},
		"single cidr":      {raw: "10.0.0.0/8", want: []string{"10.0.0.0/8"}},
		"trims and splits": {raw: " 10.0.0.0/8 , 192.168.0.0/16 ", want: []string{"10.0.0.0/8", "192.168.0.0/16"}},
		"ipv6 cidr":        {raw: "::1/128", want: []string{"::1/128"}},
		"invalid cidr":     {raw: "10.0.0.0/8,nonsense", wantErr: true},
		"bare ip rejected": {raw: "10.0.0.1", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ratelimit.ParseTrustedProxies(tc.raw)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTrustedProxies(%q) error = nil, want an error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTrustedProxies(%q) error = %v, want nil", tc.raw, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseTrustedProxies(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
