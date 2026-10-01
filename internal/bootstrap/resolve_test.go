package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// A symlink on the same filesystem is followed, since that is the ordinary case
// for a document aliased into a working directory.
func TestResolveFollowsSameDeviceLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "MEMORY.md")
	write(t, target, "Be terse.")

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	doc, err := Load(link)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Instructions != "Be terse." {
		t.Errorf("Instructions = %q, want the target contents", doc.Instructions)
	}
}

// The reported path is the one the caller named, not the resolved target, so a
// diagnostic points at the file the user typed.
func TestLoadKeepsNamedPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "MEMORY.md")
	write(t, target, "Be terse.")

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	doc, err := Load(link)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Path != link {
		t.Errorf("Path = %q, want %q", doc.Path, link)
	}
}

// A relative link is resolved against the directory holding the link, not the
// working directory.
func TestResolveRelativeLink(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	target := filepath.Join(sub, "real.md")
	link := filepath.Join(dir, "MEMORY.md")
	write(t, target, "Be terse.")

	if err := os.Symlink("sub/real.md", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Load(link); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// A link pointing at a directory is refused rather than read as a document.
func TestResolveRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub.md")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	_, err := Load(sub)
	if err == nil {
		t.Fatal("Load accepted a directory, want an error")
	}
}

// A self-referential link is reported rather than followed, so a cycle does
// not spin until the hop bound is reached.
func TestResolveReportsLoop(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")

	if err := os.Symlink(b, a); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Load(a)
	if err == nil {
		t.Fatal("Load followed a cycle, want an error")
	}
}

// A missing link target is an error, since the named file cannot be read.
func TestResolveBrokenLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "MEMORY.md")
	if err := os.Symlink(filepath.Join(dir, "absent.md"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Load(link); err == nil {
		t.Fatal("Load accepted a broken link, want an error")
	}
}
