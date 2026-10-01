package saved

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"

	// The driver is pure Go rather than one that compiles C, since the
	// cross-build target leaves cgo off and a driver that needs it produces a
	// binary that compiles for every platform and then fails at the first
	// query.
	_ "modernc.org/sqlite"
)

// Write saves the session to path.
//
// An existing file is refused unless overwrite is set. The file is created at
// mode 0600, since a conversation is the reader's own work and the loader for
// the configuration file refuses a permissive mode for the same reason.
func Write(path string, s Session, overwrite bool) error {
	if s.SavedAt.IsZero() {
		s.SavedAt = time.Now().UTC()
	}
	if err := claim(path, overwrite); err != nil {
		return err
	}
	db, err := sql.Open(Driver, path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer db.Close()

	if err := create(db); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := insert(db, s); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return db.Close()
}

// claim takes the file for writing before anything is put in it.
//
// The file is created exclusively rather than by the driver, so that the mode
// is ours to set and a file that appeared between the caller's check and this
// one is not destroyed. The driver would create it at the process umask, which
// is not a mode anyone chose.
func claim(path string, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if !overwrite {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("%w: %s", ErrExists, path)
			}
			return fmt.Errorf("creating %s: %w", path, err)
		}
		return f.Close()
	}
	// A replaced file is truncated rather than removed and recreated, so that
	// the path keeps whatever a link pointing at it resolves to.
	if err := os.Truncate(path, 0); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

// create makes the schema.
//
// The figures describing the session are held as key and value pairs rather
// than as columns, since they are read whole and never queried, and a table
// that grows a column per figure is a schema that has to be migrated to carry
// one that did not exist when it was written.
func create(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
    seq     INTEGER PRIMARY KEY,
    role    TEXT NOT NULL,
    content TEXT NOT NULL
);`)
	return err
}

// insert writes the session.
func insert(db *sql.DB, s Session) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	usage, err := json.Marshal(s.Usage)
	if err != nil {
		return fmt.Errorf("encoding the usage: %w", err)
	}
	fields := [][2]string{
		{"format", Driver},
		{"version", formatVersion},
		{"name", s.Name},
		{"saved_at", s.SavedAt.UTC().Format(time.RFC3339)},
		{"model", s.Model},
		{"tokens_in", strconv.Itoa(s.TokensIn)},
		{"tokens_out", strconv.Itoa(s.TokensOut)},
		{"usage", string(usage)},
	}
	for _, f := range fields {
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)`, f[0], f[1]); err != nil {
			return fmt.Errorf("writing %s: %w", f[0], err)
		}
	}
	for i, m := range s.Messages {
		if _, err := tx.Exec(`INSERT INTO messages (seq, role, content) VALUES (?, ?, ?)`,
			i, m.Role, m.Content); err != nil {
			return fmt.Errorf("writing message %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// Read returns the session held in the file at path.
//
// A file that is not a saved session is reported rather than read as an empty
// one, since an empty session loaded over a real one would look like a
// conversation the reader had.
func Read(path string) (*Session, error) {
	// The file is looked for before the driver opens it. The driver reports a
	// missing file in its own words, which says nothing about whether the
	// reader named something that is not there.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	db, err := sql.Open(Driver, path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer db.Close()

	// The schema is looked for before it is read from. A database of something
	// else has no metadata table, and the SQLite error for that would reach
	// the reader as though the file were ours and merely broken.
	var tables int
	if err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('meta', 'messages')`,
	).Scan(&tables); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if tables != 2 {
		return nil, fmt.Errorf("%w: %s is not one this wrote", ErrNotASession, path)
	}

	version, err := field(db, "version")
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if version == "" {
		return nil, fmt.Errorf("%w: %s carries no version", ErrNotASession, path)
	}
	if version != formatVersion {
		return nil, fmt.Errorf("%w: %s is version %s, and this reads version %s",
			ErrFuture, path, version, formatVersion)
	}

	out := &Session{}
	if out.Name, err = field(db, "name"); err != nil {
		return nil, err
	}
	if out.Model, err = field(db, "model"); err != nil {
		return nil, err
	}
	savedAt, err := field(db, "saved_at")
	if err != nil {
		return nil, err
	}
	if out.SavedAt, err = time.Parse(time.RFC3339, savedAt); err != nil {
		return nil, fmt.Errorf("reading %s: saved_at is %q, which is not a time: %w", path, savedAt, err)
	}
	if out.TokensIn, err = count(db, "tokens_in"); err != nil {
		return nil, err
	}
	if out.TokensOut, err = count(db, "tokens_out"); err != nil {
		return nil, err
	}
	usage, err := field(db, "usage")
	if err != nil {
		return nil, err
	}
	if usage != "" {
		if err := json.Unmarshal([]byte(usage), &out.Usage); err != nil {
			return nil, fmt.Errorf("reading %s: the usage is not readable: %w", path, err)
		}
	}
	if out.Messages, err = turns(db); err != nil {
		return nil, err
	}
	return out, nil
}

// field returns one value from the metadata table.
func field(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", key, err)
	}
	return v, nil
}

// count returns one numeric value from the metadata table, absent counting as
// zero rather than as an error.
func count(db *sql.DB, key string) (int, error) {
	v, err := field(db, key)
	if err != nil || v == "" {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %q is not a count: %w", key, v, err)
	}
	return n, nil
}

// turns returns the messages in the order they were written.
//
// The order is the sequence the messages were recorded in rather than the row
// identifiers, so that a file written by a later version, or one that has had
// rows deleted from it, still reads back in the order it was written in.
func turns(db *sql.DB) ([]openrouter.Message, error) {
	rows, err := db.Query(`SELECT role, content FROM messages ORDER BY seq`)
	if err != nil {
		return nil, fmt.Errorf("reading the messages: %w", err)
	}
	defer rows.Close()

	var out []openrouter.Message
	for rows.Next() {
		var m openrouter.Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, fmt.Errorf("reading the messages: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
