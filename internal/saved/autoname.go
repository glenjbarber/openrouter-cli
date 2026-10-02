package saved

import (
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// An autosave is named for who wrote it, where it was written, and when, so
// that a session left running in one project does not overwrite the save of
// another and a reader browsing the directory can tell them apart without
// opening any of them.
//
// The name is a filename, so three things cannot go in it as written: a path
// separator, a character outside ASCII, and a space. Each is replaced rather
// than stripped, since stripping two directories that differ only in a
// non-ASCII character would have two saves with one name.

// Escape is the character a replaced one is written as.
//
// It is a percent rather than something safer because a filename is read by a
// person looking at a directory listing, and % is what such a name is expected
// to look like. A literal percent in the input is doubled, so the encoding can
// be read back: a single % in a name means it was written by the encoder and a
// doubled one means the input carried a percent.
const Escape byte = '%'

// maxAutoName is the longest name an autosave is given before the path it
// encodes is shortened from the front.
//
// The limit is on the filename rather than on the path, since the path is
// short and the encoded directory is what grows. A long path is shortened at
// its front rather than its end, since the end of a path is the directory the
// session was opened in and that is the part a reader recognises.
const maxAutoName = 120

// EncodeAutoKey renders a working directory as a filename fragment.
//
// The substitutions are made in the order that keeps the encoding readable: a
// literal escape is doubled first, so that a percent in the input cannot be
// read as a replacement, and the separator is replaced after it for the same
// reason. A path already carrying a percent therefore encodes to one carrying
// two, and the two are told apart.
//
// Non-ASCII is written as the escape followed by the bytes of the rune in
// upper-case hex, two digits each. The bytes are what make it reversible: a
// rune outside the first 256 needs more than one, and stopping at the rune
// itself would give a different length for each.
//
// A space is escaped as well. A filename with a space in it has to be quoted
// at every shell that touches it, and an autosave is written without being
// named by hand.
func EncodeAutoKey(dir string) string {
	var b strings.Builder
	b.Grow(len(dir) + 8)
	for _, r := range dir {
		switch {
		case r == rune(Escape):
			b.WriteString(string([]byte{Escape, Escape}))
		case r == '/' || r == '\\':
			b.WriteByte(Escape)
		case r <= ' ' || r == 0x7f:
			// Control characters and the space. A newline or a tab in a
			// filename is read as a line break or a column in a listing,
			// which is a name that cannot be typed back.
			b.WriteString(escapeRune(r))
		case r > unicode.MaxASCII:
			b.WriteString(escapeRune(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeRune writes one rune as the escape followed by its bytes in hex.
func escapeRune(r rune) string {
	// A rune above the first 256 does not fit in a byte, and is written as
	// its code point rather than as its UTF-8 bytes, so that the name stays
	// a fixed width per character rather than growing with the encoding.
	return fmt.Sprintf("%c%04X", Escape, r)
}

// shortenAutoKey shortens an encoded path to fit a filename, from the front.
//
// The tail is kept rather than the head, since the tail of a path is the
// directory the session ran in and that is what a reader recognises. The
// shortened name is marked with an escape so that it cannot be mistaken for an
// encoding of the whole path.
func shortenAutoKey(key string) string {
	if len(key) <= maxAutoName {
		return key
	}
	tail := key[len(key)-(maxAutoName-1):]
	// A cut can land inside an escape sequence, leaving a name that ends in a
	// half-written rune. The tail is walked forward past any trailing escape
	// and the digits that follow it, so the name is never cut mid-sequence.
	// What is dropped is a few characters of a long path, which is cheaper
	// than a name that decodes to nothing.
	for i := len(tail) - 1; i >= 0; i-- {
		if tail[i] != Escape {
			break
		}
		// An odd run of trailing escapes is a sequence the cut fell inside,
		// since a written one is always doubled or followed by hex digits.
		tail = tail[1:]
	}
	return string(Escape) + tail
}

// AutoName returns the name an autosave is written under.
//
// The three parts are separated by an escape that cannot appear inside any of
// them, since every other one has been doubled. A reader can therefore see
// where the name is split, and the session can read its own parts back.
//
// The user is taken from the environment rather than from the account
// database, since a name built from a userid lookup is a name that differs
// between hosts and a reader browsing another machine's directory would find
// saves they cannot place.
func AutoName(dir string, at time.Time) string {
	who := currentUser()
	key := shortenAutoKey(EncodeAutoKey(dir))
	// The escape is written through a string rather than through the format,
	// since it is a byte and the verb would not take one.
	e := string(Escape)
	return fmt.Sprintf("%s%s%s%s%03d", e, who, e, key, at.Unix()%1000)
}

// AutoLinkName returns the name of the link pointing at the newest autosave for
// a directory.
//
// It carries the same key as the files it points at, so that the link for one
// directory cannot be mistaken for the link for another.
func AutoLinkName(dir string) string {
	return fmt.Sprintf("%slatest", shortenAutoKey(EncodeAutoKey(dir)))
}

// currentUser is the name of the account the client is running as.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		// A domain-qualified name on Windows carries a separator of its own,
		// and the name is escaped rather than trimmed.
		return EncodeAutoKey(u.Username)
	}
	if name := osGetenv("USER"); name != "" {
		return EncodeAutoKey(name)
	}
	if name := osGetenv("LOGNAME"); name != "" {
		return EncodeAutoKey(name)
	}
	return "unknown"
}

// AutoDir is the directory an autosave is written to, which is the same one
// /save uses.
func AutoDir() (string, error) { return Dir() }

// AutoPath returns where an autosave for dir at the given moment is written.
//
// It is derived from Dir rather than written here, so that an autosave lands
// beside the saves a reader made by hand and neither has to know where the
// other keeps them.
func AutoPath(dir string, at time.Time) (string, error) {
	base, err := AutoDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AutoName(dir, at)+".db"), nil
}
