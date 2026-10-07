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
//
// The programs are given the file by its bare name rather than by its path from
// the root. The arguments are held to the directory the command runs in, and a
// temporary directory on Darwin resolves from /var/folders to
// /private/var/folders, so an absolute path into it reads as reaching out of
// the tree. A bare name lands inside it whichever of the two spellings the
// root is, which is what a model in that directory would send anyway.
func TestShellRunsHexdumpAndOd(t *testing.T) {
	dir := t.TempDir()
	// Four printable bytes, so both programs have something to print that is
	// the same answer whichever of the two is asked.
	if err := os.WriteFile(filepath.Join(dir, "bytes.bin"), []byte("ABCD"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	if _, err := exec.LookPath("hexdump"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "hexdump", "args": []string{"bytes.bin"}})
		if r.Err != nil {
			t.Errorf("hexdump failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "4241") {
			// hexdump groups its bytes, and how many it puts in a group is
			// an option rather than a fixed part of the output: this one
			// prints two to a group, giving 4241 4443 for ABCD, which is 41 42 43 44
			// read a different way. The check
			// is on the first group only, since that is where A sits in
			// every grouping, rather than on the whole line, which is
			// spelled differently by each host.
			t.Errorf("hexdump returned %q, want the bytes of the file", r.Text)
		}
	}

	if _, err := exec.LookPath("od"); err == nil {
		r := shellCall(t, set, map[string]any{
			"command": "od", "args": []string{"-c", "bytes.bin"},
		})
		if r.Err != nil {
			t.Errorf("od failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "A") || !strings.Contains(r.Text, "D") {
			// -c prints one character per column, which is the form that
			// shows what the bytes spell rather than what they add up to.
			t.Errorf("od returned %q, want the bytes of the file", r.Text)
		}
	}
}
