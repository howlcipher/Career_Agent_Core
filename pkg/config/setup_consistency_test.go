package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The setup path has now produced the same bug three times: the agent ends up
// configured for a model nothing installed (bugs.md #439, the Usability Gate's
// own stale Ollama wording, and #441). #441's specific instance was that
// scripts/install_ollama.sh pulled llama3.1 + llava while .env.example shipped
// uncommented qwen names, so following the documented steps in the documented
// order guaranteed a broken run.
//
// The runtime preflight in pkg/mcp catches every such mismatch at startup, but
// only for someone who already installed. These tests catch the mismatch in
// the repository, before it is shipped to anyone — the two files are only
// consistent by convention, and nothing but this asserts it.
//
// Both are parsed with the same tolerant rules godotenv and the installer use:
// optional `export`, optional quotes, `#` comments.

var (
	envAssignmentRE = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?([A-Z0-9_]+)[ \t]*=(.*)$`)
	// Matches both the plain form, VAR="${OLLAMA_MODEL:-llama3.1}", and the
	// .env-aware one the installer uses now,
	// VAR="${OLLAMA_MODEL:-${ENV_TEXT_MODEL:-llama3.1}}", capturing the
	// innermost literal in either case.
	shDefaultRE = regexp.MustCompile(`(?m)^[ \t]*[A-Z_]+="\$\{([A-Z0-9_]+):-(?:\$\{[A-Z0-9_]+:-)?([^}$]*)\}`)
	// Anchored at the start of a command, so prose mentioning ".env" in a
	// comment cannot be mistaken for a sourcing call.
	sourcesEnvRE = regexp.MustCompile(`(?m)^[ \t]*(source|\.)[ \t]+("?\$?\{?ENV_FILE\}?"?|"?\.env"?)`)
)

func unquoteEnvValue(raw string) string {
	value := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(value, `"`):
		value = value[1:]
		if end := strings.Index(value, `"`); end >= 0 {
			value = value[:end]
		}
	case strings.HasPrefix(value, `'`):
		value = value[1:]
		if end := strings.Index(value, `'`); end >= 0 {
			value = value[:end]
		}
	default:
		if hash := strings.Index(value, "#"); hash >= 0 {
			value = value[:hash]
		}
	}
	return strings.TrimSpace(value)
}

// activeEnvExampleValues returns only the assignments a user gets by copying
// the file verbatim — commented-out recommendations are documentation, not
// configuration, which is the whole distinction #441 turned on.
func activeEnvExampleValues(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := envAssignmentRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		values[m[1]] = unquoteEnvValue(m[2])
	}
	return values
}

// installerDefaults reads the fallbacks out of the bash installer's
// `VAR="${OLLAMA_X:-default}"` lines, so the assertion tracks the script rather
// than a copy of it.
func installerDefaults(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install_ollama.sh"))
	if err != nil {
		t.Fatalf("read scripts/install_ollama.sh: %v", err)
	}
	defaults := make(map[string]string)
	for _, m := range shDefaultRE.FindAllStringSubmatch(string(raw), -1) {
		if _, seen := defaults[m[1]]; !seen {
			defaults[m[1]] = m[2]
		}
	}
	return defaults
}

func TestEnvExampleModelsAgreeWithInstallerDefaults(t *testing.T) {
	active := activeEnvExampleValues(t)
	defaults := installerDefaults(t)

	for _, key := range []string{"OLLAMA_MODEL", "OLLAMA_VISION_MODEL", "OLLAMA_EMBED_MODEL"} {
		fallback, ok := defaults[key]
		if !ok {
			t.Errorf("scripts/install_ollama.sh no longer defines a default for %s; the installer and .env.example can now drift silently", key)
			continue
		}
		configured, isActive := active[key]
		if !isActive {
			// Commented out in .env.example: the user inherits the installer's
			// default, which is by definition consistent.
			continue
		}
		if configured != fallback {
			t.Errorf(
				"%s is set to %q in .env.example but scripts/install_ollama.sh defaults to %q. "+
					"A user who runs the installer and copies .env.example verbatim would be "+
					"configured for a model that was never pulled (bugs.md #441). Either comment "+
					"the line out in .env.example or change the installer's default to match",
				key, configured, fallback,
			)
		}
	}
}

func TestInstallerReadsEnvAndVerifiesItsPulls(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install_ollama.sh"))
	if err != nil {
		t.Fatalf("read scripts/install_ollama.sh: %v", err)
	}
	script := string(raw)

	// Behaviours, not wording: each of these is a half of #441's fix, and each
	// would be silently removable without them.
	for _, required := range []struct {
		needle string
		why    string
	}{
		{"/api/tags", "the installer must verify its pulls against Ollama's installed-model list, not assume `ollama pull` succeeded"},
		{"env_file_get", "the installer must read .env so it pulls what the agent is configured to use"},
		{"normalize_model_name", "an untagged pull is stored as :latest, so both sides need normalizing or the check reports installed models as missing"},
	} {
		if !strings.Contains(script, required.needle) {
			t.Errorf("scripts/install_ollama.sh no longer contains %q: %s", required.needle, required.why)
		}
	}

	// .env holds ANTHROPIC_API_KEY and IMAP_APP_PASSWORD. Sourcing it would
	// execute user-authored shell and pull every secret into the installer's
	// environment; the parser exists specifically to avoid that.
	if m := sourcesEnvRE.FindString(script); m != "" {
		t.Errorf("scripts/install_ollama.sh appears to source .env (%q); it must parse only the OLLAMA_* keys instead, because .env holds credentials", strings.TrimSpace(m))
	}
}
