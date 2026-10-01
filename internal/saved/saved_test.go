package saved

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// A name is a label rather than a path, so one carrying a separator is
// refused rather than resolved.
func TestPathRefusesANameThatIsAPath(t *testing.T) {
	for _, name := range []string{"", "   ", "..", ".", "a/b", `a\b`} {
		if _, err := Path(name); err == nil {
			t.Errorf("Path(%q) succeeded, want it refused", name)
		}
	}
}

// The extension is optional going in and always present coming out, so that
// /save work and /save work.db name one file rather than two.
func TestPathAddsTheExtensionAndStripsTheOneGiven(t *testing.T) {
	t.Setenv("HOME", "/home/reader")
	a, err := Path("work")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	b, err := Path("work.db")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if a != b {
		t.Errorf("Path(work) = %s and Path(work.db) = %s, want the same file", a, b)
	}
	if got := filepath.Base(a); got != "work.db" {
		t.Errorf("the file is %s, want work.db", got)
	}
	if !strings.Contains(a, filepath.Join(".openrouter-cli", "sessions")) {
		t.Errorf("the file is %s, want it under .openrouter-cli/sessions", a)
	}
}

// A session is saved where a reader would look for it rather than beside the
// configuration file, so that removing one does not remove the other.
func TestDirIsItsOwnDirectory(t *testing.T) {
	t.Setenv("HOME", "/home/reader")
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if strings.Contains(dir, ".config") {
		t.Errorf("Dir = %s, want it out of the configuration path", dir)
	}
}

// A session carries an epoch when it is saved beside one already there, so
// that the earlier one survives without anything having to remember it.
func TestSuffixedAppendsTheEpoch(t *testing.T) {
	got := Suffixed("work.db", 1759341120)
	if want := "work-1759341120.db"; got != want {
		t.Errorf("Suffixed = %s, want %s", got, want)
	}
}

// The label is taken from the name the session was saved under rather than
// from the path, so a renamed file still reports what it is.
func TestNameComesFromTheFileName(t *testing.T) {
	if got := Name("/home/reader/.openrouter-cli/sessions/work.db"); got != "work" {
		t.Errorf("Name = %s, want work", got)
	}
}

// A question is a question. The opening instructions are not an exchange, and
// counting them would overstate the work in the session by one.
func TestTurnCountIgnoresTheOpeningInstructions(t *testing.T) {
	s := Session{Messages: []openrouter.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
		{Role: "user", Content: "three"},
	}}
	if got := s.TurnCount(); got != 2 {
		t.Errorf("TurnCount = %d, want 2", got)
	}
}
