package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellRunsDuAndDf checks that both reach the program rather than being
// refused on the way to it, which is what distinguishes a name being on the
// list from a name being absent from it.
//
// du is pointed at a directory the test made, so the answer is about something
// known rather than about whatever happens to be on the host. df is left with
// no arguments, since it reports on the filesystems it is given and there is
// nothing particular to ask about one of them here.
func TestShellRunsDuAndDf(t *testing.T) {
	dir := t.TempDir()
	// du reports nothing of its own for a directory holding no content, so the
	// directory is given a name and a file worth it.
	sub := filepath.Join(dir, "build")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("making the directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "artifact"), []byte("something\n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	if _, err := exec.LookPath("du"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "du", "args": []string{"-s", sub}})
		if r.Err != nil {
			t.Errorf("du failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "build") {
			t.Errorf("du returned %q, want the directory it was asked about", r.Text)
		}
	}

	if _, err := exec.LookPath("df"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "df"})
		if r.Err != nil {
			t.Errorf("df failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "Filesystem") {
			// df names its first column Filesystem, which is what every
			// platform it runs on spells it, and it is the one part of the
			// output that is the same wherever it came from.
			t.Errorf("df returned %q, want a filesystem listing", r.Text)
		}
	}
}
