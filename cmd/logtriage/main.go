// Command logtriage reads log lines from standard input and emits a compact,
// redacted JSON context packet. It never reads application data, writes files,
// invokes Git, sends email, opens a browser, or submits an application.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/internal/logtriage"
	"github.com/howlcipher/Career_Agent_Core/internal/modelbench"
)

func main() {
	fs := flag.NewFlagSet("logtriage", flag.ExitOnError)
	model := fs.String("model", "", "optional installed local Ollama model; empty uses deterministic triage only")
	host := fs.String("host", modelbench.DefaultHost, "Ollama base URL")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum optional model-call duration")
	fs.Parse(os.Args[1:])

	var events []logtriage.Event
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for scanner.Scan() {
		events = append(events, logtriage.Event{Source: "stdin", Line: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "logtriage: read standard input: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*model) != "" {
		if err := modelbench.CheckModelsAvailable(context.Background(), *host, []string{*model}); err != nil {
			fmt.Fprintf(os.Stderr, "logtriage: %v\n", err)
			os.Exit(1)
		}
	}
	packet := logtriage.Analyze(context.Background(), events, logtriage.Options{Host: *host, Model: *model, Timeout: *timeout})
	if err := json.NewEncoder(os.Stdout).Encode(packet); err != nil {
		fmt.Fprintf(os.Stderr, "logtriage: encode packet: %v\n", err)
		os.Exit(1)
	}
}
