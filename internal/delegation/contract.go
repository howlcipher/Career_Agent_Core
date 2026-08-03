// Package delegation defines the narrow, framework-independent contract used
// by cmd/localdelegate. It deliberately models proposals and patch artifacts,
// not workspace mutation: a reviewer remains responsible for applying a diff.
package delegation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	SchemaVersion     = "local-delegation/v1"
	maxProposalBytes  = 16 << 10
	maxPatchBytes     = 128 << 10
	maxRequestedFiles = 12
	maxBriefBytes     = 32 << 10
)

// Proposal is the required output of the read-only investigation phase.
type Proposal struct {
	SchemaVersion       string   `json:"schema_version"`
	Finding             string   `json:"finding"`
	RootCause           string   `json:"root_cause"`
	PlannedFiles        []string `json:"planned_files"`
	Implementation      string   `json:"implementation_summary"`
	SuccessTests        []string `json:"success_tests"`
	FailureTests        []string `json:"failure_tests"`
	Risks               []string `json:"risks"`
	UnresolvedQuestions []string `json:"unresolved_questions"`
	ReadyToEdit         bool     `json:"ready_to_edit"`
}

// ParseProposal accepts one strict, bounded JSON document from a local model.
func ParseProposal(raw []byte) (Proposal, error) {
	if len(raw) == 0 || len(raw) > maxProposalBytes {
		return Proposal{}, fmt.Errorf("proposal must be between 1 and %d bytes", maxProposalBytes)
	}
	var proposal Proposal
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, fmt.Errorf("decode proposal: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Proposal{}, err
	}
	if proposal.SchemaVersion != SchemaVersion {
		return Proposal{}, fmt.Errorf("proposal schema_version must be %q", SchemaVersion)
	}
	if err := requireText("finding", proposal.Finding); err != nil {
		return Proposal{}, err
	}
	if err := requireText("root_cause", proposal.RootCause); err != nil {
		return Proposal{}, err
	}
	if err := requireText("implementation_summary", proposal.Implementation); err != nil {
		return Proposal{}, err
	}
	if len(proposal.PlannedFiles) == 0 || len(proposal.PlannedFiles) > maxRequestedFiles {
		return Proposal{}, fmt.Errorf("planned_files must contain 1 to %d paths", maxRequestedFiles)
	}
	for _, path := range proposal.PlannedFiles {
		if err := ValidateWorkspacePath(path); err != nil {
			return Proposal{}, fmt.Errorf("planned_files: %w", err)
		}
	}
	if len(proposal.SuccessTests) == 0 || len(proposal.FailureTests) == 0 {
		return Proposal{}, fmt.Errorf("success_tests and failure_tests must both be non-empty")
	}
	return proposal, nil
}

// ProposalDigest binds a reviewer approval to the exact proposal bytes.
func ProposalDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// ValidateBrief rejects oversized and plainly sensitive work packets before
// they can be sent to a local model. It is intentionally conservative.
func ValidateBrief(brief []byte) error {
	if len(brief) == 0 || len(brief) > maxBriefBytes {
		return fmt.Errorf("brief must be between 1 and %d bytes", maxBriefBytes)
	}
	lower := strings.ToLower(string(brief))
	for _, marker := range []string{"api_key", "authorization: bearer", "imap_app_password", "private key", "begin rsa", "begin openssh"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("brief contains prohibited sensitive marker %q", marker)
		}
	}
	return nil
}

// ValidateWorkspacePath permits only normal, relative source or documentation
// paths. It keeps an untrusted response away from credentials, Git metadata,
// production data, and application artifacts.
func ValidateWorkspacePath(path string) error {
	clean := filepath.Clean(path)
	if path == "" || filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q must be a non-empty relative workspace path", path)
	}
	for _, prohibited := range []string{".git/", ".env", "pii.yaml", "applications/", "career_agent.log", "documentation/task_journals/"} {
		if clean == strings.TrimSuffix(prohibited, "/") || strings.HasPrefix(clean, prohibited) {
			return fmt.Errorf("path %q is prohibited", path)
		}
	}
	return nil
}

// ValidatePatch permits a conventional text unified diff that only touches the
// exact files the reviewer saw in the proposal. It never applies the patch.
func ValidatePatch(raw []byte, plannedFiles []string) error {
	if len(raw) == 0 || len(raw) > maxPatchBytes {
		return fmt.Errorf("patch must be between 1 and %d bytes", maxPatchBytes)
	}
	allowed := make(map[string]bool, len(plannedFiles))
	for _, path := range plannedFiles {
		allowed[filepath.ToSlash(filepath.Clean(path))] = true
	}
	lines := strings.Split(string(raw), "\n")
	var oldPath string
	filePairs := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
			return fmt.Errorf("binary patches are prohibited")
		}
		if strings.HasPrefix(line, "--- ") {
			path, err := diffPath(strings.TrimPrefix(line, "--- "))
			if err != nil {
				return err
			}
			oldPath = path
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			path, err := diffPath(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return err
			}
			if oldPath == "" || oldPath != path || !allowed[path] {
				return fmt.Errorf("patch path %q is not an approved planned file", path)
			}
			filePairs++
			oldPath = ""
		}
	}
	if filePairs == 0 || oldPath != "" {
		return fmt.Errorf("patch must contain complete --- and +++ file pairs")
	}
	return nil
}

func diffPath(value string) (string, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", fmt.Errorf("patch path is empty")
	}
	value = fields[0]
	if !strings.HasPrefix(value, "a/") && !strings.HasPrefix(value, "b/") {
		return "", fmt.Errorf("patch path %q must have a/ or b/ prefix", value)
	}
	path := strings.TrimPrefix(strings.TrimPrefix(value, "a/"), "b/")
	if err := ValidateWorkspacePath(path); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(path)), nil
}

func requireText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-empty", name)
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("proposal must contain exactly one JSON value")
		}
		return fmt.Errorf("read proposal tail: %w", err)
	}
	return nil
}
