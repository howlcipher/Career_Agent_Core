package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/util"
)

// Startup model preflight (bugs.md #441).
//
// Three bugs in this repository have had the same shape: the agent was
// configured for a model that was not installed, and nothing found out until a
// real job failed hours into a run, once per job. #439 was a hardcoded
// "llama3" that nothing ever pulled; #441 was the documented setup path itself
// — `./scripts/install_ollama.sh` pulled llama3.1 + llava while .env.example
// shipped uncommented qwen names, so following both documented steps in order
// produced a working install configured for two absent models.
//
// The installer now reads .env and verifies its own pulls against /api/tags,
// which fixes setup. This file closes the remaining half: the agent asks the
// same question at startup, so *any* route to a missing model — a hand-edited
// .env, a model deleted later, a second machine sharing the config, a bad
// OLLAMA_HOST — is a loud startup error instead of a slow per-job one.
//
// Deliberately not a fallback: it does not silently substitute an installed
// model. Which model writes the documents that go to an employer is the user's
// decision, and quietly downgrading it is how #439 stayed invisible.

// modelPreflighter is implemented by providers that depend on models installed
// on the local machine. Optional interface, discovered by type assertion
// exactly like offloadTarget in provider_ollama.go: a provider needing nothing
// local (geminiProvider) simply does not implement it, and PreflightModels
// then has nothing to check.
type modelPreflighter interface {
	preflightModels(ctx context.Context) error
}

// preflightModelTimeout bounds the /api/tags call only. It is deliberately
// nothing like the provider's own timeout (120 minutes, sized for CPU
// generation): listing installed models is a local metadata read that either
// answers immediately or is not going to answer at all, and a startup check
// that can hang for two hours is worse than no startup check.
const preflightModelTimeout = 15 * time.Second

// configuredModel pairs a model name with the environment variable that
// selected it. The env var is the actionable half of any error — "llama3.1 is
// missing" sends the user hunting through the source, "llama3.1
// (OLLAMA_MODEL)" names the line of .env to change.
type configuredModel struct {
	envVar string
	name   string
}

// PreflightModels verifies that every model the configured provider needs
// locally is actually installed, and returns an error naming all of the
// missing ones at once. It returns nil for providers with no local model
// dependency.
//
// Callers should treat a non-nil error as fatal at startup: every affected
// call would fail anyway, just later and more expensively. cmd/agent does
// exactly that, with SKIP_MODEL_PREFLIGHT as an escape hatch for anyone
// pointing OLLAMA_HOST at a remote server whose tag list they cannot read.
func (c *Client) PreflightModels(ctx context.Context) error {
	p, ok := c.provider.(modelPreflighter)
	if !ok {
		log.Printf("[LLM] Model preflight skipped: provider %q needs no local models.", c.provider.Name())
		return nil
	}
	return p.preflightModels(ctx)
}

// preflightModels checks every model this provider can route a call to.
func (p *ollamaProvider) preflightModels(ctx context.Context) error {
	wanted := []configuredModel{
		{envVar: "OLLAMA_MODEL", name: p.model},
		{envVar: "OLLAMA_VISION_MODEL", name: p.visionModel},
		{envVar: "OLLAMA_EMBED_MODEL", name: p.embedModel},
	}
	// fastModel is opt-in and empty by default (see the field's comment); an
	// unset one is not a missing one.
	if p.fastModel != "" {
		wanted = append(wanted, configuredModel{envVar: "OLLAMA_FAST_MODEL", name: p.fastModel})
	}
	return p.checkModelsInstalled(ctx, wanted)
}

// preflightModels checks only the embedding model. A Claude user's text and
// vision calls go to Anthropic, so their OLLAMA_MODEL / OLLAMA_VISION_MODEL
// values are irrelevant and must not be able to fail their startup — but
// Anthropic has no embeddings API, so this provider genuinely does need a
// local embedding model (see provider_claude.go's embed field).
func (p *claudeProvider) preflightModels(ctx context.Context) error {
	return p.embed.checkModelsInstalled(ctx, []configuredModel{
		{envVar: "OLLAMA_EMBED_MODEL", name: p.embed.embedModel},
	})
}

