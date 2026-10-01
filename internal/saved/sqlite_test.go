//go:build !dragonfly

package saved

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// sample is a session carrying everything a save is supposed to carry.
func sample(name string) Session {
	return Session{
		Name:      name,
		SavedAt:   time.Date(2026, 10, 1, 15, 26, 0, 0, time.UTC),
		Model:     "vendor/model",
		TokensIn:  120,
		TokensOut: 340,
		Usage:     openrouter.Usage{Usage: 1.5, Limit: 100},
		Messages: []openrouter.Message{
			{Role: openrouter.RoleSystem, Content: "be brief"},
			{Role: openrouter.RoleUser, Content: "the question"},
			{Role: openrouter.RoleAssistant, Content: "the answer"},
		},
	}
}

// A saved session reads back as it was written, including the order of the
// turns, since an order lost would be a conversation turned inside out.
func TestAWriteReadsBackAsItWas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.db")
	want := sample("work")
	if err := Write(path, want, false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Name != want.Name || got.Model != want.Model {
		t.Errorf("name and model are %q and %q, want %q and %q",
			got.Name, got.Model, want.Name, want.Model)
	}
	if got.TokensIn != want.TokensIn || got.TokensOut != want.TokensOut {
		t.Errorf("tokens are %d in and %d out, want %d and %d",
			got.TokensIn, got.TokensOut, want.TokensIn, want.TokensOut)
	}
	if got.Usage.Usage != want.Usage.Usage || got.Usage.Limit != want.Usage.Limit {
		t.Errorf("usage is %+v, want %+v", got.Usage, want.Usage)
	}
	if !got.SavedAt.Equal(want.SavedAt) {
		t.Errorf("saved at %s, want %s", got.SavedAt, want.SavedAt)
	}
	if len(got.Messages) != len(want.Messages) {
		t.Fatalf("read %d messages, want %d", len(got.Messages), len(want.Messages))
	}
	for i := range want.Messages {
		if got.Messages[i] != want.Messages[i] {
			t.Errorf("message %d is %+v, want %+v", i, got.Messages[i], want.Messages[i])
		}
	}
}

// A save creates the file at a mode that keeps the conversation to the reader
// who wrote it.
func TestAWriteCreatesTheFileForItsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.db")
	if err := Write(path, sample("work"), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("the file is %s, want 0600", fi.Mode().Perm())
	}
}

// A second save under a name already taken is refused rather than silently
// replacing a conversation the reader may not have seen again.
func TestASecondSaveUnderTheSameNameIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.db")
	if err := Write(path, sample("first"), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	err := Write(path, sample("second"), false)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("Write returned %v, want ErrExists", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Name != "first" {
		t.Errorf("the file holds %q, want the first session left alone", got.Name)
	}
}

// An accepted overwrite replaces the file rather than adding to it, since a
// file carrying two conversations would read as one of them doubled.
func TestAnOverwriteReplacesWhatWasThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.db")
	if err := Write(path, sample("first"), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Write(path, sample("second"), true); err != nil {
		t.Fatalf("Write with overwrite: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Name != "second" {
		t.Errorf("the file holds %q, want the second session", got.Name)
	}
	if len(got.Messages) != 3 {
		t.Errorf("the file holds %d messages, want 3 rather than both sessions", len(got.Messages))
	}
}

// A file written by a later version is reported rather than read as though it
// were this one, which is what the configuration file and the JSON document do.
func TestAFileFromALaterVersionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.db")
	if err := Write(path, sample("work"), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE meta SET value = '2' WHERE key = 'version'`); err != nil {
		t.Fatalf("UPDATE: %v", err)
	}
	if _, err := Read(path); !errors.Is(err, ErrFuture) {
		t.Errorf("Read returned %v, want ErrFuture", err)
	}
}

// A file that is some other SQLite database is reported rather than read as an
// empty conversation, since an empty one loaded over a real one would look
// like a session the reader had.
func TestAFileThatIsNotASessionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE something (a TEXT)`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}
	db.Close()
	if _, err := Read(path); !errors.Is(err, ErrNotASession) {
		t.Errorf("Read returned %v, want ErrNotASession", err)
	}
}

// The directory is made as the save goes, so a reader who has never saved
// anything is not asked to make one first.
func TestAWriteMakesTheDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := Path("work")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := Write(path, sample("work"), false); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Stat: %v", err)
	}
}
