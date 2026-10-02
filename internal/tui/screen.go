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
	// mouse records whether the terminal was asked to report mouse events.
	// It is tracked rather than assumed, since the interface turns reporting
	// on and off while a session runs and the terminal has to be put back the
	// way it was found on the way out.
	mouse bool
	// closed records that the terminal has already been put back, so that a
	// signal arriving during the ordinary exit does not restore it twice.
	closed bool
}

// ANSI control sequences. Only the ones the interface actually uses are
// defined, since a sequence emitted without a reason is a sequence that can
// outlive the interface and leave a terminal looking wrong.
const (
	seqEnterAlt = "\x1b[?1049h"
	seqExitAlt  = "\x1b[?1049l"
	// 25 is the mode that shows and hides the cursor. It is written at startup
	// and on the way out and nowhere else, since the client draws no caret of
	// its own and a terminal with the cursor shown on the alternate screen is a
	// terminal with a blinking caret wherever it was last left.
	seqShowCur   = "\x1b[?25h"
	seqClear     = "\x1b[2J"
	seqClearLine = "\x1b[K"
	seqHome      = "\x1b[H"
	seqResetAttr = "\x1b[0m"
	// Bracketed paste is private mode 2004. Without it a pasted block is
	// indistinguishable from typing, and a newline inside a paste would submit
	// the line halfway through and send half of it as a message.
	// 31 is the bright red of the eight-colour foreground, which is the
	// colour a terminal already means by danger. It is in the eight every
	// terminal has rather than in a cube one of them may not show.
	seqRed      = "\x1b[31m"
	seqPasteOn  = "\x1b[?2004h"
	seqPasteOff = "\x1b[?2004l"
	// Mouse reporting is turned on and off with the private mode 1000, which
	// is the button-event mode the wheel reports arrive in. The alternative
	// modes are not used: 1002 also reports a motion drag, and 1003 reports
	// every motion, both of which a wheel interface has no use for and both of
	// which flood the input stream while the pointer moves.
	seqMouseOn  = "\x1b[?1000h"
	seqMouseOff = "\x1b[?1000l"
	// 1006 is the encoding rather than another mode. It asks for the SGR form
	// of a report rather than the older one, where a coordinate is a single
	// byte and a pane wider or taller than 223 cannot be scrolled at all.
	// Terminal.app sends the older form unless asked, and reports a position
	// past that limit as the last one it can express, so a reader scrolling in
	// a large window watches the view stop at the edge.
	seqMouseSGROn  = "\x1b[?1006h"
	seqMouseSGROff = "\x1b[?1006l"
)

// isTerminal reports whether the file is a terminal.
//
// The query is a termios read rather than a stat on the mode, since a stat
// cannot tell a character device that is a terminal from one that is not.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
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

	// The cursor is shown rather than hidden, and is left blinking rather than
	// made steady. The client draws no caret of its own: it places the terminal
	// one on the prompt row on every paint, and a terminal that blinks it
	// blinks it at whatever rate the reader has configured in their profile.
	//
	// Driving the blink here instead would mean writing the show and hide
	// sequences on a timer, which is a frame the client emits for no reason
	// and a rate that disagrees with every terminal the reader already has
	// set. A terminal configured not to blink is a reader who asked for that.
	s.write(seqEnterAlt + seqClear + seqHome + seqPasteOn)
	if err := s.refreshSize(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// enterRaw switches the terminal to raw mode, saving the prior state.
func (s *Screen) enterRaw() error {
	if err := ioctlTermios(s.in.Fd(), ioctlGetTermios, &s.saved); err != nil {
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

	if err := ioctlTermios(s.in.Fd(), ioctlSetTermios, &raw); err != nil {
		return fmt.Errorf("setting raw mode: %w", err)
	}
	return nil
}

// restore puts the terminal back the way it was found.
func (s *Screen) restore() {
	if s.closed {
		// Restoring twice would leave the alternate screen twice and put the
		// cursor back twice, and the signal handler races the ordinary exit.
		return
	}
	s.closed = true
	ioctlTermios(s.in.Fd(), ioctlSetTermios, &s.saved)
	// Reporting is turned off first, since a terminal left reporting sends
	// every wheel notch into a program that is no longer reading them. It goes
	// through SetMouse so that the encoding is put back as well: a terminal
	// left in the SGR form reports in it to whatever runs next, and the shell
	// the reader is returned to has no use for a wheel report.
	s.SetMouse(false)
	// Bracketed paste is turned off before the alternate screen is left, and
	// not after it. The mode belongs to the terminal rather than to the
	// screen, so it survives the switch back: a terminal left believing it is
	// on wraps a paste in markers that nothing is reading, and the shell the
	// reader is returned to loses the text that was pasted into it.
	s.write(seqPasteOff)
	s.write(seqShowCur + seqExitAlt)
}

// write emits a control sequence.
func (s *Screen) write(seq string) {
	fmt.Fprint(s.out, seq)
}

// Size returns the terminal dimensions in rows and columns.
//
// The dimensions are re-read on every call rather than being cached from
// startup. A terminal resized while the client is running would otherwise be
// drawn to the size it had when it opened, which leaves rows of the prompt
// below the bottom of the window or cuts the reply pane short of its edge. A
// call that cannot read the size keeps the last one rather than reporting
// nothing, since a frame drawn to a stale size is better than no frame at all.
func (s *Screen) Size() (height, width int) {
	if h, w, err := terminalSize(s.out.Fd()); err == nil && h > 0 && w > 0 {
		s.height, s.width = h, w
	}
	return s.height, s.width
}

// refreshSize re-reads the terminal dimensions.
func (s *Screen) refreshSize() error {
	h, w, err := terminalSize(s.out.Fd())
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

// SetMouse turns mouse reporting on or off.
//
// Reporting is not turned on by default. A terminal that reports events
// delivers them to this program alone, so the drag that would otherwise begin a
// selection does not reach the terminal any more and the text cannot be
// selected. It is therefore offered as a choice, and the caller decides.
func (s *Screen) SetMouse(on bool) {
	if on == s.mouse {
		return
	}
	// The encoding is turned on before the mode and off after it, so that a
	// terminal is never asked to report in a form this reader cannot parse,
	// and is not sent a report at all once the mode is off.
	//
	// The two are separate settings on the same terminal, and a reader who
	// turns reporting off and finds the wheel dead has to be able to turn it
	// back on without also having to reset the encoding.
	if on {
		s.write(seqMouseSGROn)
		s.write(seqMouseOn)
	} else {
		s.write(seqMouseOff)
		s.write(seqMouseSGROff)
	}
	s.mouse = on
}

// Mouse reports whether the terminal is being asked to report mouse events.
func (s *Screen) Mouse() bool { return s.mouse }

// Close restores the terminal and releases the screen.
func (s *Screen) Close() {
	s.restore()
}

// terminalSize reads the terminal size through the window-size query.
//
// The query is spelled the same on every supported platform; only the termios
// ioctls differ, so the layout itself lives in the platform files.
func terminalSize(fd uintptr) (height, width int, err error) {
	var ws windowSize
	if err := readWindowSize(fd, &ws); err != nil {
		return 0, 0, fmt.Errorf("reading the terminal size: %w", err)
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
