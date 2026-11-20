package automate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Kind identifies which of the three data-collector message shapes a raw
// payload is.
type Kind int

const (
	KindUnknown Kind = iota
	KindRunStart
	KindRunConverge
	KindComplianceReport
)

// peek is the subset of fields needed to tell the three message shapes
// apart without fully decoding them: chef-client messages carry
// "message_type", InSpec's automate reporter instead carries "type":
// "inspec_report".
type peek struct {
	MessageType string `json:"message_type"`
	Type        string `json:"type"`
}

// Detect inspects raw and reports which data-collector message it is.
func Detect(raw []byte) (Kind, error) {
	var p peek
	if err := json.Unmarshal(raw, &p); err != nil {
		return KindUnknown, fmt.Errorf("automate: not a JSON object: %w", err)
	}
	switch {
	case p.MessageType == "run_start":
		return KindRunStart, nil
	case p.MessageType == "run_converge":
		return KindRunConverge, nil
	case p.Type == "inspec_report":
		return KindComplianceReport, nil
	default:
		return KindUnknown, fmt.Errorf("automate: unrecognized message (message_type=%q type=%q)", p.MessageType, p.Type)
	}
}

// ControlCounts tallies control-level outcomes across every profile in a
// report, the way Cinc Console's compliance summary needs them.
type ControlCounts struct {
	Passed  int
	Failed  int
	Skipped int
}

// Counts reports pass/fail/skip totals for r. A control counts as failed if
// any of its results failed, skipped if all results were skipped, and
// passed otherwise -- the same roll-up InSpec's own CLI summary uses.
func (r *ComplianceReport) Counts() ControlCounts {
	var c ControlCounts
	for _, profile := range r.Profiles {
		for _, control := range profile.Controls {
			switch controlStatus(control) {
			case "failed":
				c.Failed++
			case "skipped":
				c.Skipped++
			default:
				c.Passed++
			}
		}
	}
	return c
}

func controlStatus(c Control) string {
	if len(c.Results) == 0 {
		return "skipped"
	}
	allSkipped := true
	for _, res := range c.Results {
		if res.Status == "failed" {
			return "failed"
		}
		if res.Status != "skipped" {
			allSkipped = false
		}
	}
	if allSkipped {
		return "skipped"
	}
	return "passed"
}

// SearchText flattens the human-readable text of a report -- profile and
// control titles/descriptions, and failure messages -- into one string for
// SereneDB's inverted index, which is declared over a real text column
// rather than a JSON path. See docs/specs for why.
func (r *ComplianceReport) SearchText() string {
	var b strings.Builder
	for _, profile := range r.Profiles {
		b.WriteString(profile.Name)
		b.WriteByte(' ')
		b.WriteString(profile.Title)
		b.WriteByte(' ')
		for _, control := range profile.Controls {
			b.WriteString(control.ID)
			b.WriteByte(' ')
			b.WriteString(control.Title)
			b.WriteByte(' ')
			b.WriteString(control.Desc)
			b.WriteByte(' ')
			for _, res := range control.Results {
				if res.Message != "" {
					b.WriteString(res.Message)
					b.WriteByte(' ')
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}
