package tools

import (
	"os/exec"
	"strings"
	"testing"
)

// TestShellRunsPs checks that ps is on the list and reaches the program, since
// it is a read of what the machine is doing and a model diagnosing a build that
// will not finish asks for it.
//
// ps without arguments prints once and exits. That is the reason it is on the
// list and top is not: top is interactive unless it is given -b, so a model
// asking for it would get a screen of escape sequences rather than a process
// listing, and a worker would sit on it until its deadline.
func TestShellRunsPs(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skipf("ps is not on the PATH: %v", err)
	}
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "ps"})

	if r.Err != nil {
		t.Fatalf("ps failed: %v", r.Err)
	}
	// The header line carries the column names, so a program listing with no
	// header is not what ps printed.
	if !strings.Contains(r.Text, "PID") {
		t.Errorf("ps returned %q, want a process listing", r.Text)
	}
}

// TestShellNamesPsInARefusal checks that the list the refusal prints and the
// schema the model is offered both carry ps, since the prose is written out in
// each and either can fall behind the declaration.
//
// The schema is checked for the bare name rather than for the name and the
// punctuation around it. The name was last in the list when it was added, so
// the check could be written against ", ps." and pass, and the scanners added
// after it moved it off the end without the check noticing. A check written
// against a neighbours punctuation fails the next time anything is appended to
// the list, and a list that fails a test every time a program is added is a
// list that gets the assertion relaxed rather than the failure fixed.
func TestShellNamesPsInARefusal(t *testing.T) {
	if !shellPermittedMap["ps"] {
		t.Error("ps is not on the allowlist")
	}
	if !strings.Contains(permittedPrograms(), "ps") {
		t.Errorf("the refusal names %q, want ps among them", permittedPrograms())
	}
	if !strings.Contains(shellParameters, "ps") {
		t.Error("the schema the model is offered does not name ps")
	}
}

// TestShellStillRefusesTop checks the decision recorded above: top is left off
// rather than added with a check that would require it. A program on the list
// a model cannot use usefully is worse than one it is refused by name, since
// the refusal says what is available and the interactive form returns a screen
// of escape sequences.
func TestShellStillRefusesTop(t *testing.T) {
	dir := t.TempDir()
	asked := false
	set := NewShell(dir, approveFunc(func(string, []string, string) bool {
		asked = true
		return true
	}))

	r := shellCall(t, set, map[string]any{"command": "top", "args": []string{"-b"}})

	if r.Err == nil {
		t.Fatal("top was not refused")
	}
	if !strings.Contains(r.Err.Error(), "not permitted") {
		t.Errorf("the refusal did not say what was wrong: %v", r.Err)
	}
	if asked {
		t.Error("the reader was asked about a program that would have been refused anyway")
	}
}
