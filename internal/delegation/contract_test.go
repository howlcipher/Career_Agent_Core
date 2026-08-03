package delegation

import (
	"strings"
	"testing"
)

func validProposal() []byte {
	return []byte(`{"schema_version":"local-delegation/v1","finding":"missing guard","root_cause":"no contract","planned_files":["internal/delegation/contract.go"],"implementation_summary":"add validation","success_tests":["accept valid JSON"],"failure_tests":["reject unsafe path"],"risks":[],"unresolved_questions":[],"ready_to_edit":true}`)
}

func TestParseProposalAcceptsStrictValidContract(t *testing.T) {
	proposal, err := ParseProposal(validProposal())
	if err != nil {
		t.Fatalf("ParseProposal() error = %v", err)
	}
	if proposal.PlannedFiles[0] != "internal/delegation/contract.go" || !proposal.ReadyToEdit {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
}

func TestParseProposalRejectsUnknownFieldsAndSensitivePaths(t *testing.T) {
	for name, raw := range map[string][]byte{
		"unknown field":  []byte(strings.Replace(string(validProposal()), `"ready_to_edit":true`, `"ready_to_edit":true,"surprise":1`, 1)),
		"sensitive path": []byte(strings.Replace(string(validProposal()), "internal/delegation/contract.go", ".env", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProposal(raw); err == nil {
				t.Fatal("ParseProposal() succeeded, want rejection")
			}
		})
	}
}

func TestValidateBriefRejectsSensitiveMarker(t *testing.T) {
	if err := ValidateBrief([]byte("API_KEY=not-a-real-secret")); err == nil {
		t.Fatal("ValidateBrief() succeeded, want rejection")
	}
}

func TestValidatePatchOnlyAllowsReviewedFiles(t *testing.T) {
	good := []byte("--- a/internal/delegation/contract.go\n+++ b/internal/delegation/contract.go\n@@ -1 +1 @@\n-old\n+new\n")
	if err := ValidatePatch(good, []string{"internal/delegation/contract.go"}); err != nil {
		t.Fatalf("ValidatePatch(good) error = %v", err)
	}
	bad := []byte("--- a/.env\n+++ b/.env\n@@ -1 +1 @@\n-old\n+new\n")
	if err := ValidatePatch(bad, []string{".env"}); err == nil {
		t.Fatal("ValidatePatch(bad) succeeded, want rejection")
	}
}

func TestProposalDigestBindsExactBytes(t *testing.T) {
	if ProposalDigest(validProposal()) == ProposalDigest(append(validProposal(), '\n')) {
		t.Fatal("ProposalDigest() did not change when proposal bytes changed")
	}
}
