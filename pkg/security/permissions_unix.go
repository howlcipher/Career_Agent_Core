//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package security

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func setPrivateUmask() {
	unix.Umask(0077)
}

// chmodNoFollow opens the final path component with O_NOFOLLOW and applies the
// mode through the resulting descriptor. This prevents a symbolic-link target
// from being changed between inspection and chmod.
func chmodNoFollow(path string, mode os.FileMode, directory bool) error {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return fmt.Errorf("open private path without following links %q: %w", path, err)
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("inspect opened private path %q: %w", path, err)
	}
	fileType := stat.Mode & unix.S_IFMT
	if directory && fileType != unix.S_IFDIR {
		return fmt.Errorf("private directory changed type while securing %q", path)
	}
	if !directory && fileType != unix.S_IFREG {
		return fmt.Errorf("private file changed type while securing %q", path)
	}
	if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
		return fmt.Errorf("set private mode on %q: %w", path, err)
	}
	return nil
}
