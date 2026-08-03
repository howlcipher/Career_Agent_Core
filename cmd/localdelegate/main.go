// Command localdelegate provides a deliberately narrow local-Ollama delegation
// workflow. It creates reviewed JSON proposals and candidate patch artifacts;
// it never executes tools, applies patches, touches Git, controls a browser,
// writes application data, or accesses credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/internal/delegation"
	"github.com/howlcipher/Career_Agent_Core/internal/modelbench"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("localdelegate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	phase := fs.String("phase", "", "required: propose or patch")
	host := fs.String("host", delegation.DefaultHost, "local Ollama base URL")
	model := fs.String("model", "", "required local Ollama model")
	briefFile := fs.String("brief-file", "", "required bounded, sanitized task brief")
	proposalFile := fs.String("proposal-file", "", "proposal output for propose; reviewed proposal input for patch")
	patchFile := fs.String("patch-file", "", "required output file for patch phase")
	approvedDigest := fs.String("approved-proposal-sha256", "", "required exact digest of a reviewed proposal")
	approvedBy := fs.String("approved-by", "", "required human reviewer identifier")
	timeout := fs.Duration("timeout", 2*time.Minute, "local Ollama request timeout")
	lockPath := fs.String("lock-path", modelbench.AgentLockPath, "agent single-instance lock; delegation refuses while it is held")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*model) == "" || strings.TrimSpace(*briefFile) == "" || strings.TrimSpace(*proposalFile) == "" {
		fmt.Fprintln(stderr, "localdelegate: -model, -brief-file, and -proposal-file are required")
		return 2
	}
	brief, err := os.ReadFile(*briefFile)
	if err != nil {
		fmt.Fprintf(stderr, "localdelegate: read brief: %v\n", err)
		return 1
	}
	if err := delegation.ValidateBrief(brief); err != nil {
		fmt.Fprintf(stderr, "localdelegate: unsafe brief: %v\n", err)
		return 1
	}
	running, pid, err := modelbench.IsAgentRunning(*lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "localdelegate: check agent lock: %v\n", err)
		return 1
	}
	if running {
		fmt.Fprintf(stderr, "localdelegate: production agent appears to be running (lock held, pid %d); local delegation refuses to compete with application work\n", pid)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch *phase {
	case "propose":
		raw, err := delegation.GenerateProposal(ctx, nil, *host, *model, string(brief))
		if err != nil {
			fmt.Fprintf(stderr, "localdelegate: proposal generation: %v\n", err)
			return 1
		}
		if _, err := delegation.ParseProposal(raw); err != nil {
			fmt.Fprintf(stderr, "localdelegate: rejected proposal: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*proposalFile, raw, 0600); err != nil {
			fmt.Fprintf(stderr, "localdelegate: write proposal: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Proposal written to %s\nReview digest: %s\n", *proposalFile, delegation.ProposalDigest(raw))
		return 0
	case "patch":
		if strings.TrimSpace(*patchFile) == "" || strings.TrimSpace(*approvedBy) == "" || strings.TrimSpace(*approvedDigest) == "" {
			fmt.Fprintln(stderr, "localdelegate: patch phase requires -patch-file, -approved-by, and -approved-proposal-sha256")
			return 2
		}
		proposalRaw, err := os.ReadFile(*proposalFile)
		if err != nil {
			fmt.Fprintf(stderr, "localdelegate: read proposal: %v\n", err)
			return 1
		}
		if delegation.ProposalDigest(proposalRaw) != *approvedDigest {
			fmt.Fprintln(stderr, "localdelegate: approval digest does not match the reviewed proposal")
			return 1
		}
		proposal, err := delegation.ParseProposal(proposalRaw)
		if err != nil {
			fmt.Fprintf(stderr, "localdelegate: rejected reviewed proposal: %v\n", err)
			return 1
		}
		if !proposal.ReadyToEdit {
			fmt.Fprintln(stderr, "localdelegate: proposal is not ready_to_edit; a reviewer must resolve its questions first")
			return 1
		}
		prompt := fmt.Sprintf("Approved reviewer: %s\nApproved paths: %s\nTask brief:\n%s", *approvedBy, strings.Join(proposal.PlannedFiles, ", "), brief)
		patch, err := delegation.GeneratePatch(ctx, nil, *host, *model, prompt)
		if err != nil {
			fmt.Fprintf(stderr, "localdelegate: patch generation: %v\n", err)
			return 1
		}
		if err := delegation.ValidatePatch(patch, proposal.PlannedFiles); err != nil {
			fmt.Fprintf(stderr, "localdelegate: rejected patch: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*patchFile, patch, 0600); err != nil {
			fmt.Fprintf(stderr, "localdelegate: write patch: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Candidate patch written to %s; it has not been applied.\n", *patchFile)
		return 0
	default:
		fmt.Fprintln(stderr, "localdelegate: -phase must be propose or patch")
		return 2
	}
}
