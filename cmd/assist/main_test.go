package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
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

func TestContinueAssistedApplicationKeepsBrowserOpenForManualReview(t *testing.T) {
	oldLoadDocument := loadAssistedDocument
	oldLoadPII := loadAssistedPII
	oldFillPage := fillAssistedPage
	oldRecordRefill := recordAssistedRefill
	oldRecordManualReview := recordAssistedManualReview
	t.Cleanup(func() {
		loadAssistedDocument = oldLoadDocument
		loadAssistedPII = oldLoadPII
		fillAssistedPage = oldFillPage
		recordAssistedRefill = oldRecordRefill
		recordAssistedManualReview = oldRecordManualReview
	})

	for _, tc := range []struct {
		name     string
		docErr   error
		fillErr  error
		wantFill bool
	}{
		{name: "documents unavailable", docErr: errors.New("missing document")},
		{name: "refill rejected", fillErr: errors.New("mapping no longer usable"), wantFill: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manualReviewRecorded := false
			fillCalled := false
			loadAssistedDocument = func(_ *sql.DB, _, _ string) (storage.AssistedDocument, error) {
				return storage.AssistedDocument{Path: "fixture"}, tc.docErr
			}
			loadAssistedPII = func(string) (*config.PII, error) { return &config.PII{}, nil }
			fillAssistedPage = func(playwright.Page, *security.QuarantineLayer, string, string, string, string, *config.PII) error {
				fillCalled = true
				return tc.fillErr
			}
			recordAssistedRefill = func(*sql.DB, string, string, time.Time) error {
				t.Fatal("refill state recorded after continuation failure")
				return nil
			}
			recordAssistedManualReview = func(*sql.DB, string, string, time.Time) error {
				manualReviewRecorded = true
				return nil
			}

			keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "1"}, "owner")
			if !keepOpen || !manualReviewRecorded || fillCalled != tc.wantFill {
				t.Fatalf("keepOpen=%v manualReviewRecorded=%v fillCalled=%v", keepOpen, manualReviewRecorded, fillCalled)
			}
		})
	}
}

func TestAssistedBrowserProfileDirRejectsEmptyCacheDirectory(t *testing.T) {
	if _, err := assistedBrowserProfileDir(""); err == nil {
		t.Fatal("expected empty cache directory to be rejected")
	}
}

func TestAssistedPageTitleMatchesRole(t *testing.T) {
	for _, tc := range []struct {
		title string
		role  string
		want  bool
	}{
		{"Meesho - Platform Engineer", "Platform Engineer", true},
		{"Meesho - Forward Deployed Engineer II", "Platform Engineer", false},
		{"CloudCo | Senior Platform Engineer", "Platform Engineer", true},
		{"CloudCo - Platform Engineering Manager", "Platform Engineer", false},
		{"Meesho", "Platform Engineer", false},
	} {
		if got := assistedPageTitleMatchesRole(tc.title, tc.role); got != tc.want {
			t.Errorf("assistedPageTitleMatchesRole(%q, %q) = %v, want %v", tc.title, tc.role, got, tc.want)
		}
	}
}
