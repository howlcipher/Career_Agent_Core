//go:build ignore

// audit_prompt_injection reports aggregate prompt-injection audit statistics
// without printing URLs, employer names, matched text, or payload content.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
)

const auditPath = "applications/prompt_injection_detections.csv"

func main() {
	f, err := os.Open(auditPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	for _, required := range []string{"detected_at", "threat_type", "severity", "guard", "matched_text"} {
		if _, ok := index[required]; !ok {
			log.Fatalf("audit CSV lacks %q column", required)
		}
	}

	typeCounts := map[string]int{}
	encodingByGuard := map[string]int{}
	encodingBySeverity := map[string]int{}
	encodingEvidence := map[string]int{}
	encodingByDay := map[string]int{}
	var total, emptyMatched, encodingTotal, encodingEmpty int
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if len(record) != len(header) {
			log.Fatalf("malformed audit row: got %d fields, want %d", len(record), len(header))
		}
		total++
		if record[index["matched_text"]] == "" {
			emptyMatched++
		}
		threatType := record[index["threat_type"]]
		typeCounts[threatType]++
		if threatType != "encoding_attack" {
			continue
		}
		encodingTotal++
		if record[index["matched_text"]] == "" {
			encodingEmpty++
		}
		encodingByGuard[record[index["guard"]]]++
		encodingBySeverity[record[index["severity"]]]++
		evidence := "located"
		if record[index["matched_text"]] == "" {
			evidence = "unlocated"
		}
		encodingEvidence[record[index["guard"]]+"/"+record[index["severity"]]+"/"+evidence]++
		detectedAt := record[index["detected_at"]]
		if len(detectedAt) < len("2006-01-02") {
			log.Fatalf("malformed detected_at value %q", detectedAt)
		}
		encodingByDay[detectedAt[:len("2006-01-02")]]++
	}

	fmt.Printf("all_detections=%d\nall_empty_matched_text=%d\n", total, emptyMatched)
	printCounts("threat_type", typeCounts)
	fmt.Printf("encoding_attack_total=%d\nencoding_attack_empty_matched_text=%d\n", encodingTotal, encodingEmpty)
	printCounts("encoding_attack_guard", encodingByGuard)
	printCounts("encoding_attack_severity", encodingBySeverity)
	printCounts("encoding_attack_evidence", encodingEvidence)
	printCounts("encoding_attack_day", encodingByDay)
}

func printCounts(label string, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s[%s]=%d\n", label, key, counts[key])
	}
}
