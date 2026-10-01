package saved

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// A turn that called a tool carries the call, and a turn that answered one
// carries the identifier it answers. Without both, a saved conversation of a
// turn that used tools cannot be put back together, and the model resumes
// having asked for something it was never shown the answer to.
func TestAToolCallSurvivesTheRoundTrip(t *testing.T) {
	call := openrouter.ToolCall{
		ID:   "call_abc",
		Type: openrouter.ToolTypeFunction,
	}
	call.Function = openrouter.ToolCallFunction{
		Name:      "read_file",
		Arguments: `{"path":"README.md"}`,
	}

	want := sample("tools")
	want.Messages = []openrouter.Message{
		{Role: openrouter.RoleUser, Content: "what does the readme say"},
		{Role: openrouter.RoleAssistant, ToolCalls: []openrouter.ToolCall{call}},
		{Role: openrouter.RoleTool, Content: "it says hello", ToolCallID: "call_abc"},
	}

	path := filepath.Join(t.TempDir(), "tools.db")
	if err := Write(path, want, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("the file holds %d turns, want 3: %+v", len(got.Messages), got.Messages)
	}
	if len(got.Messages[1].ToolCalls) != 1 {
		t.Fatalf("the assistant turn carries %d calls, want 1: %+v",
			len(got.Messages[1].ToolCalls), got.Messages[1])
	}
	back := got.Messages[1].ToolCalls[0]
	if back.ID != "call_abc" || back.Function.Name != "read_file" {
		t.Errorf("the call came back as %+v, want the one written", back)
	}
	if back.Function.Arguments != `{"path":"README.md"}` {
		t.Errorf("the arguments came back as %q, want the arguments written",
			back.Function.Arguments)
	}
	if got.Messages[2].ToolCallID != "call_abc" {
		t.Errorf("the answer carries %q, want the identifier it answers",
			got.Messages[2].ToolCallID)
	}
}

// The file is a database so that it can be read afterwards by any tool rather
// than only by this client. A turn carrying a call must still be readable as
// role and content in plain text, or the reason for the format is gone.
func TestATurnWithACallIsStillReadableAsPlainText(t *testing.T) {
	call := openrouter.ToolCall{ID: "call_1", Type: openrouter.ToolTypeFunction}
	call.Function = openrouter.ToolCallFunction{Name: "read_file", Arguments: `{}`}

	want := sample("plain")
	want.Messages = []openrouter.Message{
		{Role: openrouter.RoleUser, Content: "read the readme"},
		{Role: openrouter.RoleAssistant, ToolCalls: []openrouter.ToolCall{call}},
	}
	path := filepath.Join(t.TempDir(), "plain.db")
	if err := Write(path, want, false); err != nil {
		t.Fatalf("Write: %v", err)
	}

	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatalf("opening the file with the driver alone: %v", err)
	}
	defer db.Close()

	var role, content string
	if err := db.QueryRow(`SELECT role, content FROM messages WHERE seq = 1`).
		Scan(&role, &content); err != nil {
		t.Fatalf("reading the turn as plain columns: %v", err)
	}
	if role != openrouter.RoleAssistant {
		t.Errorf("role = %q, want the assistant", role)
	}
	if content != "" {
		t.Errorf("content = %q, want empty, since the turn carried a call and no text", content)
	}
}

// A turn that is ordinary prose stores no envelope at all, rather than an
// empty one. A reader distinguishing the two is what leaves room for a field
// added later without a migration of the files already written.
func TestAnOrdinaryTurnStoresNoEnvelope(t *testing.T) {
	want := sample("bare")
	path := filepath.Join(t.TempDir(), "bare.db")
	if err := Write(path, want, false); err != nil {
		t.Fatalf("Write: %v", err)
	}

	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatalf("opening the file with the driver alone: %v", err)
	}
	defer db.Close()

	var extra sql.NullString
	if err := db.QueryRow(`SELECT extra FROM messages WHERE seq = 0`).Scan(&extra); err != nil {
		t.Fatalf("reading the envelope column: %v", err)
	}
	if extra.Valid {
		t.Errorf("an ordinary turn stored the envelope %q, want none", extra.String)
	}
}

// A file written before the column existed is still read. A schema this can
// interpret is not a reason to refuse a conversation the reader saved.
func TestAFileWrittenBeforeTheColumnIsStillRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	if err := writeVersionOne(t, path); err != nil {
		t.Fatalf("writing a version one file: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("the file holds %d turns, want 2: %+v", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Content != "the question" || got.Messages[1].Content != "the answer" {
		t.Errorf("the turns came back as %+v, want the two that were written", got.Messages)
	}
	if len(got.Messages[1].ToolCalls) != 0 || got.Messages[1].ToolCallID != "" {
		t.Errorf("an ordinary turn came back carrying %+v, want nothing added to it",
			got.Messages[1])
	}
}

// A file from a later version is still refused, and the refusal is not softened
// by the fact that older files are accepted. A schema this cannot interpret
// must be reported rather than half-read.
func TestALaterVersionIsRefusedAndNamed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	if err := writeVersionOne(t, path); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatalf("opening the file: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE meta SET value = '9' WHERE key = 'version'`); err != nil {
		t.Fatalf("marking the file as a later version: %v", err)
	}
	db.Close()

	if _, err := Read(path); err == nil {
		t.Fatal("a file from a later version was read, want it refused")
	}
}

// writeVersionOne writes a file in the schema that predates the envelope
// column, which is three columns and version one in the metadata.
func writeVersionOne(t *testing.T, path string) error {
	t.Helper()
	db, err := sql.Open(Driver, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE messages (seq INTEGER PRIMARY KEY, role TEXT NOT NULL, content TEXT NOT NULL);
INSERT INTO meta (key, value) VALUES
    ('format','session'), ('version','1'), ('name','old'),
    ('saved_at','2026-01-02T03:04:05Z'), ('model','test/model'),
    ('tokens_in','0'), ('tokens_out','0'), ('usage','{}');
INSERT INTO messages (seq, role, content) VALUES
    (0, 'user', 'the question'),
    (1, 'assistant', 'the answer');`); err != nil {
		return err
	}
	return nil
}
