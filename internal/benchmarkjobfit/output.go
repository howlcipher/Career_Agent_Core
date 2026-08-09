package benchmarkjobfit

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WritePrivateArtifacts writes ignored benchmark data with user-only modes.
func WritePrivateArtifacts(
	cohortPath string,
	reviewPath string,
	labelsPath string,
	cohort Cohort,
	labelCount int,
) error {
	if labelCount < 0 {
		return fmt.Errorf("label count must not be negative")
	}
	if labelCount > len(cohort.Jobs) {
		labelCount = len(cohort.Jobs)
	}
	encoded, err := json.MarshalIndent(cohort, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cohort: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := writePrivateFile(cohortPath, encoded); err != nil {
		return fmt.Errorf("write cohort: %w", err)
	}

	var review strings.Builder
	review.WriteString("# Private job-fit human review\n\n")
	review.WriteString("This ignored local artifact contains sanitized excerpts from production postings. Do not commit it.\n\n")
	review.WriteString("Label each item in the companion CSV: 3 = strong match / actively apply; 2 = reasonable; 1 = weak; 0 = clearly wrong. Optionally set must_apply or would_skip to true.\n\n")
	for index := 0; index < labelCount; index++ {
		item := cohort.Jobs[index]
		review.WriteString("## " + item.BenchmarkID + ": " + item.Title + "\n\n")
		review.WriteString("Current score band: " + item.ScoreBand + "\n\n")
		excerpt := []rune(item.Description)
		if len(excerpt) > 1800 {
			excerpt = append(excerpt[:1800], '…')
		}
		review.WriteString(string(excerpt) + "\n\n")
		review.WriteString("Human label (0-3): \n\nMust apply (true/false): \n\nWould skip (true/false): \n\nNotes: \n\n")
	}
	if err := writePrivateFile(reviewPath, []byte(review.String())); err != nil {
		return fmt.Errorf("write human review: %w", err)
	}

	var labels bytes.Buffer
	writer := csv.NewWriter(&labels)
	if err := writer.Write([]string{"benchmark_id", "human_label_0_3", "must_apply", "would_skip", "reviewer_notes"}); err != nil {
		return fmt.Errorf("write labels header: %w", err)
	}
	for index := 0; index < labelCount; index++ {
		if err := writer.Write([]string{cohort.Jobs[index].BenchmarkID, "", "", "", ""}); err != nil {
			return fmt.Errorf("write label row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush labels: %w", err)
	}
	if err := writePrivateFile(labelsPath, labels.Bytes()); err != nil {
		return fmt.Errorf("write human labels: %w", err)
	}
	return nil
}

func writePrivateFile(path string, contents []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".benchmark-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
