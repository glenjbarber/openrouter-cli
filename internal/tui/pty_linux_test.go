//go:build linux

package tui

import (
	"os"
	"strconv"
	"syscall"
	"testing"
	"unsafe"
)

// System V spells the window-size request with the G, unlike the BSD family.
const ioctlSetWindowSize = syscall.TIOCSWINSZ

const (
	ioctlUnlockPTY = syscall.TIOCSPTLCK
	ioctlPTYNumber = syscall.TIOCGPTN
)

// openResizablePTY returns the master side of a pseudo-terminal pair along with
// a function that resizes it.
//
// A pty is the only thing whose reported window size changes under the reader,
// which is exactly the case a cached size missed. The ioctl numbers differ
// between the BSD family and System V, so the two families are spelled
// separately in files named to match the termios layer that already has to
// agree with them.
func openResizablePTY(t *testing.T) (*os.File, func(rows, cols uint16)) {
	t.Helper()

	master, slaveName, err := openPTYMaster()
	if err != nil {
		t.Skipf("opening a pseudo-terminal: %v", err)
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Skipf("opening the pty slave: %v", err)
	}
	t.Cleanup(func() {
		slave.Close()
		master.Close()
	})

	setSize := func(rows, cols uint16) {
		t.Helper()
		ws := windowSize{rows: rows, cols: cols}
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(),
			uintptr(ioctlSetWindowSize), uintptr(unsafe.Pointer(&ws))); errno != 0 {
			t.Fatalf("setting the pty size: %v", errno)
		}
	}
	return master, setSize
}

// openPTYMaster unlocks a pty and returns it with the name of its slave.
func openPTYMaster() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, "", err
	}
	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(),
		uintptr(ioctlUnlockPTY), uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		master.Close()
		return nil, "", errno
	}
	var number uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(),
		uintptr(ioctlPTYNumber), uintptr(unsafe.Pointer(&number))); errno != 0 {
		master.Close()
		return nil, "", errno
	}
	return master, "/dev/pts/" + strconv.Itoa(int(number)), nil
}
