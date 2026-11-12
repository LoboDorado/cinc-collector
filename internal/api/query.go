package api

import (
	"net/http"
	"strconv"

	"github.com/cinc-project/cinc-collector/internal/auth"
	"github.com/cinc-project/cinc-collector/internal/store"
)

// registerQueryRoutes wires the read API Cinc Console uses to display node
// converge history and compliance reports. Every route is scoped to the
// authenticated token's org.
func (a *API) registerQueryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v0/nodes", a.requireAuth(a.handleListNodes))
	mux.HandleFunc("GET /api/v0/nodes/{name}", a.requireAuth(a.handleGetNode))
	mux.HandleFunc("GET /api/v0/nodes/{name}/runs", a.requireAuth(a.handleListRuns))
	mux.HandleFunc("GET /api/v0/nodes/{name}/compliance", a.requireAuth(a.handleListReports))
	mux.HandleFunc("GET /api/v0/runs/{run_id}", a.requireAuth(a.handleGetRun))
	mux.HandleFunc("GET /api/v0/compliance/reports/{report_id}", a.requireAuth(a.handleGetReport))
	mux.HandleFunc("GET /api/v0/compliance/search", a.requireAuth(a.handleSearchReports))
}

func (a *API) handleListNodes(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	nodes, err := a.opts.Nodes.List(r.Context(), org, queryLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (a *API) handleGetNode(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	node, err := a.opts.Nodes.Get(r.Context(), org, r.PathValue("name"))
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get node")
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (a *API) handleListRuns(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	runs, err := a.opts.Runs.ListByNode(r.Context(), org, r.PathValue("name"), queryLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (a *API) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := a.opts.Runs.Get(r.Context(), r.PathValue("run_id"))
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *API) handleListReports(w http.ResponseWriter, r *http.Request) {
	org := auth.TokenOrg(r.Context())
	reports, err := a.opts.Reports.ListByNode(r.Context(), org, r.PathValue("name"), queryLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list compliance reports")
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

func (a *API) handleGetReport(w http.ResponseWriter, r *http.Request) {
	report, err := a.opts.Reports.Get(r.Context(), r.PathValue("report_id"))
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// handleSearchReports performs full-text search over compliance reports
// using SereneDB's native inverted index (see docs/specs). Query params:
// q (required), status (optional exact filter), limit (optional).
func (a *API) handleSearchReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: q")
		return
	}
	org := auth.TokenOrg(r.Context())
	status := r.URL.Query().Get("status")
	results, err := a.opts.Reports.Search(r.Context(), org, q, status, queryLimit(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func queryLimit(r *http.Request) int {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
