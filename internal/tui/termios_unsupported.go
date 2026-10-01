//go:build !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !linux

package tui

import "syscall"

// A platform with neither the BSD nor the System V termios ioctl naming cannot
// drive the terminal here. The build is made to fail with an explanation rather
// than at run time with an undefined constant, since a target that cannot be
// built is easier to diagnose than one that fails when first used.
const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)

// windowSize mirrors the common layout.
type windowSize struct {
	rows, cols, xpixel, ypixel uint16
}

// readWindowSize reports that the platform is not supported.
func readWindowSize(fd uintptr, w *windowSize) error {
	return syscall.ENOTSUP
}
