package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NewFilesystemAtOrSkip returns a filesystem set over a directory holding a
// file and a subdirectory, for the tests about the registry and the schemas
// rather than about one tool.
func NewFilesystemAtOrSkip(t *testing.T) *Set {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	s, err := NewFilesystemAt(dir)
	if err != nil {
		t.Fatalf("opening a root over %s: %v", dir, err)
	}
	return s
}

// fixture returns a filesystem set over a directory with a file, a
// subdirectory, and a symlink pointing out of the tree.
//
// The symlink is the shape a prefix check on a cleaned path does not catch: the
// name of it is inside the tree and what it names is not.
func fixture(t *testing.T) (*Set, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	s, err := NewFilesystemAt(dir)
	if err != nil {
		t.Fatalf("opening a root over %s: %v", dir, err)
	}
	return s, dir
}

// linkOut returns a symlink inside dir pointing at a file outside it, and the
// path of that file.
func linkOut(t *testing.T, dir string) (name, outside string) {
	t.Helper()
	outsideDir := t.TempDir()
	outside = filepath.Join(outsideDir, "secret")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	name = filepath.Join(dir, "link")
	if err := os.Symlink(outside, name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return name, outside
}

// TestReadFileReturnsTheContent checks the ordinary read, since a tool that
// could not do that would be refused on the same grounds as one that could not
// be contained.
func TestReadFileReturnsTheContent(t *testing.T) {
	s, _ := fixture(t)

	if got := mustText(t, call(t, s, readFileTool, `{"path":"file"}`)); got != "content\n" {
		t.Errorf("the read returned %q, want %q", got, "content\n")
	}
}

// TestReadFileOnAMissingFileNamesIt checks that a read of nothing is an error
// naming the file rather than an empty success, since a model cannot tell an
// empty file from one that is not there.
func TestReadFileOnAMissingFileNamesIt(t *testing.T) {
	s, _ := fixture(t)

	err := mustFail(t, call(t, s, readFileTool, `{"path":"no-such-file"}`))
	if !strings.Contains(err.Error(), "no-such-file") {
		t.Errorf("the refusal did not name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot read") {
		t.Errorf("the refusal did not say what was being attempted: %v", err)
	}
}

// TestReadFileOnADirectorySaysSo checks that a directory is reported as one
// rather than read, since the read of one fails with a message naming a
// system call the model has no use for.
func TestReadFileOnADirectorySaysSo(t *testing.T) {
	s, _ := fixture(t)

	err := mustFail(t, call(t, s, readFileTool, `{"path":"sub"}`))
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("the refusal did not say the path is a directory: %v", err)
	}
}

// TestReadFileRefusesAPathLeavingTheRoot checks the four shapes an escape
// arrives in. A refusal reported as the system error beneath it reads as a
// permission problem on the file rather than as a path the model chose
// wrongly, and the second is the one it can act on.
func TestReadFileRefusesAPathLeavingTheRoot(t *testing.T) {
	s, dir := fixture(t)
	link, outside := linkOut(t, dir)

	for _, path := range []string{
		"../x",
		"sub/../../x",
		"/etc/hosts",
		link,
	} {
		err := mustFail(t, call(t, s, readFileTool, `{"path":`+quote(path)+`}`))
		if !strings.Contains(err.Error(), "outside the root") {
			t.Errorf("reading %q was refused without naming the refusal: %v", path, err)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("the refusal for %q did not name the path: %v", path, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the file outside the tree was disturbed: %v", err)
	}
}

// TestReadFileRefusesALargeFile checks the limit, since a file too large for
// one message is refused with the figure rather than read, and the refusal is
// what tells a model to ask for something smaller.
func TestReadFileRefusesALargeFile(t *testing.T) {
	s, dir := fixture(t)

	big := filepath.Join(dir, "big")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	chunk := make([]byte, 64*1024)
	for written := 0; written <= MaxRead; written += len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}
	f.Close()

	err = mustFail(t, call(t, s, readFileTool, `{"path":"big"}`))
	if !strings.Contains(err.Error(), "1048576") {
		t.Errorf("the refusal did not state the limit: %v", err)
	}
}

// TestReadFileReturnsAFileAtTheLimit checks the boundary, since a limit that
// refused the file exactly on it would be a limit read as one smaller than it
// is.
func TestReadFileReturnsAFileAtTheLimit(t *testing.T) {
	s, dir := fixture(t)

	body := strings.Repeat("x", MaxRead)
	if err := os.WriteFile(filepath.Join(dir, "exact"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	if got := mustText(t, call(t, s, readFileTool, `{"path":"exact"}`)); len(got) != MaxRead {
		t.Errorf("the read returned %d bytes, want %d", len(got), MaxRead)
	}
}

// TestWriteFileWritesInsideTheRoot checks the ordinary write, and that the
// parent directories are made, since a model writing a file two levels down
// should not have to create the directory first.
func TestWriteFileWritesInsideTheRoot(t *testing.T) {
	s, dir := fixture(t)

	text := mustText(t, call(t, s, writeFileTool,
		`{"path":"made/deep/file","content":"written\n"}`))
	if !strings.Contains(text, "made/deep/file") || !strings.Contains(text, "8") {
		t.Errorf("the report was %q, which names neither the path nor the size", text)
	}
	got, err := os.ReadFile(filepath.Join(dir, "made", "deep", "file"))
	if err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	if string(got) != "written\n" {
		t.Errorf("the file holds %q, want %q", got, "written\n")
	}
}

// TestWriteFileReplacesTheWholeFile checks that a second write leaves no tail
// of the first, which is what the description offers a model.
func TestWriteFileReplacesTheWholeFile(t *testing.T) {
	s, _ := fixture(t)

	mustText(t, call(t, s, writeFileTool, `{"path":"file","content":"a long first content"}`))
	mustText(t, call(t, s, writeFileTool, `{"path":"file","content":"second"}`))

	if got := mustText(t, call(t, s, readFileTool, `{"path":"file"}`)); got != "second" {
		t.Errorf("the file holds %q, want %q", got, "second")
	}
}

// TestWriteFileRefusesAPathLeavingTheRoot checks that writing is contained the
// way reading is, including the parent directories: a check on the file alone
// would create the directories before it ran.
func TestWriteFileRefusesAPathLeavingTheRoot(t *testing.T) {
	s, dir := fixture(t)
	outsideDir := t.TempDir()

	for _, path := range []string{
		"../x",
		"sub/../../x",
		"/tmp/openrouter-tools-should-not-exist",
	} {
		err := mustFail(t, call(t, s, writeFileTool,
			`{"path":`+quote(path)+`,"content":"x"}`))
		if !strings.Contains(err.Error(), "outside the root") {
			t.Errorf("writing %q was refused without naming the refusal: %v", path, err)
		}
	}
	// The parent directory of a relative escape is where a mkdir would have
	// gone, so it is checked as well as the file.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "x")); err == nil {
		t.Error("a directory outside the root was created")
	}
	if _, err := os.Stat("/tmp/openrouter-tools-should-not-exist"); err == nil {
		t.Error("a file outside the root was created")
	}
	entries, err := os.ReadDir(outsideDir)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the directory outside the root holds %v", entries)
	}
}

// TestWriteFileRefusesASymlinkOutOfTheTree checks the shape a prefix check on a
// cleaned path does not catch.
func TestWriteFileRefusesASymlinkOutOfTheTree(t *testing.T) {
	s, dir := fixture(t)
	link, outside := linkOut(t, dir)

	err := mustFail(t, call(t, s, writeFileTool,
		`{"path":`+quote(link)+`,"content":"overwritten"}`))
	if !strings.Contains(err.Error(), "outside the root") {
		t.Errorf("writing through the symlink was refused without naming the refusal: %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("reading the file outside the tree: %v", err)
	}
	if string(got) != "outside\n" {
		t.Errorf("the file outside the tree holds %q, want it untouched", got)
	}
}

// TestWriteFileNeedsBothArguments checks the two required arguments by name,
// since a model that has filled in one of them is told the other rather than
// shown a decode error.
func TestWriteFileNeedsBothArguments(t *testing.T) {
	s, _ := fixture(t)

	err := mustFail(t, call(t, s, writeFileTool, `{"path":"file"}`))
	if !strings.Contains(err.Error(), `"content"`) {
		t.Errorf("the refusal did not name the missing argument: %v", err)
	}
	err = mustFail(t, call(t, s, writeFileTool, `{"content":"x"}`))
	if !strings.Contains(err.Error(), `"path"`) {
		t.Errorf("the refusal did not name the missing argument: %v", err)
	}
}

// TestListDirPutsDirectoriesFirst checks the order and the marker, since the
// listing is what a model reads to choose what to read next.
func TestListDirPutsDirectoriesFirst(t *testing.T) {
	s, dir := fixture(t)
	for _, name := range []string{"alpha", "zulu"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}
	for _, name := range []string{"beta", "alpha-dir"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}

	got := mustText(t, call(t, s, listDirTool, `{}`))
	want := "alpha-dir/\nbeta/\nsub/\nalpha\nfile\nzulu"
	if got != want {
		t.Errorf("the listing was:\n%s\nwant:\n%s", got, want)
	}
}

// TestListDirDefaultsToTheRoot checks that no path at all lists the working
// directory, since a model asking what is here should not have to name it.
func TestListDirDefaultsToTheRoot(t *testing.T) {
	s, _ := fixture(t)

	if got := mustText(t, call(t, s, listDirTool, `{"path":"."}`)); !strings.Contains(got, "file") {
		t.Errorf("the listing was %q, which does not hold the fixture", got)
	}
}

// TestListDirOnASubdirectory checks that a path inside the tree is read, since
// the containment refuses the escapes and nothing else.
func TestListDirOnASubdirectory(t *testing.T) {
	s, dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, "sub", "inside"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	if got := mustText(t, call(t, s, listDirTool, `{"path":"sub"}`)); got != "inside" {
		t.Errorf("the listing was %q, want %q", got, "inside")
	}
}

// TestListDirRefusesAPathLeavingTheRoot checks the same four shapes the read
// refuses, since a listing of a directory outside the tree is as much a
// disclosure as a read of a file in it.
func TestListDirRefusesAPathLeavingTheRoot(t *testing.T) {
	s, dir := fixture(t)
	link, _ := linkOut(t, dir)

	for _, path := range []string{"../", "sub/../..", "/tmp", link} {
		err := mustFail(t, call(t, s, listDirTool, `{"path":`+quote(path)+`}`))
		if !strings.Contains(err.Error(), "outside the root") {
			t.Errorf("listing %q was refused without naming the refusal: %v", path, err)
		}
	}
}

// TestListDirOnAFileIsAnError checks that a path naming a file is refused,
// since the read of a directory as a file is refused the other way round.
func TestListDirOnAFileIsAnError(t *testing.T) {
	s, _ := fixture(t)

	err := mustFail(t, call(t, s, listDirTool, `{"path":"file"}`))
	if !strings.Contains(err.Error(), "cannot list") {
		t.Errorf("the refusal did not say what was being attempted: %v", err)
	}
}

// TestNilRootOffersNoTools checks that a root the session could not take does
// not become a tool that cannot run, since a call answered by nothing is a call
// a model repeats.
func TestNilRootOffersNoTools(t *testing.T) {
	s := NewFilesystem(nil)

	if names := s.Names(); len(names) != 0 {
		t.Errorf("the set offers %v, which cannot be run", names)
	}
}

// TestNewFilesystemAtRefusesAMissingDirectory checks the open, since a
// containment is the descriptor that was opened rather than the name it was
// given.
func TestNewFilesystemAtRefusesAMissingDirectory(t *testing.T) {
	_, err := NewFilesystemAt(filepath.Join(t.TempDir(), "no-such-directory"))
	if err == nil {
		t.Fatal("a root was opened over a directory that is not there")
	}
}

// quote spells a path as a JSON string, so that a fixture holding a space or a
// backslash is escaped the way the argument of a call carries it rather than by
// hand.
func quote(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		// A string cannot fail to marshal, so a test reaching here has found
		// something the standard library did not expect.
		panic(err)
	}
	return string(quoted)
}
