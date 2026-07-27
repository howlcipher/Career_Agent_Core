//go:build windows

package security

import (
	"fmt"
	"os"
)

func setPrivateUmask() {
}

func chmodNoFollow(path string, mode os.FileMode, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symbolic link private path %q", path)
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("private directory changed type while securing %q", path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("private file changed type while securing %q", path)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set private mode on %q: %w", path, err)
	}
	return nil
}
