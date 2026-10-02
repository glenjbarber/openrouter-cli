package saved

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The name of an autosave has to be a filename, has to be readable, and has to
// tell two directories apart. These check the three.

func TestASeparatorIsEscaped(t *testing.T) {
	got := EncodeAutoKey("/home/gjb/work")
	if strings.ContainsRune(got, '/') {
		t.Errorf("the encoded key holds a separator: %q", got)
	}
	if got != "%home%gjb%work" {
		t.Errorf("encoded %q, want %q", got, "%home%gjb%work")
	}
}

func TestALiteralEscapeIsDoubled(t *testing.T) {
	// Without this, a path already carrying a percent would encode to the
	// same name as one where the percent was a separator.
	a := EncodeAutoKey("/home/gjb/a%b")
	b := EncodeAutoKey("/home/gjb/ab")
	if a == b {
		t.Errorf("two different paths encoded to the same name: %q", a)
	}
	if !strings.Contains(a, "%%") {
		t.Errorf("a literal escape was not doubled: %q", a)
	}
}

func TestNonASCIIIsEscapedAsHex(t *testing.T) {
	// The bytes rather than the character, since a terminal and a filename
	// disagree about what a rune outside the first page is.
	got := EncodeAutoKey("/home/gjb/naïve")
	if strings.ContainsAny(got, "ï") {
		t.Errorf("the key still holds the character: %q", got)
	}
	if !strings.Contains(got, "%") {
		t.Errorf("the key was not escaped: %q", got)
	}
}

func TestTwoNonASCIIDirectoriesDoNotCollide(t *testing.T) {
	// The failure a replacement without hex would give, since both would be
	// written as a single escape.
	a := EncodeAutoKey("/home/naïve")
	b := EncodeAutoKey("/home/naïve")
	if a != b {
		t.Error("the same path encoded two ways")
	}
	c := EncodeAutoKey("/home/naive")
	if a == c {
		t.Errorf("a non-ASCII path collided with the ASCII one: %q", a)
	}
}

func TestASpaceIsEscaped(t *testing.T) {
	// A filename with a space in it has to be quoted at every shell that
	// touches it, and an autosave is written without being named.
	got := EncodeAutoKey("/home/gjb/my project")
	if strings.ContainsRune(got, ' ') {
		t.Errorf("the key holds a space: %q", got)
	}
}

func TestANameIsSafeAsAFilename(t *testing.T) {
	for _, dir := range []string{
		"/home/gjb/openrouter-cli",
		"/home/gjb/my project",
		"/home/gjb/naïve",
		"/home/gjb/日本語のディレクトリ",
		"/home/gjb/a%b",
		"relative/path",
		"",
	} {
		name := AutoName(dir, time.Unix(1700000000, 0))
		if strings.ContainsAny(name, "/\x00") {
			t.Errorf("%q encoded to a name holding a separator: %q", dir, name)
		}
		if name == "" {
			t.Errorf("%q encoded to nothing", dir)
		}
	}
}

func TestTwoDirectoriesGetDifferentNames(t *testing.T) {
	at := time.Unix(1700000000, 0)
	a := AutoName("/home/gjb/one", at)
	b := AutoName("/home/gjb/two", at)

	if a == b {
		t.Errorf("two directories named the same file: %q", a)
	}
}

func TestTheNameCarriesTheMoment(t *testing.T) {
	// Two saves a second apart are told apart, so a burst of saves in one
	// session does not overwrite itself.
	first := AutoName("/home/gjb/work", time.Unix(1700000000, 0))
	second := AutoName("/home/gjb/work", time.Unix(1700000001, 0))

	if first == second {
		t.Errorf("two moments named the same file: %q", first)
	}
}

func TestALongPathIsShortenedAndMarked(t *testing.T) {
	long := "/home/gjb/" + strings.Repeat("directory/", 30)
	got := AutoName(long, time.Unix(1700000000, 0))

	if len(got) > maxAutoName+8 {
		t.Errorf("the name is %d characters, over the limit", len(got))
	}
	// A shortened name is marked, so it cannot be read as a whole path.
	if !strings.HasPrefix(got, string(Escape)) {
		t.Errorf("the shortened name is not marked: %q", got)
	}
}

func TestTheLinkNameIsPerDirectory(t *testing.T) {
	a := AutoLinkName("/home/gjb/one")
	b := AutoLinkName("/home/gjb/two")

	if a == b {
		t.Errorf("two directories share a link name: %q", a)
	}
	if !strings.HasSuffix(a, "latest") {
		t.Errorf("the link name is %q, want it to end in latest", a)
	}
}

func TestTheAutoPathLandsBesideTheHandMadeSaves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := AutoPath("/home/gjb/work", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("AutoPath: %v", err)
	}

	wantDir := filepath.Join(home, ".openrouter-cli", "sessions")
	if filepath.Dir(got) != wantDir {
		t.Errorf("the autosave went to %s, want %s", filepath.Dir(got), wantDir)
	}
	if !strings.HasSuffix(got, ".db") {
		t.Errorf("the autosave is %q, want a .db", got)
	}
}

// The name has to survive a filesystem, which is the only place it is ever used.
func TestTheNameIsAcceptedByTheFilesystem(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"/home/gjb/naïve", "/home/gjb/my project", "/home/gjb/日本語"} {
		name := AutoName(p, time.Unix(1700000000, 0)) + ".db"
		full := filepath.Join(dir, name)
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Errorf("%q became %q, which the filesystem refused: %v", p, name, err)
		}
		if _, err := os.Stat(full); err != nil {
			t.Errorf("the file did not survive: %v", err)
		}
	}
}

// A caller asking to overwrite is asking to write there, not to be refused for
// the absence of the thing they offered to replace. Truncating a path with
// nothing at it fails, and every autosave names a file that is not there yet.
func TestOverwritingAFileThatIsNotThereWritesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")

	if err := Write(path, Session{Name: "first", Messages: []openrouter.Message{{Role: "user"}}}, true); err != nil {
		t.Fatalf("writing to a path with nothing at it: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file is not there: %v", err)
	}
}

// A link pointing at the file keeps resolving after an overwrite, which is what
// the autosave newest link depends on.
func TestOverwritingKeepsThePathResolving(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.db")
	link := filepath.Join(dir, "newest.db")

	if err := Write(path, Session{Name: "first"}, true); err != nil {
		t.Fatalf("the first write: %v", err)
	}
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("making the link: %v", err)
	}
	if err := Write(path, Session{Name: "second"}, true); err != nil {
		t.Fatalf("the second write: %v", err)
	}
	if _, err := os.Stat(link); err != nil {
		t.Errorf("the link no longer resolves: %v", err)
	}
}
