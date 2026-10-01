//go:build darwin || freebsd || linux || netbsd || openbsd

package tui

import (
	"io"
	"syscall"
	"time"
)

// readWithin reads a block from the descriptor if something arrives within the
// wait, and reports that nothing did otherwise.
//
// A read deadline on the file is the obvious way to bound a wait like this, and
// it is what a program built on a terminal library reaches for. A terminal is
// not given one. Go hands the standard streams to a program through
// os.NewFile, which leaves the descriptor blocking and outside the runtime
// poller, and SetReadDeadline on a descriptor the runtime is not watching fails
// with ErrNoDeadline. The hold asked for a deadline, took the failure as its
// answer, and handed a report that had arrived in pieces back to the key
// reader, whose first byte is an escape. An escape on an empty line ends the
// session, so a wheel notch delivered in pieces took the session down with it.
//
// The kernel is asked instead. select(2) is the obvious candidate and poll(2)
// the modern one, and neither can be used here as it stands: Go spells the
// descriptor set differently on every BSD it carries, so a select needs a
// spelling per system for the sake of one bit, and poll(2) on Darwin ignores
// its timeout and waits for ever when nothing is ready, which is the one case
// the bound exists for. What is left is a read that cannot block and a short
// wait between attempts, which needs no request number and no structure from
// either.
//
// The descriptor is put back the way it was found. The terminal is given to the
// client as it was handed over, and the mode of a descriptor is not the client's
// to keep.
func readWithin(fd int, wait time.Duration, buf []byte) (int, error) {
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
		syscall.F_GETFL, 0)
	if errno != 0 {
		// The mode could not be read, so it could not be put back either. A
		// read that cannot be made without keeping it is not made.
		return 0, errPrefixUnfinished
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		return 0, errPrefixUnfinished
	}
	defer syscall.SetNonblock(fd, flags&syscall.O_NONBLOCK != 0)

	deadline := time.Now().Add(wait)
	for {
		n, err := syscall.Read(fd, buf)
		switch {
		case n > 0:
			return n, nil
		case err == nil:
			// Nothing where a terminal is concerned is a hangup. There is no
			// other end to assemble a report with, and a reader that never
			// sees the end of its input cannot be left.
			return 0, io.EOF
		case err == syscall.EAGAIN || err == syscall.EWOULDBLOCK || err == syscall.EINTR:
			// More may yet arrive. A signal is not the end of the input
			// either, so another attempt is made rather than handing back a
			// report that is merely late.
		default:
			return 0, err
		}
		if !time.Now().Before(deadline) {
			return 0, errPrefixUnfinished
		}
		step := time.Until(deadline)
		if step > readPollStep {
			step = readPollStep
		}
		time.Sleep(step)
	}
}

// readPollStep is how long the wait between attempts sleeps.
//
// It is short enough that a report arriving in pieces is assembled as promptly as
// one arriving whole, and it costs nothing while nothing is arriving, since the
// read that precedes it found nothing to take.
const readPollStep = 2 * time.Millisecond
