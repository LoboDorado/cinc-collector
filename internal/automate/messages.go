// Package automate defines the wire format of Chef Automate's
// data-collector API: the run_start/run_converge messages chef-client (or
// cinc-server-ng's data-collector reverse proxy) sends, and the compliance
// report InSpec's "automate" reporter plugin sends. These shapes are taken
// directly from chef/chef's lib/chef/data_collector/*.rb and inspec/inspec's
// lib/inspec/reporters/{automate,json,json_automate}.rb, not reconstructed
// from documentation, since Cinc Collector must byte-for-byte accept what
// real clients actually send.
package automate

import "encoding/json"

// RunStartMessage is sent at the beginning of a chef-client run.
// message_version "1.0.0".
type RunStartMessage struct {
	MessageType      string  `json:"message_type"` // always "run_start"
	MessageVersion   string  `json:"message_version"`
	ChefServerFQDN   string  `json:"chef_server_fqdn"`
	EntityUUID       string  `json:"entity_uuid"`
	ID               *string `json:"id"`
	NodeName         string  `json:"node_name"`
	OrganizationName string  `json:"organization_name"`
	RunID            *string `json:"run_id"`
	Source           string  `json:"source"`
	StartTime        *string `json:"start_time"` // ISO8601 UTC
}

// RunConvergeMessage reports the end of a chef-client run's converge phase.
// message_version "1.1.0". Loosely-typed nested fields (node, cookbooks,
// expanded_run_list, deprecations) are kept as json.RawMessage since their
// shape isn't pinned by the source and Cinc Collector stores them opaquely
// anyway.
type RunConvergeMessage struct {
	MessageType          string          `json:"message_type"` // always "run_converge"
	MessageVersion       string          `json:"message_version"`
	ChefServerFQDN       string          `json:"chef_server_fqdn"`
	EntityUUID           string          `json:"entity_uuid"`
	ID                   *string         `json:"id"`
	NodeName             string          `json:"node_name"`
	OrganizationName     string          `json:"organization_name"`
	RunID                *string         `json:"run_id"`
	Source               string          `json:"source"`
	Status               string          `json:"status"` // "success" | "failure"
	StartTime            *string         `json:"start_time"`
	EndTime              *string         `json:"end_time"`
	TotalResourceCount   int             `json:"total_resource_count"`
	UpdatedResourceCount int             `json:"updated_resource_count"`
	PolicyName           *string         `json:"policy_name"`
	PolicyGroup          *string         `json:"policy_group"`
	RunList              json.RawMessage `json:"run_list"`
	ExpandedRunList      json.RawMessage `json:"expanded_run_list"`
	Node                 json.RawMessage `json:"node"`
	Cookbooks            json.RawMessage `json:"cookbooks"`
	Deprecations         json.RawMessage `json:"deprecations"`
	Resources            []Resource      `json:"resources"`
	Error                *RunError       `json:"error,omitempty"`
}

// Resource is one entry in a run_converge message's "resources" array.
// Note "duration" and "delta" are strings on the wire, not numbers.
type Resource struct {
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	ID              string          `json:"id"`
	After           json.RawMessage `json:"after"`
	Before          json.RawMessage `json:"before"`
	Duration        string          `json:"duration"`
	Delta           string          `json:"delta"`
	IgnoreFailure   bool            `json:"ignore_failure"`
	Result          string          `json:"result"`
	Status          string          `json:"status"` // updated | up-to-date | skipped | failed | unprocessed
	CookbookName    string          `json:"cookbook_name,omitempty"`
	CookbookVersion *string         `json:"cookbook_version,omitempty"`
	RecipeName      string          `json:"recipe_name,omitempty"`
	Conditional     string          `json:"conditional,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	Error           *RunError       `json:"error,omitempty"`
}

// RunError describes a run-level or resource-level exception.
type RunError struct {
	Class       string   `json:"class"`
	Message     string   `json:"message"`
	Backtrace   []string `json:"backtrace"`
	Description string   `json:"description"`
}

// ComplianceReport is the payload InSpec's "automate" reporter plugin POSTs
// to the data-collector endpoint for a compliance scan.
type ComplianceReport struct {
	Type        string          `json:"type"` // always "inspec_report"
	Platform    Platform        `json:"platform"`
	Profiles    []Profile       `json:"profiles"`
	Statistics  Statistics      `json:"statistics"`
	Version     string          `json:"version"`
	EndTime     string          `json:"end_time"`
	NodeUUID    string          `json:"node_uuid"`
	ReportUUID  string          `json:"report_uuid"`
	NodeName    string          `json:"node_name,omitempty"`
	Environment string          `json:"environment,omitempty"`
	JobUUID     string          `json:"job_uuid,omitempty"`
	Passthrough json.RawMessage `json:"passthrough,omitempty"`
}

// Platform identifies the scanned target.
type Platform struct {
	Name     string `json:"name"`
	Release  string `json:"release"`
	TargetID string `json:"target_id"`
}

// Statistics holds scan-level stats. InSpec only ever populates Duration.
type Statistics struct {
	Duration float64 `json:"duration"`
}

// Profile is one InSpec profile's results within a report.
type Profile struct {
	Name           string          `json:"name"`
	Version        string          `json:"version,omitempty"`
	SHA256         string          `json:"sha256"`
	Title          string          `json:"title,omitempty"`
	Maintainer     string          `json:"maintainer,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	License        string          `json:"license,omitempty"`
	Copyright      string          `json:"copyright,omitempty"`
	CopyrightEmail string          `json:"copyright_email,omitempty"`
	Supports       json.RawMessage `json:"supports,omitempty"`
	Attributes     json.RawMessage `json:"attributes,omitempty"`
	Depends        json.RawMessage `json:"depends,omitempty"`
	Controls       []Control       `json:"controls"`
	Status         string          `json:"status,omitempty"`
	StatusMessage  string          `json:"status_message,omitempty"`
	SkipMessage    string          `json:"skip_message,omitempty"`
}

// Control is one InSpec control within a profile.
type Control struct {
	ID             string          `json:"id"`
	Title          string          `json:"title,omitempty"`
	Desc           string          `json:"desc,omitempty"`
	Descriptions   []Description   `json:"descriptions,omitempty"`
	Impact         float64         `json:"impact"`
	Refs           json.RawMessage `json:"refs,omitempty"`
	Tags           json.RawMessage `json:"tags,omitempty"`
	Code           string          `json:"code,omitempty"`
	SourceLocation SourceLocation  `json:"source_location"`
	Results        []Result        `json:"results"`
}

// Description is one labeled description block on a control.
type Description struct {
	Label string `json:"label"`
	Data  string `json:"data"`
}

// SourceLocation points at where a control is defined.
type SourceLocation struct {
	Ref  string `json:"ref"`
	Line int    `json:"line"`
}

// Result is one assertion's outcome within a control's run.
type Result struct {
	Status         string   `json:"status"`
	CodeDesc       string   `json:"code_desc"`
	RunTime        float64  `json:"run_time"`
	StartTime      string   `json:"start_time"`
	SkipMessage    string   `json:"skip_message,omitempty"`
	Message        string   `json:"message,omitempty"`
	Exception      string   `json:"exception,omitempty"`
	Backtrace      []string `json:"backtrace,omitempty"`
	ResourceClass  string   `json:"resource_class,omitempty"`
	ResourceParams string   `json:"resource_params,omitempty"`
	ResourceID     string   `json:"resource_id,omitempty"`
}
