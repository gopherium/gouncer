// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"bytes"
	"cmp"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/gopherium/gouncer/authkit"
)

// siteHost is the address the guarded site answers on in these tests.
const siteHost = "app.example.com"

// proxyPeer is the address a reverse proxy connects from in these tests.
const proxyPeer = "10.0.0.2:1234"

// browserCall describes one request as it reaches the guard.
type browserCall struct {
	method         string
	path           string
	host           string
	remote         string
	fetchSite      string
	origin         string
	forwardedHost  string
	forwardedProto string
	tls            bool
}

// request builds the HTTP request call describes.
func (c browserCall) request() *http.Request {
	request := httptest.NewRequest(cmp.Or(c.method, http.MethodPost), cmp.Or(c.path, "/api/graphql"), nil)
	request.Host = cmp.Or(c.host, siteHost)
	request.RemoteAddr = cmp.Or(c.remote, request.RemoteAddr)
	headers := map[string]string{
		"Sec-Fetch-Site": c.fetchSite, "Origin": c.origin,
		"X-Forwarded-Host": c.forwardedHost, "X-Forwarded-Proto": c.forwardedProto,
	}
	for name, value := range headers {
		if value != "" {
			request.Header.Set(name, value)
		}
	}
	if c.tls {
		request.TLS = &tls.ConnectionState{}
	}
	return request
}

// guarded wraps a write counter in the guard, logging into the returned buffer.
func guarded() (http.Handler, *int, *bytes.Buffer) {
	var logged bytes.Buffer
	writes := 0
	return authkit.CrossOriginGuard(slog.New(slog.NewJSONHandler(&logged, nil)))(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			writes++
			w.WriteHeader(http.StatusNoContent)
		})), &writes, &logged
}

// loggedLine decodes the single JSON line the guard logged.
func loggedLine(t *testing.T, logged *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(logged.Bytes(), &line); err != nil {
		t.Fatalf("decoding the log line %q: %v, want one JSON line", logged.String(), err)
	}
	return line
}

// assertRefusedWriteLog checks safe JSON output and exactly the expected refusal fields, ignoring the timestamp.
func assertRefusedWriteLog(t *testing.T, logged *bytes.Buffer, fields map[string]any) {
	t.Helper()
	if value := strings.TrimSuffix(logged.String(), "\n"); strings.ContainsFunc(value, unicode.IsControl) {
		t.Errorf("log = %q, want no raw control characters", logged.String())
	}
	line := loggedLine(t, logged)
	delete(line, "time")
	want := map[string]any{"level": "WARN", "msg": "write refused", "method": http.MethodPost, "path": "/api/graphql"}
	maps.Copy(want, fields)
	if !maps.Equal(line, want) {
		t.Errorf("log = %#v, want exactly the fields %#v", line, want)
	}
}

// controlCharacters returns all C0 and C1 control characters, including DEL.
func controlCharacters() string {
	var controls strings.Builder
	for control := rune(0); control <= 0x1f; control++ {
		controls.WriteRune(control)
	}
	for control := rune(0x7f); control <= 0x9f; control++ {
		controls.WriteRune(control)
	}
	return controls.String()
}

// assertEscapedLogAttr checks request headers before serialization and records which fields the handler received.
func assertEscapedLogAttr(t *testing.T, seen map[string]bool, attr slog.Attr) slog.Attr {
	t.Helper()
	switch attr.Key {
	case "host", "origin", "fetch_site":
		seen[attr.Key] = true
		if value := attr.Value.String(); strings.ContainsFunc(value, unicode.IsControl) {
			t.Errorf("handler received %s = %q, want controls escaped", attr.Key, value)
		}
	}
	return attr
}

