//go:build !freebsd

package tui

import (
	"os"
	"testing"
)

// openResizablePTY is unavailable outside FreeBSD, where the pty ioctl numbers
// differ from every other platform Go builds for. The resize tests skip rather
// than fail, since the behaviour they check is in the client and is exercised on
// the platform it was written for.
//
// Nothing in the client is conditioned on this helper. It exists so that the
// terminal size can be changed under the test rather than being a fixed value
// read from a regular file, which would pass whether the size were cached or
// re-read.
func openResizablePTY(t *testing.T) (*os.File, func(rows, cols uint16)) {
	t.Helper()
	t.Skip("the pty helper is implemented for FreeBSD only")
	return nil, nil
}
