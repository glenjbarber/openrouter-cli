package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// maxHops bounds a symlink chain. The bound is a backstop so that a cycle
// produces a diagnostic instead of hanging.
const maxHops = 32

// resolve follows a symlink chain and returns the final target.
//
// A link is followed only while the target stays on the same filesystem as the
// link, compared by device identifier. A cross-device link is refused rather
// than followed, since a link that leaves the filesystem it was created in is
// reaching outside the directory the user pointed at. The final target of a
// chain is what is compared, and a loop is reported rather than followed.
func resolve(path string) (string, error) {
	current := path
	for hop := 0; hop <= maxHops; hop++ {
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("examining %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}

		// The link itself is compared against its target. Lstat is what gives
		// the link without following it; Stat would return the target and so
		// would compare the target with itself.
		linkInfo := info
		dest, err := os.Readlink(current)
		if err != nil {
			return "", fmt.Errorf("reading the link %s: %w", current, err)
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(current), dest)
		}

		targetInfo, err := os.Stat(dest)
		if err != nil {
			return "", fmt.Errorf("resolving %s to %s: %w", current, dest, err)
		}
		if !sameDevice(linkInfo, targetInfo) {
			return "", fmt.Errorf(
				"%s is a link to %s, which is on another filesystem: "+
					"a link that leaves its own filesystem is refused",
				current, dest,
			)
		}
		current = dest
	}
	return "", fmt.Errorf("%s is a link chain of more than %d hops", path, maxHops)
}

// sameDevice reports whether two file information values name the same
// filesystem. The stat structures are platform specific, so the comparison is
// isolated here rather than spread through the walk.
func sameDevice(a, b os.FileInfo) bool {
	as, aok := a.Sys().(*syscall.Stat_t)
	bs, bok := b.Sys().(*syscall.Stat_t)
	if !aok || !bok {
		// Without a device identifier there is nothing to compare, so the
		// link is followed rather than refused on a guess.
		return true
	}
	return as.Dev == bs.Dev
}
