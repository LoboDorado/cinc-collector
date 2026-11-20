package api

import "net/http"

// instrument counts responses by status-code class
// (cinc_collector_http_requests_total{outcome=2xx|3xx|4xx|5xx}), mirroring
// cinc-server-ng's own metric naming. It wraps the whole handler, outside
// auth, so even rejected/unauthenticated requests are counted.
func (a *API) instrument(next http.Handler) http.Handler {
	if a.opts.Metrics == nil {
		return next
	}

	c2xx := a.opts.Metrics.Counter("cinc_collector_http_requests_total_2xx", "2xx responses")
	c3xx := a.opts.Metrics.Counter("cinc_collector_http_requests_total_3xx", "3xx responses")
	c4xx := a.opts.Metrics.Counter("cinc_collector_http_requests_total_4xx", "4xx responses")
	c5xx := a.opts.Metrics.Counter("cinc_collector_http_requests_total_5xx", "5xx responses")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		switch {
		case rec.status < 300:
			c2xx.Inc()
		case rec.status < 400:
			c3xx.Inc()
		case rec.status < 500:
			c4xx.Inc()
		default:
			c5xx.Inc()
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
