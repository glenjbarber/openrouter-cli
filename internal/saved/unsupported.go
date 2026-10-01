//go:build dragonfly

package saved

import "fmt"

// DragonFly is left without a database driver.
//
// The pure Go driver carries an emulation of the C library, and that emulation
// has no DragonFly in it, so the client would fail to build for a target the
// project supports. Rather than drop the target, the two calls that need a
// driver are answered with an explanation.
//
// The commands stay in the vocabulary and report this, rather than being
// compiled out. A reader who typed /save on a platform where it cannot work is
// told that it cannot work; a reader who typed a command this build does not
// have is told the command is unknown, which is a different problem and the
// wrong one to send them chasing.
func Write(path string, _ Session, _ bool) error {
	return fmt.Errorf("%w: this build has no database driver, so %s was not written",
		ErrUnsupported, path)
}

// Read reports that there is nothing to read.
func Read(path string) (*Session, error) {
	return nil, fmt.Errorf("%w: this build has no database driver, so %s was not read",
		ErrUnsupported, path)
}
