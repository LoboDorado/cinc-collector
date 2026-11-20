package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cinc-project/cinc-collector/internal/auth"
	"github.com/cinc-project/cinc-collector/internal/automate"
	"github.com/cinc-project/cinc-collector/internal/store"
)

func (a *API) registerIngestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /data-collector/v0", a.requireAuth(a.handleIngest))
}

// requireAuth wraps next with bearer-token auth, or runs it unauthenticated
// if no token store was configured (e.g. in tests exercising a single
// handler directly).
func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	if a.auth == nil {
		return next
	}
	return a.auth.Require(next)
}

// handleIngest accepts a Chef Automate data-collector message: a
// chef-client run_start or run_converge message, or an InSpec compliance
// report. This is the one endpoint cinc-server-ng's data-collector reverse
// proxy and InSpec's "automate" reporter plugin both POST to.
func (a *API) handleIngest(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	kind, err := automate.Detect(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch kind {
	case automate.KindRunStart:
		err = a.ingestRunStart(r, raw)
	case automate.KindRunConverge:
		err = a.ingestRunConverge(r, raw)
	case automate.KindComplianceReport:
		err = a.ingestComplianceReport(r, raw)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store message")
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}

func (a *API) ingestRunStart(r *http.Request, raw []byte) error {
	var msg automate.RunStartMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}

	org := msg.OrganizationName
	if org == "" {
		org = auth.TokenOrg(r.Context())
	}
	runID := strOrEmpty(msg.RunID)
	if runID == "" {
		runID = strOrEmpty(msg.ID)
	}
	if runID == "" {
		return nil // nothing to key a row by; silently accept, matching a dumb-pipe ingest's tolerance
	}

	return a.opts.Runs.Upsert(r.Context(), store.ConvergeRun{
		RunID:      runID,
		Org:        org,
		NodeName:   msg.NodeName,
		Status:     "started",
		StartTime:  parseTimeOr(msg.StartTime, time.Now()),
		RawMessage: raw,
	})
}

func (a *API) ingestRunConverge(r *http.Request, raw []byte) error {
	var msg automate.RunConvergeMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}

	org := msg.OrganizationName
	if org == "" {
		org = auth.TokenOrg(r.Context())
	}
	runID := strOrEmpty(msg.RunID)
	if runID == "" {
		runID = strOrEmpty(msg.ID)
	}
	if runID == "" {
		return nil
	}

	resources, err := json.Marshal(msg.Resources)
	if err != nil {
		return err
	}
	var errJSON json.RawMessage
	if msg.Error != nil {
		if errJSON, err = json.Marshal(msg.Error); err != nil {
			return err
		}
	}

	ctx := r.Context()
	endTime := parseTimePtr(msg.EndTime)
	if err := a.opts.Runs.Upsert(ctx, store.ConvergeRun{
		RunID:                runID,
		Org:                  org,
		NodeName:             msg.NodeName,
		Status:               msg.Status,
		StartTime:            parseTimeOr(msg.StartTime, time.Now()),
		EndTime:              endTime,
		TotalResourceCount:   &msg.TotalResourceCount,
		UpdatedResourceCount: &msg.UpdatedResourceCount,
		RunList:              msg.RunList,
		Resources:            resources,
		Error:                errJSON,
		RawMessage:           raw,
	}); err != nil {
		return err
	}

	fields := automate.ExtractNodeFields(msg.Node)
	seenAt := time.Now()
	if endTime != nil {
		seenAt = *endTime
	}
	return a.opts.Nodes.Upsert(ctx, store.NodeUpsert{
		Org:             org,
		Name:            msg.NodeName,
		FQDN:            fields.FQDN,
		Platform:        fields.Platform,
		PlatformFamily:  fields.PlatformFamily,
		PlatformVersion: fields.PlatformVersion,
		Environment:     fields.Environment,
		ChefVersion:     fields.ChefVersion,
		PolicyName:      strOrEmpty(msg.PolicyName),
		PolicyGroup:     strOrEmpty(msg.PolicyGroup),
		Attributes:      msg.Node,
		LastCCRStatus:   msg.Status,
		LatestRunID:     runID,
		SeenAt:          seenAt,
	})
}

func (a *API) ingestComplianceReport(r *http.Request, raw []byte) error {
	var rep automate.ComplianceReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		return err
	}

	// Compliance reports carry no org field at all (Chef Automate's own
	// data model isn't org-scoped) -- the authenticated token's home org is
	// the only source of org scoping here. See docs/specs.
	org := auth.TokenOrg(r.Context())

	profiles, err := json.Marshal(rep.Profiles)
	if err != nil {
		return err
	}
	counts := rep.Counts()
	status := "passed"
	if counts.Failed > 0 {
		status = "failed"
	} else if counts.Passed == 0 && counts.Skipped > 0 {
		status = "skipped"
	}

	reportID := rep.ReportUUID
	endTime := parseTimePtr(&rep.EndTime)
	startTime := time.Now()
	if endTime != nil {
		startTime = endTime.Add(-time.Duration(rep.Statistics.Duration * float64(time.Second)))
	}

	return a.opts.Reports.Create(r.Context(), store.ComplianceReport{
		ReportID:        reportID,
		Org:             org,
		NodeName:        rep.NodeName,
		Environment:     rep.Environment,
		Status:          status,
		ControlsPassed:  &counts.Passed,
		ControlsFailed:  &counts.Failed,
		ControlsSkipped: &counts.Skipped,
		Profiles:        profiles,
		RawReport:       raw,
		StartTime:       startTime,
		EndTime:         endTime,
	}, rep.SearchText())
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseTimeOr(s *string, fallback time.Time) time.Time {
	if t := parseTimePtr(s); t != nil {
		return *t
	}
	return fallback
}

func parseTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}
