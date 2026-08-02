package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

func TestAssistedBrowserProfileDirUsesPrivateCacheSubdirectory(t *testing.T) {
	cacheDir := t.TempDir()
	profileDir, err := assistedBrowserProfileDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cacheDir, "career-agent", "assisted-browser-profile")
	if profileDir != want {
		t.Fatalf("profile directory = %q, want %q", profileDir, want)
	}
	for _, path := range []string{filepath.Join(cacheDir, "career-agent"), profileDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != security.PrivateDirMode {
			t.Fatalf("mode for %q = %o, want %o", path, info.Mode().Perm(), security.PrivateDirMode)
		}
	}
}

func TestAssistedBrowserProfileDirRejectsEmptyCacheDirectory(t *testing.T) {
	if _, err := assistedBrowserProfileDir(""); err == nil {
		t.Fatal("expected empty cache directory to be rejected")
	}
}