// checkModelsInstalled compares wanted against the server's installed set and
// reports every miss in one error. It logs on success too: this repository has
// paid for the rule that a shipped fix is not a working fix until something
// observes it firing (bugs.md #76), and a silent check is indistinguishable
// from one that never ran.
func (p *ollamaProvider) checkModelsInstalled(ctx context.Context, wanted []configuredModel) error {
	installed, err := p.installedModels(ctx)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(installed))
	for _, name := range installed {
		have[name] = true
	}

	// Two variables can legitimately name the same model; report it once,
	// naming both, rather than twice.
	var order []string
	sources := make(map[string][]string)
	for _, w := range wanted {
		if strings.TrimSpace(w.name) == "" {
			continue
		}
		if _, seen := sources[w.name]; !seen {
			order = append(order, w.name)
		}
		sources[w.name] = append(sources[w.name], w.envVar)
	}

	var missing, confirmed []string
	for _, name := range order {
		label := fmt.Sprintf("%s (%s)", name, strings.Join(sources[name], ", "))
		if have[normalizeModelTag(name)] {
			confirmed = append(confirmed, name)
			continue
		}
		missing = append(missing, label)
	}

	if len(missing) == 0 {
		log.Printf("[LLM] Model preflight passed against %s: %s", p.host, strings.Join(confirmed, ", "))
		return nil
	}

	pulls := make([]string, 0, len(missing))
	for _, name := range order {
		if !have[normalizeModelTag(name)] {
			pulls = append(pulls, "ollama pull "+name)
		}
	}
	installedList := "none"
	if len(installed) > 0 {
		installedList = strings.Join(installed, ", ")
	}
	return fmt.Errorf(
		"ollama at %s is missing %d configured model(s): %s. Installed there: %s. "+
			"Fix either side — pull what you configured (%s), or point those variables in .env "+
			"at a model you already have. ./scripts/install_ollama.sh reads .env and pulls "+
			"whatever it names, so re-running it also resolves this",
		p.host,
		len(missing),
		strings.Join(missing, "; "),
		installedList,
		strings.Join(pulls, " && "),
	)
}

// ollamaTagsResponse is the subset of GET /api/tags this check reads.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// installedModels returns the normalized, sorted names Ollama reports as
// installed. An empty slice with a nil error is a real answer: a reachable
// server with nothing pulled, which is a different problem from an unreachable
// one and gets a different message.
func (p *ollamaProvider) installedModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, preflightModelTimeout)
	defer cancel()

	url := p.host + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build ollama /api/tags request for %s: %w", p.host, err)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"could not reach ollama at %s to verify configured models (is it running? "+
				"run ./scripts/install_ollama.sh, or set OLLAMA_HOST to point elsewhere; "+
				"set SKIP_MODEL_PREFLIGHT=1 to start anyway): %w",
			p.host, err,
		)
	}
	defer resp.Body.Close()

	body, err := util.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ollama /api/tags response from %s: %w", p.host, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama at %s returned HTTP %d from /api/tags: %s", p.host, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tags ollamaTagsResponse
	if err := json.Unmarshal(body, &tags); err != nil {
		return nil, fmt.Errorf("failed to parse ollama /api/tags response from %s: %w", p.host, err)
	}

	names := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		if strings.TrimSpace(m.Name) == "" {
			continue
		}
		names = append(names, normalizeModelTag(m.Name))
	}
	sort.Strings(names)
	return names, nil
}

// normalizeModelTag makes both sides of the comparison agree about implicit
// tags. Ollama stores an untagged pull as ":latest", so a configured
// "llama3.1" is installed as "llama3.1:latest" and a literal comparison would
// report a present model as missing. A check that cries wolf gets disabled,
// which would put us right back where #441 started.
func normalizeModelTag(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ":") {
		return name
	}
	return name + ":latest"
}
