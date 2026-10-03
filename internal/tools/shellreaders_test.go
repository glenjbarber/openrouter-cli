package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The readers added beside grep and ls. Each is checked the same way the ones
// before them are: the program has to be on the list, has to reach the program
// rather than be refused, and has to be named in both copies of the list, the
// one a refusal prints and the one the model is offered. The checks are for the
// bare name rather than for the punctuation around it, since a check written
// against a neighbours punctuation fails the next time anything is appended.

func TestShellNamesTheReadersBesideGrepAndLsInARefusal(t *testing.T) {
	for _, name := range []string{"rg", "stat", "file", "diff"} {
		if !shellPermittedMap[name] {
			t.Errorf("%s is not on the allowlist", name)
		}
		if !strings.Contains(permittedPrograms(), name) {
			t.Errorf("the refusal does not name %s: %q", name, permittedPrograms())
		}
		if !strings.Contains(shellParameters, name) {
			t.Errorf("the schema the model is offered does not name %s", name)
		}
	}
}

// TestShellRunsRg checks that rg reaches the program rather than being refused
// on its way to it, which is the whole difference between a name being on the
// list and a name being absent from it.
//
// rg is not on every host, so a host without it is a skip rather than a
// failure: the list is portable, and a program missing from a PATH is not a
// broken tool.
func TestShellRunsRg(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skipf("rg is not on the PATH: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("a line worth finding\n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{
		"command": "rg",
		"args":    []string{"worth", dir},
	})

	if r.Err != nil {
		t.Fatalf("rg failed: %v", r.Err)
	}
	// The matched line is what rg prints, so a result carrying the pattern
	// without the line around it is not a search that ran.
	if !strings.Contains(r.Text, "a line worth finding") {
		t.Errorf("rg returned %q, want the line that matched", r.Text)
	}
}

// TestShellRunsStatAndFile checks the two that answer what a path is, since a
// model that has just written a file needs to know whether it wrote what it
// meant to before it acts on the file.
//
// Neither is on every host, and file in particular is a port on some systems
// and a command on others, so each is skipped rather than failed when absent.
func TestShellRunsStatAndFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(name, []byte("content\n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	if _, err := exec.LookPath("stat"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "stat", "args": []string{name}})
		if r.Err != nil {
			t.Errorf("stat failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "note.txt") {
			t.Errorf("stat returned %q, want the file it was asked about", r.Text)
		}
	}

	if _, err := exec.LookPath("file"); err == nil {
		r := shellCall(t, set, map[string]any{"command": "file", "args": []string{name}})
		if r.Err != nil {
			t.Errorf("file failed: %v", r.Err)
		} else if !strings.Contains(r.Text, "note.txt") {
			t.Errorf("file returned %q, want the file it was asked about", r.Text)
		}
	}
}

// TestShellRunsDiff checks that diff reaches the program and that what it
// printed comes back, which is the one property here that needed the runner
// changed to be true.
//
// diff prints the differences on stdout and exits 1 when the files differ. That
// status is the answer rather than a fault, and a runner that reported only
// stderr would hand a model the words "exit status 1" and throw away the
// difference, which is the whole of what it asked for. The arrangement in run
// carries stdout into the failure for exactly this case.
func TestShellRunsDiff(t *testing.T) {
	if _, err := exec.LookPath("diff"); err != nil {
		t.Skipf("diff is not on the PATH: %v", err)
	}
	dir := t.TempDir()
	before := filepath.Join(dir, "before.txt")
	after := filepath.Join(dir, "after.txt")
	if err := os.WriteFile(before, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("writing the first file: %v", err)
	}
	if err := os.WriteFile(after, []byte("one\nthree\n"), 0o600); err != nil {
		t.Fatalf("writing the second file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "diff", "args": []string{before, after}})

	// The exit status is still a status: two files that differ is a report to
	// the model rather than a successful call, and the runner does not pretend
	// otherwise. What matters is that the difference travels with it.
	if !strings.Contains(r.Err.Error(), "two") || !strings.Contains(r.Err.Error(), "three") {
		t.Errorf("diff did not report what changed: %v", r.Err)
	}
}

// TestShellStillPrefersStderr checks that the change made for diff did not
// swallow a diagnostic. A program that fails and explains itself on stderr must
// have that explanation reported, since it names the cause and stdout usually
// does not.
func TestShellStillPrefersStderr(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	// cat on an absent file says why on stderr and prints nothing on stdout.
	r := shellCall(t, set, map[string]any{"command": "cat", "args": []string{filepath.Join(dir, "absent")}})

	if r.Err == nil {
		t.Fatal("a failing command reported no error")
	}
	if !strings.Contains(r.Err.Error(), "cat failed") {
		t.Errorf("the failure did not name the program: %v", r.Err)
	}
	// The diagnostic is carried, rather than the bare status a stdout fallback
	// would have left behind.
	if strings.Contains(r.Err.Error(), "exit status") {
		t.Errorf("the diagnostic on stderr was replaced by the exit status: %v", r.Err)
	}
}
