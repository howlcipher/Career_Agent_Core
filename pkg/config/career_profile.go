package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// CareerProfilePathEnv provides a portable environment override shared by
	// cmd/agent and cmd/reingest.
	CareerProfilePathEnv = "CAREER_PROFILE_PATH"
	// DefaultCareerProfilePath is the repository-local profile name checked
	// before the standard sibling knowledge-library checkout.
	DefaultCareerProfilePath = "USER_PROFILE.md"
)

// ResolveCareerProfilePath selects and validates the career profile used for
// RAG ingestion. Explicit flags take precedence over the environment. Without
// either, it checks a repository-local USER_PROFILE.md and then the sibling
// ai_knowledge_library layout referenced by this repository's AGENTS.md.
func ResolveCareerProfilePath(flagPath, envPath, baseDir string) (string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = "."
	}
	absoluteBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve career-profile base directory: %w", err)
	}

	if configured := strings.TrimSpace(flagPath); configured != "" {
		return validateConfiguredCareerProfile(configured, absoluteBase, "-profile")
	}
	if configured := strings.TrimSpace(envPath); configured != "" {
		return validateConfiguredCareerProfile(
			configured,
			absoluteBase,
			CareerProfilePathEnv,
		)
	}

	candidates := []string{
		filepath.Join(absoluteBase, DefaultCareerProfilePath),
		filepath.Join(
			filepath.Dir(absoluteBase),
			"ai_knowledge_library",
			DefaultCareerProfilePath,
		),
	}
	for _, candidate := range candidates {
		resolved, err := ValidateCareerProfilePath(candidate)
		if err == nil {
			return resolved, nil
		}
	}

	return "", fmt.Errorf(
		"no readable career profile found; provide -profile, set %s, "+
			"place %s in the repository or sibling ai_knowledge_library, "+
			"or start cmd/agent with -no-rag",
		CareerProfilePathEnv,
		DefaultCareerProfilePath,
	)
}

func validateConfiguredCareerProfile(path, baseDir, source string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	resolved, err := ValidateCareerProfilePath(path)
	if err != nil {
		return "", fmt.Errorf(
			"career profile configured by %s is not readable: %w",
			source,
			err,
		)
	}
	return resolved, nil
}

// ValidateCareerProfilePath confirms that path identifies a readable regular
// file. It deliberately does not read or return profile contents.
func ValidateCareerProfilePath(path string) (string, error) {
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	file, err := os.Open(absolutePath)
	if err != nil {
		return "", fmt.Errorf("open profile: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect profile: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("profile must be a regular file")
	}
	return absolutePath, nil
}
