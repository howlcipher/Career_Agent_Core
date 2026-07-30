//go:build ignore

// verify_tailoring drives one real per-job tailoring call through
// mcp.ProcessJobApplication and prints what came back.
//
// It exists because bug #439 could not be closed by unit tests: the defect was
// that this path asked a live Ollama for a model nothing installs, and the only
// way to see that a fix works is to make a real generation happen. Run it after
// any change to the tailoring path, on both routes:
//
//	# in-process (the default; nothing else needs to be running)
//	OLLAMA_MODEL=qwen3:4b-instruct go run scripts/verify_tailoring.go
//
//	# offloaded to nlp_service (start it first: uvicorn main:app --port 8000)
//	NLP_SERVICE_URL=http://localhost:8000 OLLAMA_MODEL=qwen3:4b-instruct \
//	    go run scripts/verify_tailoring.go
//
// A small model is fine here -- this checks the wiring, not output quality.
// Watch the log lines: they name which route was taken and which model was
// asked for, which is the thing that was wrong.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
)

func main() {
	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))

	scraped := map[string]string{
		"title": "Senior Automation Engineer",
		"desc": "We need an engineer to build internal automation in Go and " +
			"Python: log parsing, anomaly detection on service metrics, and " +
			"secure deployment pipelines. Kubernetes and Terraform are a plus. " +
			"Fully remote in the United States.",
	}
	constraints := map[string]interface{}{
		"cover_letter_tone":   "direct and technical",
		"target_compensation": 165000,
	}
	profile := "Nine years of IT and software experience. Python and Go " +
		"automation tooling, log parsing and anomaly detection, MS in Cyber " +
		"Defense coursework, CCNA foundation, secure network infrastructure " +
		"deployments."

	start := time.Now()
	resume, cover, prep, err := client.ProcessJobApplication(scraped, constraints, profile)
	elapsed := time.Since(start).Round(time.Second)

	if err != nil {
		log.Fatalf("FAILED after %s: %v", elapsed, err)
	}

	fmt.Printf("\nOK in %s\n", elapsed)
	for _, doc := range []struct {
		name string
		body string
	}{
		{"resume", resume},
		{"cover letter", cover},
		{"interview prep", prep},
	} {
		fmt.Printf("\n--- %s (%d chars) ---\n%s\n", doc.name, len(doc.body), firstLines(doc.body, 4))
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "...")
	}
	return strings.Join(lines, "\n")
}
