//go:build linux

package tui

import (
	"syscall"
	"unsafe"
)

// Linux uses the System V spelling of the termios ioctls, which omits the
// trailing A that the BSD family uses. The structure layout is the same.
const (
	ioctlGetTermios = syscall.TCGETS
	ioctlSetTermios = syscall.TCSETS
)

// windowSize is the Linux layout of the window-size query, whose field names
// differ from the BSD spelling but whose shape does not.
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
