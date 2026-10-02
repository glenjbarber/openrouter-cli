package tools

import (
	"os/exec"
	"strings"
	"testing"
)

// The scanners and generators a reader may permit a model to run. Each reads
// the tree and prints findings rather than changing it, so each belongs on the
// list on the same reasoning as go build and grep.
var scannerPrograms = []string{
	"errcheck",
	"gosec",
	"govulncheck",
	"protoc-gen-go",
	"protoc-gen-go-grpc",
	"staticcheck",
}

// TestShellCarriesTheScanners checks the whole group rather than one at a
// time, since a list that gained five of the six would pass a test naming any
// one of them, and the omission is the failure nobody would notice.
func TestShellCarriesTheScanners(t *testing.T) {
	for _, name := range scannerPrograms {
		if !shellPermittedMap[name] {
			t.Errorf("%s is not on the allowlist", name)
		}
		if !strings.Contains(permittedPrograms(), name) {
			t.Errorf("the refusal does not name %s", name)
		}
		if !strings.Contains(shellParameters, name) {
			t.Errorf("the schema the model is offered does not name %s", name)
		}
	}
}

// TestShellAScannerResolvesByNameAndIsAskedAbout runs one of the group where
// it is present, which proves two things at once: the name reaches the program
// rather than being refused, and the reader is asked before it runs.
//
// staticcheck is used rather than one of the generators because it is a
// scanner that runs without a project argument and prints its own version. The
// generators are named for the same list and reach the same check.
func TestShellAScannerResolvesByNameAndIsAskedAbout(t *testing.T) {
	if _, err := exec.LookPath("staticcheck"); err != nil {
		t.Skipf("staticcheck is not on the PATH: %v", err)
	}
	dir := t.TempDir()
	var asked string
	set := NewShell(dir, approveFunc(func(command string, _ []string, _ string) bool {
		asked = command
		return true
	}))

	r := shellCall(t, set, map[string]any{"command": "staticcheck", "args": []string{"-version"}})

	if r.Err != nil {
		t.Fatalf("staticcheck failed: %v", r.Err)
	}
	if asked != "staticcheck" {
		t.Errorf("the reader was asked about %q, want staticcheck", asked)
	}
}

// TestShellRefusesAScannerWhenTheReaderSaysNo checks the refusal reaches the
// model rather than the program running anyway, since a name on the list is a
// bound on what may be proposed rather than permission to run.
func TestShellRefusesAScannerWhenTheReaderSaysNo(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, approveFunc(func(string, []string, string) bool { return false }))

	r := shellCall(t, set, map[string]any{"command": "govulncheck", "args": []string{"./..."}})

	if r.Err == nil {
		t.Fatal("govulncheck ran without being approved")
	}
	if !strings.Contains(r.Err.Error(), "did not approve") {
		t.Errorf("the refusal did not say what was wrong: %v", r.Err)
	}
}
