// Package tui renders the interactive terminal interface.
//
// It is built on the standard library only. The terminal is driven through
// syscalls rather than a third-party library, so that mouse reporting and the
// alternate screen are under direct control. That control is required by the
// copyable-text requirement, since a higher-level library captures the mouse by
// default and a capture intercepts the drag that begins a selection.
package tui

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// ErrNotTerminal reports that the output is not a terminal.
//
// The interface is not drawn in that case. Escape sequences written to a pipe
// or a file are noise in the captured output, and a caller redirecting the
// client expects text rather than a frame.
var ErrNotTerminal = errors.New("output is not a terminal")

// Screen is a terminal in raw mode on the alternate screen.
type Screen struct {
	out    *os.File
	in     *os.File
	saved  syscall.Termios
	height int
	width  int
}

// ANSI control sequences. Only the ones the interface actually uses are
// defined, since a sequence emitted without a reason is a sequence that can
// outlive the interface and leave a terminal looking wrong.
const (
	seqEnterAlt  = "\x1b[?1049h"
	seqExitAlt   = "\x1b[?1049l"
	seqHideCur   = "\x1b[?25l"
	seqShowCur   = "\x1b[?25h"
	seqClear     = "\x1b[2J"
	seqClearLine = "\x1b[K"
	seqHome      = "\x1b[H"
	seqResetAttr = "\x1b[0m"
)

// isTerminal reports whether the file is a terminal.
//
// The query is a termios read rather than a stat on the mode, since a stat
// cannot tell a character device that is a terminal from one that is not.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}

// IsTerminal reports whether the given file is a terminal.
func IsTerminal(f *os.File) bool {
	return isTerminal(f)
}

// NewScreen puts the terminal into raw mode on the alternate screen.
//
// The previous terminal state is saved and restored by Close, so that a crash
// cannot leave the terminal in raw mode with no echo. Raw mode is required
// because the interface reads keys itself rather than leaving the line
// discipline to assemble a line.
func NewScreen(out, in *os.File) (*Screen, error) {
	if !isTerminal(out) {
		return nil, ErrNotTerminal
	}
	if !isTerminal(in) {
		return nil, ErrNotTerminal
	}

	s := &Screen{out: out, in: in}
	if err := s.enterRaw(); err != nil {
		return nil, err
	}

	// A signal restores the terminal before the process exits, since a
	// terminal left in raw mode has no echo and no line discipline.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		s.Close()
		os.Exit(1)
	}()

	s.write(seqEnterAlt + seqHideCur + seqClear + seqHome)
	if err := s.refreshSize(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// enterRaw switches the terminal to raw mode, saving the prior state.
func (s *Screen) enterRaw() error {
	if err := ioctlTermios(s.in.Fd(), syscall.TIOCGETA, &s.saved); err != nil {
		return fmt.Errorf("reading the terminal state: %w", err)
	}

	raw := s.saved
	// Input arrives a byte at a time without echo, so that keys are seen as
	// they are pressed rather than when a line is completed.
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := ioctlTermios(s.in.Fd(), syscall.TIOCSETA, &raw); err != nil {
		return fmt.Errorf("setting raw mode: %w", err)
	}
	return nil
}

// restore puts the terminal back the way it was found.
func (s *Screen) restore() {
	ioctlTermios(s.in.Fd(), syscall.TIOCSETA, &s.saved)
	s.write(seqShowCur + seqExitAlt)
}

// write emits a control sequence.
func (s *Screen) write(seq string) {
	fmt.Fprint(s.out, seq)
}

// Size returns the terminal dimensions in rows and columns.
func (s *Screen) Size() (height, width int) {
	return s.height, s.width
}

// refreshSize re-reads the terminal dimensions.
func (s *Screen) refreshSize() error {
	h, w, err := windowSize(s.out.Fd())
	if err != nil {
		return err
	}
	if h <= 0 || w <= 0 {
		// A zero size is reported by a terminal that has just started or has
		// no window yet. Falling back keeps the frame drawable rather than
		// looping on arithmetic that would divide by zero.
		h, w = 24, 80
	}
	s.height, s.width = h, w
	return nil
}

// Close restores the terminal and releases the screen.
func (s *Screen) Close() {
	s.restore()
}

// windowSize reads the terminal size through the window-size query.
func windowSize(fd uintptr) (height, width int, err error) {
	var ws struct {
		rows, cols, xpixel, ypixel uint16
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0, fmt.Errorf("reading the terminal size: %w", errno)
	}
	return int(ws.rows), int(ws.cols), nil
}

// ioctlTermios reads or writes the terminal attributes on fd.
func ioctlTermios(fd uintptr, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}