// TestCrossOriginGuardRefusesBrowserWritesFromAnotherOrigin verifies that cross-origin writes are logged and blocked.
func TestCrossOriginGuardRefusesBrowserWritesFromAnotherOrigin(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		call   browserCall
		reason string
	}{
		{name: "cross site", reason: "fetch-site",
			call: browserCall{fetchSite: "cross-site", origin: "https://example.org"}},
		{name: "sibling subdomain", reason: "fetch-site",
			call: browserCall{fetchSite: "same-site", origin: "https://other.example.com"}},
		{name: "cross site put", reason: "fetch-site",
			call: browserCall{method: http.MethodPut, fetchSite: "cross-site", origin: "https://example.org"}},
		{name: "cross site naming the site as its origin", reason: "fetch-site",
			call: browserCall{fetchSite: "cross-site", origin: "https://app.example.com"}},
		{name: "cross site with no origin", reason: "fetch-site", call: browserCall{fetchSite: "cross-site"}},
		{name: "origin only from another host", reason: "origin",
			call: browserCall{origin: "https://example.org", tls: true}},
		{name: "origin only from another port", reason: "origin",
			call: browserCall{origin: "https://app.example.com:8443", tls: true}},
		{name: "opaque origin", reason: "origin", call: browserCall{origin: "null"}},
		{name: "origin only through a proxy forwarding the site's host", reason: "origin",
			call: browserCall{
				host: "app:8080", remote: proxyPeer, origin: "https://app.example.com",
				forwardedHost: siteHost, forwardedProto: "https",
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, writes, logged := guarded()
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, tc.call.request())

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if *writes != 0 {
				t.Errorf("writes = %d, want none to reach the handler", *writes)
			}
			if got := loggedLine(t, logged)["reason"]; got != tc.reason {
				t.Errorf("logged reason = %v, want %q", got, tc.reason)
			}
		})
	}
}

// TestCrossOriginGuardLetsTheSiteAndOtherServersWrite verifies allowed requests pass through without logging.
func TestCrossOriginGuardLetsTheSiteAndOtherServersWrite(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call browserCall
	}{
		{name: "same origin browser write", call: browserCall{fetchSite: "same-origin", origin: "https://app.example.com"}},
		{name: "address typed by the user", call: browserCall{fetchSite: "none"}},
		{name: "origin only over https", call: browserCall{origin: "https://app.example.com", tls: true}},
		{name: "origin only over plain http", call: browserCall{origin: "http://app.example.com"}},
		{name: "origin only from an https page over plain http", call: browserCall{origin: "https://app.example.com"}},
		{name: "origin only from a plain http page over https",
			call: browserCall{origin: "http://app.example.com", tls: true}},
		{name: "origin only on an explicit port",
			call: browserCall{host: "app.example.com:8443", origin: "https://app.example.com:8443", tls: true}},
		{name: "origin only on an IPv6 host", call: browserCall{host: "[::1]:8080", origin: "http://[::1]:8080"}},
		{name: "origin only through a proxy forwarding another host", call: browserCall{
			remote: proxyPeer, origin: "https://app.example.com", forwardedHost: "example.org", forwardedProto: "http",
		}},
		{name: "server write without browser headers", call: browserCall{}},
		{name: "server write through a proxy", call: browserCall{
			host: "app:8080", remote: proxyPeer, forwardedHost: siteHost, forwardedProto: "https",
		}},
		{name: "cross site read", call: browserCall{
			method: http.MethodGet, fetchSite: "cross-site", origin: "https://example.org",
		}},
		{name: "origin only head", call: browserCall{method: http.MethodHead, origin: "https://example.org"}},
		{name: "origin only options", call: browserCall{method: http.MethodOptions, origin: "https://example.org"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, writes, logged := guarded()
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, tc.call.request())

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if *writes != 1 {
				t.Errorf("writes = %d, want the request to reach the handler once", *writes)
			}
			if logged.Len() != 0 {
				t.Errorf("log = %q, want nothing logged for a request let through", logged.String())
			}
		})
	}
}

