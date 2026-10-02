//go:build !windows

package saved

import "os"

// osGetenv is the environment read, kept behind one name so that the encoding
// does not reach for the process environment directly.
func osGetenv(key string) string { return os.Getenv(key) }
