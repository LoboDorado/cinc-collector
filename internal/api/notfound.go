package api

import "net/http"

// withJSONErrors wraps mux so that unrouted 404s and 405s (which
// http.ServeMux would otherwise render as a plain-text body) come back in
// the same {"error": [...]} JSON shape as every other response.
func withJSONErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern == "" {
			// No route matched at all: is it a 404, or a 405 for a known
			// path under a different method? http.ServeMux doesn't expose
			// that distinction directly, so just report 404; handlers that
			// care about method mismatches on their own path register
			// explicit method+path patterns, which is the normal case here.
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
