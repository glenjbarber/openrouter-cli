package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellRunsHexdumpAndOd checks that both reach the program rather than
// being refused on the way to it, which is what distinguishes a name being on
// the list from a name being absent from it.
//
// They are two programs rather than one under two names, so each is looked for
// on its own and each is skipped when the host does not carry it. Neither is
// guaranteed to be present: od is the one POSIX names and hexdump is the one
// the BSDs and macOS carry alongside it, and a host may have either or both.
func TestShellRunsHexdumpAndOd(t *testing.T) {
	dir := t.TempDir()
	// Four printable bytes, so both programs have something to print that is
	// the same answer whichever of the two is asked.
	name := filepath.Join(dir, "bytes.bin")
	if err := os.WriteFile(name, []byte("ABCD"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	if _, err := exec.LookPath("hexdump"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "hexdump", "args": []string{name}})
		if r.Err != nil {
			t.Errorf("hexdump failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "41 42 43 44") {
			// ABCD as hex is what both programs are asked for, and a
			// listing without it is not the bytes the file holds.
			t.Errorf("hexdump returned %q, want the bytes of the file", r.Text)
		}
	}

	if _, err := exec.LookPath("od"); err == nil {
		r := shellCall(t, set, map[string]any{
			"command": "od", "args": []string{"-c", name},
		})
		if r.Err != nil {
			t.Errorf("od failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "A") || !strings.Contains(r.Text, "D") {
			t.Errorf("od returned %q, want the bytes of the file", r.Text)
		}
	}
}
