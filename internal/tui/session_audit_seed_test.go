package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/bootstrap"
	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// The bootstrap document is read once at startup to be validated, and the
// document that was validated is the one the session is seeded with. Reading it
// again let a file changed between the two reads seed a document nobody
// checked.
func TestSeedTakesTheDocumentRatherThanReadingItAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := os.WriteFile(path, []byte("Answer in the third person."), 0o600); err != nil {
		t.Fatalf("writing the document: %v", err)
	}
	doc, err := bootstrap.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The file is gone before the session is seeded. A Seed that read the
	// document for itself would fail here, and one that read it earlier would
	// carry text the file no longer holds.
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing the document: %v", err)
	}

	s := &Session{conv: NewConversation(), screen: &Screen{}}
	s.Seed(doc)

	pending := s.conv.Pending("")
	if len(pending) == 0 {
		t.Fatal("the seeded conversation carries no opening instructions")
	}
	if !strings.Contains(pending[0].Content, "third person") {
		t.Errorf("instructions = %q, want the document that was loaded", pending[0].Content)
	}
}

// A Seed given no document leaves the conversation as it was, since the session
// is built before the flag is known.
func TestSeedWithNoDocumentChangesNothing(t *testing.T) {
	s := &Session{conv: NewConversation(), screen: &Screen{}}
	s.Seed(nil)
	if got := len(s.conv.Pending("")); got != 1 {
		t.Errorf("the conversation carries %d turns, want the question and nothing else", got)
	}
}

// The diagnostic for an absent key named one path. The loader checks several,
// and a reader whose file is at the second was told to edit the first, which is
// a file the loader never read.
func TestTheAbsentKeyDiagnosticNamesWhereTheClientLooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	s := &Session{conv: NewConversation(), screen: &Screen{}}
	problem := s.credentialProblem()
	if !strings.Contains(problem, "no API key is configured") {
		t.Fatalf("problem = %q, want it to name the missing key", problem)
	}
	for _, path := range config.SearchPaths() {
		if !strings.Contains(problem, path) {
			t.Errorf("problem = %q, want it to name %q, which the loader checks", problem, path)
		}
	}
	if strings.Contains(problem, "~/") {
		t.Errorf("problem = %q, want no unexpanded path, since the reader cannot edit one", problem)
	}
}
