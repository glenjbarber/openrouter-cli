//go:build unix

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The mode passed to open is filtered through the umask, so a umask clearing
// any of the owner bits leaves a file at a mode the loader refuses on the next
// run, which is the client rejecting its own work. The mode is set on the open
// descriptor for that reason, so the file lands at 0600 whatever the umask is.
//
// The umask is process wide, so this is not marked parallel, and it is restored
// before the test returns however the test ends.
func TestDefaultIsCreatedAtTheRequiredModeUnderAnyUmask(t *testing.T) {
	for _, mask := range []int{0o022, 0o077, 0o777, 0o200, 0o600} {
		t.Run(umaskName(mask), func(t *testing.T) {
			// The directory is made first, since the umask governs its
			// creation too and a mask clearing the owner bits would leave the
			// test unable to create anything at all.
			dir := t.TempDir()

			old := syscall.Umask(mask)
			defer syscall.Umask(old)

			path := filepath.Join(dir, DefaultFileName)
			written, err := InstallDefaultAt(path)
			if err != nil {
				t.Fatalf("InstallDefaultAt: %v", err)
			}
			if !written {
				t.Fatal("written = false, want true")
			}

			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if got := info.Mode().Perm(); got != RequiredMode {
				t.Errorf("mode = %04o, want %04o under a umask of %04o", got, RequiredMode, mask)
			}

			// The file the client wrote must be one the client then accepts.
			if err := checkMode(path); err != nil {
				t.Errorf("checkMode: %v", err)
			}
		})
	}
}

// A concurrent reader must not be able to observe the file between its
// creation and the write, which would show a configuration holding no
// endpoint. The file is created empty and filled immediately, so the window is
// small rather than absent; what is asserted is that the file is never readable
// at a permissive mode while it is empty, which is the exposure the mode
// governs.
func TestEmptyFileIsNeverPermissive(t *testing.T) {
	dir := t.TempDir()

	old := syscall.Umask(0)
	defer syscall.Umask(old)

	path := filepath.Join(dir, DefaultFileName)
	written, err := InstallDefaultAt(path)
	if err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("mode = %04o, want no group or other access", info.Mode().Perm())
	}
}

// umaskName renders a mask for a subtest name.
func umaskName(mask int) string {
	const digits = "01234567"
	return string([]byte{
		'0', 'o',
		digits[(mask>>6)&7], digits[(mask>>3)&7], digits[mask&7],
	})
}
