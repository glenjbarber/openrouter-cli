//go:build darwin || freebsd || netbsd || openbsd

package tui

import (
	"syscall"
	"unsafe"
)

// The BSD family spells the termios ioctls with a trailing A, where the
// System V spelling used by Linux omits it. Both operate on the same structure,
// so only the request numbers differ.
const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)

// windowSize is the BSD layout of the window-size query.
type windowSize struct {
	rows, cols, xpixel, ypixel uint16
}

// readWindowSize fills w from the terminal.
func readWindowSize(fd uintptr, w *windowSize) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(w)))
	if errno != 0 {
		return errno
	}
	return nil
}
