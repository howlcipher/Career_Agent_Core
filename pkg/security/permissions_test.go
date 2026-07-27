//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package security

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSetPrivateUmaskRestrictsNewFilesAndDirectories(t *testing.T) {
	previous := syscall.Umask(0)
	syscall.Umask(previous)
	t.Cleanup(func() {
		syscall.Umask(previous)
	})

	SetPrivateUmask()

	root := t.TempDir()
	filePath := filepath.Join(root, "private.txt")
	if err := os.WriteFile(filePath, []byte("private"), 0666); err != nil {
		t.Fatalf("write private file: %v", err)
	}
	dirPath := filepath.Join(root, "private")
	if err := os.Mkdir(dirPath, 0777); err != nil {
		t.Fatalf("create private directory: %v", err)
	}

	assertMode(t, filePath, PrivateFileMode)
	assertMode(t, dirPath, PrivateDirMode)
}

func TestRepairPrivatePathsSecuresKnownFilesAndApplicationTree(t *testing.T) {
	root := t.TempDir()
	writeWithMode(t, filepath.Join(root, ".env"), 0644)
	writeWithMode(t, filepath.Join(root, "applications.db"), 0666)
	writeWithMode(t, filepath.Join(root, "master_resume.pdf"), 0644)
	writeWithMode(t, filepath.Join(root, "career_agent-old.log"), 0640)

	companyDir := filepath.Join(root, "applications", "Example")
	if err := os.MkdirAll(companyDir, 0755); err != nil {
		t.Fatalf("create application directory: %v", err)
	}
	applicationPath := filepath.Join(companyDir, "resume.md")
	writeWithMode(t, applicationPath, 0644)

	if err := RepairPrivatePaths(root); err != nil {
		t.Fatalf("repair private paths: %v", err)
	}
	if err := RepairPrivatePaths(root); err != nil {
		t.Fatalf("second repair must be idempotent: %v", err)
	}

	assertMode(t, filepath.Join(root, ".env"), PrivateFileMode)
	assertMode(t, filepath.Join(root, "applications.db"), PrivateFileMode)
	assertMode(t, filepath.Join(root, "master_resume.pdf"), PrivateFileMode)
	assertMode(t, filepath.Join(root, "career_agent-old.log"), PrivateFileMode)
	assertMode(t, filepath.Join(root, "applications"), PrivateDirMode)
	assertMode(t, companyDir, PrivateDirMode)
	assertMode(t, applicationPath, PrivateFileMode)
}

func TestRepairPrivatePathsRefusesSymlinksWithoutChangingTargets(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	envTarget := filepath.Join(outside, "env-target")
	writeWithMode(t, envTarget, 0644)
	if err := os.Symlink(envTarget, filepath.Join(root, ".env")); err != nil {
		t.Fatalf("create credential symlink: %v", err)
	}

	applicationsDir := filepath.Join(root, "applications")
	if err := os.Mkdir(applicationsDir, 0755); err != nil {
		t.Fatalf("create applications directory: %v", err)
	}
	documentTarget := filepath.Join(outside, "document-target")
	writeWithMode(t, documentTarget, 0644)
	if err := os.Symlink(documentTarget, filepath.Join(applicationsDir, "resume.md")); err != nil {
		t.Fatalf("create document symlink: %v", err)
	}

	err := RepairPrivatePaths(root)
	if err == nil {
		t.Fatal("repair must report skipped symlinks")
	}
	if count := strings.Count(err.Error(), "refusing symbolic link"); count != 2 {
		t.Fatalf("repair error must identify both refused symlinks, got %q", err)
	}

	assertMode(t, envTarget, 0644)
	assertMode(t, documentTarget, 0644)
	assertMode(t, applicationsDir, PrivateDirMode)
}

func TestPreparePrivateWorkspacePrintsClearWarning(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	writeWithMode(t, target, 0644)
	if err := os.Symlink(target, filepath.Join(root, "pii.yaml")); err != nil {
		t.Fatalf("create PII symlink: %v", err)
	}

	var warning bytes.Buffer
	err := PreparePrivateWorkspace(root, &warning)
	if err == nil {
		t.Fatal("prepare must return the repair failure")
	}
	got := warning.String()
	if !strings.Contains(got, "WARNING") ||
		!strings.Contains(got, "private file permissions could not be fully secured") {
		t.Fatalf("warning is not clear: %q", got)
	}
}

func writeWithMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("private"), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set mode on %s: %v", path, err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}