// TestCrossOriginGuardAnswersAnUncachedJSONError verifies the status, JSON error, and cache headers of refused writes.
func TestCrossOriginGuardAnswersAnUncachedJSONError(t *testing.T) {
	t.Parallel()

	for name, call := range map[string]browserCall{
		"fetch-site": {fetchSite: "cross-site", origin: "https://example.org"},
		"origin":     {origin: "https://example.org"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _, _ := guarded()
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, call.request())

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if got := recorder.Body.String(); got != `{"error":"cross-origin request refused","code":"request_cross_origin"}` {
				t.Errorf("body = %s, want the message and the code", got)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

// TestCrossOriginGuardLogsTheRefusedWrite verifies the refusal fields for ordinary requests.
func TestCrossOriginGuardLogsTheRefusedWrite(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call browserCall
		want map[string]any
	}{
		{
			name: "fetch-site",
			call: browserCall{
				host: "app:8080", remote: proxyPeer, fetchSite: "same-site", origin: "https://other.example.com",
				forwardedHost: siteHost, forwardedProto: "https",
			},
			want: map[string]any{
				"reason": "fetch-site", "host": "app:8080", "origin": "https://other.example.com", "fetch_site": "same-site",
			},
		},
		{
			name: "origin",
			call: browserCall{method: http.MethodDelete, origin: "https://example.org"},
			want: map[string]any{
				"reason": "origin", "method": http.MethodDelete, "host": siteHost, "origin": "https://example.org",
				"fetch_site": "",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, _, logged := guarded()
			handler.ServeHTTP(httptest.NewRecorder(), tc.call.request())
			assertRefusedWriteLog(t, logged, tc.want)
		})
	}
}

// TestCrossOriginGuardLogsEscapedRefusalFields verifies path encoding and safe header values in JSON logs.
func TestCrossOriginGuardLogsEscapedRefusalFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call browserCall
		want map[string]any
	}{
		{
			name: "encoded control in path",
			call: browserCall{path: "/%C2%9B31m", origin: "https://example.org"},
			want: map[string]any{"reason": "origin", "host": siteHost, "origin": "https://example.org",
				"path": "/%C2%9B31m", "fetch_site": ""},
		},
		{
			name: "preserved path escaping",
			call: browserCall{path: "/api%2fgraphql", origin: "https://example.org"},
			want: map[string]any{"reason": "origin", "host": siteHost, "origin": "https://example.org",
				"path": "/api%2fgraphql", "fetch_site": ""},
		},
		{
			name: "control in origin",
			call: browserCall{origin: "https://example.org/\u009b31m"},
			want: map[string]any{"reason": "origin", "host": siteHost, "origin": `https://example.org/\u009b31m`,
				"fetch_site": ""},
		},
		{
			name: "control in fetch site",
			call: browserCall{fetchSite: "same-site\u009b31m", origin: "https://example.org"},
			want: map[string]any{"reason": "fetch-site", "host": siteHost, "origin": "https://example.org",
				"fetch_site": `same-site\u009b31m`},
		},
		{
			name: "control in host",
			call: browserCall{host: "app.example.com\u009b31m", fetchSite: "cross-site", origin: "https://example.org"},
			want: map[string]any{"reason": "fetch-site", "host": `app.example.com\u009b31m`, "origin": "https://example.org",
				"fetch_site": "cross-site"},
		},
		{
			name: "other controls in host and origin",
			call: browserCall{host: "app\x00\t\n\r\x1b\x7f\u0080\u009f", fetchSite: "cross-site",
				origin: "https://example.org/\x00\t\n\r\x1b\x7f\u0080\u009f"},
			want: map[string]any{"reason": "fetch-site", "host": `app\x00\t\n\r\x1b\x7f\u0080\u009f`,
				"origin": `https://example.org/\x00\t\n\r\x1b\x7f\u0080\u009f`, "fetch_site": "cross-site"},
		},
		{
			name: "printable characters are unchanged",
			call: browserCall{host: "例子.example", fetchSite: "cross-site", origin: `https://例子.example/"\literal`},
			want: map[string]any{"reason": "fetch-site", "host": "例子.example", "origin": `https://例子.example/"\literal`,
				"fetch_site": "cross-site"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, _, logged := guarded()
			handler.ServeHTTP(httptest.NewRecorder(), tc.call.request())
			assertRefusedWriteLog(t, logged, tc.want)
		})
	}
}

// TestCrossOriginGuardEscapesControlsBeforeTheLogHandler verifies escaping before the handler formats the log.
func TestCrossOriginGuardEscapesControlsBeforeTheLogHandler(t *testing.T) {
	t.Parallel()

	controls := controlCharacters()
	var logged bytes.Buffer
	seen := make(map[string]bool)
	logger := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			return assertEscapedLogAttr(t, seen, attr)
		},
	}))
	handler := authkit.CrossOriginGuard(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("refused request reached the handler")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, browserCall{
		host: "app.example.com" + controls, origin: "https://example.org/" + controls,
		fetchSite: "same-site" + controls,
	}.request())
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if len(seen) != 3 {
		t.Errorf("logged fields = %v, want host, origin and fetch_site", seen)
	}
}

// TestCrossOriginGuardLogsToTheDefaultLoggerWhenGivenNone verifies the fallback to slog.Default when the logger is nil.
func TestCrossOriginGuardLogsToTheDefaultLoggerWhenGivenNone(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var logged bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	handler := authkit.CrossOriginGuard(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), browserCall{origin: "https://example.org"}.request())

	if reason := loggedLine(t, &logged)["reason"]; reason != "origin" {
		t.Errorf("logged reason = %v, want origin on the default logger", reason)
	}
}
