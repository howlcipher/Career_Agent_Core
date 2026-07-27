package security

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	PrivateDirMode  os.FileMode = 0700
	PrivateFileMode os.FileMode = 0600
)

var privateWorkspaceFiles = []string{
	".env",
	"pii.yaml",
	"profile.yaml",
	"master_resume.pdf",
	"master_cover_letter.txt",
	"Omni_CoverLetter.pdf",
}

var privateWorkspaceGlobs = []string{
	"applications*.db*",
	"career_agent*.log",
	"agent_run_batch*.log",
}

// SetPrivateUmask ensures files and directories created by this process do
// not grant group or other permissions, even when a call site requests a
// broader mode. Explicit private modes remain the primary control; the umask
// is a process-wide backstop for SQLite, log rotation, and future writers.
func SetPrivateUmask() {
	setPrivateUmask()
}

// PreparePrivateWorkspace installs the restrictive process policy and repairs
// the known private paths below root. Callers receive the error so they can
// fail closed. The warning is also written immediately because later logging
// may itself depend on a path that could not be secured.
func PreparePrivateWorkspace(root string, warningWriter io.Writer) error {
	SetPrivateUmask()
	err := RepairPrivatePaths(root)
	if err != nil && warningWriter != nil {
		fmt.Fprintf(
			warningWriter,
			"WARNING: private file permissions could not be fully secured: %v\n",
			err,
		)
	}
	return err
}

// RepairPrivatePaths applies owner-only modes to the repository's known
// credentials, databases, logs, source documents, and generated application
// tree. It never follows symbolic links and reports every path it refuses or
// cannot repair.
func RepairPrivatePaths(root string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	rootInfo, err := os.Lstat(absoluteRoot)
	if err != nil {
		return fmt.Errorf("inspect workspace root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symbolic link workspace root %q", absoluteRoot)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("workspace root %q is not a directory", absoluteRoot)
	}

	var repairErrors []error
	seen := make(map[string]struct{})
	secureFile := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		if err := secureExistingFile(path); err != nil {
			repairErrors = append(repairErrors, err)
		}
	}

	for _, name := range privateWorkspaceFiles {
		secureFile(filepath.Join(absoluteRoot, name))
	}
	for _, pattern := range privateWorkspaceGlobs {
		matches, globErr := filepath.Glob(filepath.Join(absoluteRoot, pattern))
		if globErr != nil {
			repairErrors = append(
				repairErrors,
				fmt.Errorf("match private paths for %q: %w", pattern, globErr),
			)
			continue
		}
		for _, path := range matches {
			secureFile(path)
		}
	}

	applicationsPath := filepath.Join(absoluteRoot, "applications")
	if err := secureApplicationTree(applicationsPath); err != nil {
		repairErrors = append(repairErrors, err)
	}
	return errors.Join(repairErrors...)
}

// SecurePrivateFile applies the private file mode to one existing regular
// file without following a symbolic link. A missing file is already safe to
// create later under SetPrivateUmask, so it is not an error.
func SecurePrivateFile(path string) error {
	return secureExistingFile(path)
}

func secureApplicationTree(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private directory %q: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symbolic link private directory %q", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("private application path %q is not a directory", root)
	}
	if info.Mode().Perm() != PrivateDirMode {
		if err := chmodNoFollow(root, PrivateDirMode, true); err != nil {
			return err
		}
	}

	var repairErrors []error
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			repairErrors = append(
				repairErrors,
				fmt.Errorf("walk private path %q: %w", path, walkErr),
			)
			return nil
		}
		if path == root {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			repairErrors = append(
				repairErrors,
				fmt.Errorf("inspect private path %q: %w", path, err),
			)
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			repairErrors = append(
				repairErrors,
				fmt.Errorf("refusing symbolic link private path %q", path),
			)
			return nil
		}

		switch {
		case info.IsDir():
			if info.Mode().Perm() != PrivateDirMode {
				if err := chmodNoFollow(path, PrivateDirMode, true); err != nil {
					repairErrors = append(repairErrors, err)
				}
			}
		case info.Mode().IsRegular():
			if info.Mode().Perm() != PrivateFileMode {
				if err := chmodNoFollow(path, PrivateFileMode, false); err != nil {
					repairErrors = append(repairErrors, err)
				}
			}
		default:
			repairErrors = append(
				repairErrors,
				fmt.Errorf("refusing non-regular private path %q", path),
			)
		}
		return nil
	})
	if walkErr != nil {
		repairErrors = append(
			repairErrors,
			fmt.Errorf("walk private application tree %q: %w", root, walkErr),
		)
	}
	return errors.Join(repairErrors...)
}

func secureExistingFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symbolic link private file %q", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular private file %q", path)
	}
	if info.Mode().Perm() == PrivateFileMode {
		return nil
	}
	return chmodNoFollow(path, PrivateFileMode, false)
}
