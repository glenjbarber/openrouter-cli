package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// writeSession saves a conversation to a path and returns it.
func writeSession(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "work.db")
	sess := saved.Session{
		Name:    "work",
		SavedAt: time.Date(2026, 10, 1, 15, 26, 0, 0, time.UTC),
		Model:   "vendor/model",
		Messages: []openrouter.Message{
			{Role: openrouter.RoleSystem, Content: "be brief"},
			{Role: openrouter.RoleUser, Content: "the question"},
			{Role: openrouter.RoleAssistant, Content: "the answer"},
		},
	}
	if err := saved.Write(path, sess, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return path
}

// A saved conversation can be handed to a session at startup, which is what
// makes the file worth more than an export.
func TestASavedSessionLoadsAsABootstrapDocument(t *testing.T) {
	doc, err := Load(writeSession(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != FormatSession {
		t.Errorf("Format = %s, want %s", doc.Format, FormatSession)
	}
	if doc.Session == nil {
		t.Fatal("the document carries no session")
	}
	if doc.Name != "work" {
		t.Errorf("Name = %q, want the name the session was saved under", doc.Name)
	}
	if len(doc.Session.Messages) != 3 {
		t.Errorf("the session holds %d messages, want the three it was saved with", len(doc.Session.Messages))
	}
	if doc.Session.Model != "vendor/model" {
		t.Errorf("Model = %q, want vendor/model", doc.Session.Model)
	}
}

// The format is selected by the extension alone, since a database cannot be
// probed as text and a guess that is wrong would be worse than a refusal.
func TestTheExtensionSelectsTheSessionFormat(t *testing.T) {
	f, err := formatFor("/home/reader/work.db")
	if err != nil {
		t.Fatalf("formatFor: %v", err)
	}
	if f != FormatSession {
		t.Errorf("formatFor = %s, want %s", f, FormatSession)
	}
}

// A session with nothing in it would begin a session with nothing in it, which
// is the silent failure an empty document is refused for.
func TestASessionWithNothingInItIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	if err := saved.Write(path, saved.Session{Name: "empty"}, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load succeeded, want the empty session refused")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Load said %v, want it to say the document was empty", err)
	}
}

// A file that is some other database is reported rather than begun a session
// from.
func TestAFileThatIsNotASessionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	if err := os.WriteFile(path, []byte("this is not a database at all"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded, want it refused")
	}
}
