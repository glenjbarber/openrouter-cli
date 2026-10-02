package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellRunsJq checks that jq is on the list and reaches the program, since
// it is the reader for a JSON file in a repository that holds several and the
// rest of the list can only print one whole.
//
// jq is asked for by a model reading a response body or a saved session, and
// `jq .fields file.json` answers that without a program added for it. The
// arguments go to it as an array and no shell is read, so it cannot reach a
// pipe even though a filter is what it is named after.
func TestShellRunsJq(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skipf("jq is not on the PATH: %v", err)
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "doc.json")
	if err := os.WriteFile(name, []byte(`{"name":"openrouter-cli","version":1}`), 0o600); err != nil {
		t.Fatalf("writing the JSON: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{
		"command": "jq",
		"args":    []string{".name", name},
	})

	if r.Err != nil {
		t.Fatalf("jq failed: %v", r.Err)
	}
	// jq prints the value with its own quotes, so the name has to arrive
	// wrapped rather than bare. A result carrying the field name rather than
	// its value is not what the filter asked for.
	if !strings.Contains(r.Text, `"openrouter-cli"`) {
		t.Errorf("jq returned %q, want the value of the name field", r.Text)
	}
}

// TestShellNamesJqInARefusal checks that the list the refusal prints and the
// schema the model is offered both carry jq, since the prose is written out in
// each and either can fall behind the declaration.
//
// The schema is checked for the bare name rather than for the punctuation
// around it. A check written against a neighbours punctuation fails the next
// time anything is appended to the list, and that failure has already been
// made once: the check written for ps passed while ps was last in the list and
// broke when the scanners were added after it.
func TestShellNamesJqInARefusal(t *testing.T) {
	if !shellPermittedMap["jq"] {
		t.Error("jq is not on the allowlist")
	}
	if !strings.Contains(permittedPrograms(), "jq") {
		t.Errorf("the refusal names %q, want jq among them", permittedPrograms())
	}
	if !strings.Contains(shellParameters, "jq") {
		t.Error("the schema the model is offered does not name jq")
	}
}
