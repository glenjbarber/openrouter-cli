// Package saved writes a session to a local database and reads it back.
//
// A saved session is a conversation with the model alongside the figures that
// went with it: the model that was asked, the token counts, and the usage the
// backend reported. It is written to one file per save rather than to a shared
// database, so that a file can be handed to someone else, opened with any
// SQLite tool, or deleted without touching anything else.
//
// What is not written is as deliberate as what is. The credential is not: the
// configuration file is the only source of the key. The session preferences are
// not: those belong to the configuration file, and a session saved next to
// them would be a second place to keep them. An ephemeral thread is not, since
// a thread records nothing and so has nothing to write.
package saved

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Driver is the database/sql driver name the package opens.
const Driver = "sqlite"

// formatVersion is the schema this package writes.
//
// It is carried in every file so that a file written by a later version is
// reported rather than read as though it were this one, which is the same
// treatment the configuration file and the JSON bootstrap document get.
//
// Version 2 added a column to the message table, for the turns a tool call
// makes. It is one nullable column rather than a column per field, so a file
// stays readable as plain text in any SQLite tool, which is the reason the
// format is a database rather than a file of prose.
const formatVersion = "2"

// oldestReadable is the earliest schema this package reads.
//
// A file written before the column was added is still read, and a file written
// by a later version is still refused. The refusal is the important half: a
// schema this cannot interpret must be reported rather than half-read, since a
// conversation missing its tool turns reads as though the model never called
// anything.
const oldestReadable = "1"

// Errors reported by the package.
var (
	// ErrExists reports a file already sitting where the session would be
	// written. It is returned rather than overwritten, since a file named by
	// the reader is a file they asked for and clobbering it would destroy a
	// conversation they had not seen again.
	ErrExists = errors.New("a saved session is already at that path")

	// ErrBadName reports a name that cannot be used as a file name.
	ErrBadName = errors.New("unusable session name")

	// ErrNotASession reports a file that is not a saved session, which is a
	// SQLite database of something else or a file that is not one at all.
	ErrNotASession = errors.New("not a saved session")

	// ErrFuture reports a file written by a later version than this one reads.
	ErrFuture = errors.New("saved by a later version")

	// ErrUnsupported reports a platform this package cannot reach a database
	// on. It is a distinct error rather than a generic failure so that the
	// interface can say what is wrong instead of showing the reader a
	// database message from a machine that has no driver.
	ErrUnsupported = errors.New("saved sessions are not available on this platform")
)

// Session is what a saved file carries.
type Session struct {
	// Name is the label the session was saved under.
	Name string
	// SavedAt is when it was written, which is reported rather than inferred
	// from the file, since a copied file carries a modification time that
	// says nothing about when the conversation was had.
	SavedAt time.Time
	// Model is the model the conversation was carried by, empty when none was
	// ever chosen.
	Model string
	// TokensIn and TokensOut are the counts accumulated over the session.
	TokensIn  int
	TokensOut int
	// Usage is what the backend reported against the key during the session.
	Usage openrouter.Usage
	// Messages are the turns, in order, beginning with the opening
	// instructions where the session had them.
	Messages []openrouter.Message
}

// TurnCount returns how many questions the session holds, which is the figure
// a reader wants rather than a count of messages.
//
// A system turn is not counted. It is the opening instructions rather than an
// exchange, and reporting it as a turn would overstate the work by one.
func (s Session) TurnCount() int {
	n := 0
	for _, m := range s.Messages {
		if m.Role == openrouter.RoleUser {
			n++
		}
	}
	return n
}

// Dir returns the directory saved sessions are held in.
//
// It is a directory of its own rather than one under the configuration search
// path, since the two hold different things: the configuration file is
// written at setup and read thereafter, while a saved session is written
// whenever the reader asks for one. Keeping them apart means a reader who
// removes a configuration file does not lose conversations with it.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".openrouter-cli", "sessions"), nil
}

// Path returns the file a name refers to.
//
// A name is a single path element. Anything carrying a separator, or naming a
// directory, is refused rather than resolved, since a session name is a label
// the reader typed and a path is not what they typed.
func Path(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: the name is empty", ErrBadName)
	}
	// The extension is optional on the way in and always present on the way
	// out, so that /save work and /save work.db name one file rather than
	// two.
	name = strings.TrimSuffix(name, ".db")
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("%w: %s", ErrBadName, name)
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: %s names a path rather than a session", ErrBadName, name)
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".db"), nil
}

// Name returns the label a saved file carries under, taken from the file
// itself rather than from its path, so that a file that was renamed reports the
// name it was saved as.
func Name(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Suffixed returns a name carrying an epoch, used where a file already exists
// and the reader declined to replace it.
//
// The epoch is appended rather than a counter, since it cannot collide with
// itself and needs nothing remembered to stay unique.
func Suffixed(name string, epoch int64) string {
	return strings.TrimSuffix(name, ".db") + "-" + strconv.FormatInt(epoch, 10) + ".db"
}
