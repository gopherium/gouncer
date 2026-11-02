// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"log/slog"
	"net/http"
	"strconv"
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
	logger.WarnContext(r.Context(), "write refused", "reason", reason, "method", escapeForLog(r.Method),
		"path", r.URL.EscapedPath(), "host", escapeForLog(r.Host), "origin", escapeForLog(r.Header.Get("Origin")),
		"fetch_site", escapeForLog(fetchSite))
	w.Header().Set("Cache-Control", "no-store")
	RespondError(w, http.StatusForbidden, ErrorResponse{
		Message: "cross-origin request refused", Code: "request_cross_origin",
	})
}

// escapeForLog returns value as printable ASCII, in Go string escapes.
func escapeForLog(value string) string {
	quoted := strconv.QuoteToASCII(value)
	return quoted[1 : len(quoted)-1]
}
