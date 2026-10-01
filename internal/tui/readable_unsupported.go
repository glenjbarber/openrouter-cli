//go:build !darwin && !freebsd && !linux && !netbsd && !openbsd

package tui

import "time"

// readWithin reports that nothing arrived, since there is no bounded read to be
// had here.
//
// Every platform the client is built for has one, in the file that carries the
// request it is made with, and this is the other side of that choice. A read
// with no bound is not taken, since it would swallow a lone escape: that is a
// key the reader pressed, and the one key that interrupts.
func readWithin(fd int, wait time.Duration, buf []byte) (int, error) {
	return 0, errPrefixUnfinished
}
