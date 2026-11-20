package automate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return raw
}

func TestDetect(t *testing.T) {
	cases := []struct {
		file string
		want Kind
	}{
		{"run_start.json", KindRunStart},
		{"run_converge.json", KindRunConverge},
		{"compliance_report.json", KindComplianceReport},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			got, err := Detect(readTestdata(t, c.file))
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if got != c.want {
				t.Errorf("Detect(%s) = %v, want %v", c.file, got, c.want)
			}
		})
	}
}

func TestDetectUnrecognized(t *testing.T) {
	_, err := Detect([]byte(`{"foo":"bar"}`))
	if err == nil {
		t.Fatal("expected an error for an unrecognized message shape")
	}
}

func TestRunStartMessageFields(t *testing.T) {
	var msg RunStartMessage
	if err := json.Unmarshal(readTestdata(t, "run_start.json"), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.OrganizationName != "acme" {
		t.Errorf("OrganizationName = %q, want %q", msg.OrganizationName, "acme")
	}
	if msg.NodeName != "web01.example.com" {
		t.Errorf("NodeName = %q, want %q", msg.NodeName, "web01.example.com")
	}
	if got := strOrEmptyTest(msg.RunID); got != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("RunID = %q", got)
	}
}

func TestRunConvergeMessageFields(t *testing.T) {
	var msg RunConvergeMessage
	if err := json.Unmarshal(readTestdata(t, "run_converge.json"), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Status != "success" {
		t.Errorf("Status = %q, want success", msg.Status)
	}
	if msg.TotalResourceCount != 2 || msg.UpdatedResourceCount != 1 {
		t.Errorf("resource counts = %d/%d, want 2/1", msg.TotalResourceCount, msg.UpdatedResourceCount)
	}
	if len(msg.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(msg.Resources))
	}
	if msg.Resources[0].Duration != "12" {
		t.Errorf("Resources[0].Duration = %q, want \"12\" (string, not number)", msg.Resources[0].Duration)
	}

	fields := ExtractNodeFields(msg.Node)
	if fields.Platform != "ubuntu" || fields.PlatformFamily != "debian" {
		t.Errorf("ExtractNodeFields platform = %+v", fields)
	}
	if fields.ChefVersion != "19.0.0" {
		t.Errorf("ExtractNodeFields.ChefVersion = %q, want 19.0.0", fields.ChefVersion)
	}
	if fields.Environment != "production" {
		t.Errorf("ExtractNodeFields.Environment = %q, want production", fields.Environment)
	}
}

func TestComplianceReportCountsAndSearchText(t *testing.T) {
	var rep ComplianceReport
	if err := json.Unmarshal(readTestdata(t, "compliance_report.json"), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	counts := rep.Counts()
	if counts.Failed != 1 || counts.Passed != 0 || counts.Skipped != 0 {
		t.Errorf("Counts() = %+v, want {Passed:0 Failed:1 Skipped:0}", counts)
	}

	text := rep.SearchText()
	for _, want := range []string{"ssh-hardening", "Disable root login over SSH", "PermitRootLogin"} {
		if !strings.Contains(text, want) {
			t.Errorf("SearchText() = %q, want it to contain %q", text, want)
		}
	}
}

func strOrEmptyTest(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
