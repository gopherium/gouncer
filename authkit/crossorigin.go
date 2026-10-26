// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// CrossOriginGuard returns root middleware refusing cross-origin browser writes, logging to logger or slog.Default.
func CrossOriginGuard(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuseCrossOrigin(w, r, logger)
	}))
	return protection.Handler
}

// refuseCrossOrigin answers a cross-origin write with the shared 403 and logs it to logger.
func refuseCrossOrigin(w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	fetchSite := r.Header.Get("Sec-Fetch-Site")
	reason := "origin"
	if fetchSite != "" {
		reason = "fetch-site"
	}
	logger.WarnContext(r.Context(), "write refused", "reason", reason, "method", r.Method, "path", r.URL.EscapedPath(),
		"host", escapeLogControls(r.Host), "origin", escapeLogControls(r.Header.Get("Origin")),
		"fetch_site", escapeLogControls(fetchSite))
	w.Header().Set("Cache-Control", "no-store")
	RespondError(w, http.StatusForbidden, ErrorResponse{
		Message: "cross-origin request refused", Code: "request_cross_origin",
	})
}

// escapeLogControls escapes control characters before any log handler can write them.
func escapeLogControls(value string) string {
	if !strings.ContainsFunc(value, unicode.IsControl) {
		return value
	}
	var escaped strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			quoted := strconv.QuoteRune(r)
			escaped.WriteString(quoted[1 : len(quoted)-1])
		} else {
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}
