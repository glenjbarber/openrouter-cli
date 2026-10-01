package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A link whose target is genuinely on another filesystem must be refused.
//
// The check under test compares device identifiers, so the test needs a real
// second device rather than a simulated one. The candidate is discovered at run
// time and the test skips when the host offers only one writable filesystem,
// which is the case on the maintainer's host.
func TestCrossDeviceLinkRefused(t *testing.T) {
	second := secondFilesystem(t)

	target := filepath.Join(second, "openrouter-cli-xdev-test.md")
	if err := os.WriteFile(target, []byte("Be terse."), 0o600); err != nil {
		t.Skipf("cannot write to %s: %v", second, err)
	}
	t.Cleanup(func() { os.Remove(target) })

	dir := t.TempDir()
	link := filepath.Join(dir, "MEMORY.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Load(link)
	if err == nil {
		t.Fatal("Load followed a cross-device link, want a refusal")
	}
	if !strings.Contains(err.Error(), "another filesystem") {
		t.Errorf("err = %q, want it to mention the filesystem boundary", err)
	}
}

// secondFilesystem returns a writable directory on a device other than the one
// backing the temporary directory.
func secondFilesystem(t *testing.T) string {
	t.Helper()

	base, err := os.Stat(os.TempDir())
	if err != nil {
		t.Skipf("TempDir: %v", err)
	}

	for _, dir := range secondFilesystemCandidates {
		if !writable(dir) {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || sameDevice(base, info) {
			continue
		}
		return dir
	}

	t.Skip("no writable second filesystem on this host")
	return ""
}

// secondFilesystemCandidates are the mount points worth probing. None is
// present on every host, which is why the search skips rather than fails.
var secondFilesystemCandidates = []string{
	"/var/tmp",
	"/var/log",
	"/dev/shm",
	"/run/shm",
}

func writable(dir string) bool {
	probe := filepath.Join(dir, "openrouter-cli-probe")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}
