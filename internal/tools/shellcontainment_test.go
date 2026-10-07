package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellRefusesAnArgumentReachingOutsideTheTree checks the bound the working
// directory alone does not give: a process given a directory can still open any
// path it can name, so the arguments are held to the tree as well.
//
// Each case is one way out. An absolute path outside is the plainest, a
// parent reference is what a model writes when it means the one above, and a
// symlink is the one that passes a cleaned-path test.
func TestShellRefusesAnArgumentReachingOutsideTheTree(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	outside := filepath.Join(filepath.Dir(dir), "elsewhere")
	cases := [][]string{
		{"../elsewhere"},
		{outside},
		{"a/../../elsewhere"},
	}
	for _, args := range cases {
		r := shellCall(t, set, map[string]any{"command": "cat", "args": args})
		if r.Err == nil {
			t.Errorf("%v was not refused", args)
			continue
		}
		if !strings.Contains(r.Err.Error(), "outside") {
			t.Errorf("%v was refused without saying why: %v", args, r.Err)
		}
	}
}

// TestShellRefusesASymlinkReachingOutOfTheTree checks the case a cleaned-path
// test cannot see. The link is inside the directory and the name is too, so
// nothing about the argument itself says where it leads.
func TestShellRefusesASymlinkReachingOutOfTheTree(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("a symlink cannot be made here: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "cat", "args": []string{"link/secret"}})

	if r.Err == nil {
		t.Fatal("a symlink out of the tree was not refused")
	}
	if !strings.Contains(r.Err.Error(), "outside") {
		t.Errorf("the refusal did not say why: %v", r.Err)
	}
}

// TestShellAllowsAPathInsideTheTree checks that the bound refuses the outside
// without refusing the inside. An absolute path naming a file in the working
// directory is not reaching out of it, and a check written against absolute
// paths rather than against destinations would refuse this one.
func TestShellAllowsAPathInsideTheTree(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "file")
	if err := os.WriteFile(name, []byte("content\n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	for _, args := range [][]string{{name}, {"file"}, {"./file"}, {dir}} {
		r := shellCall(t, set, map[string]any{"command": "cat", "args": args})
		if r.Err == nil {
			continue
		}
		// The directory itself is not a file, so cat fails on it for its own
		// reason, and that failure belongs to the program rather than to the
		// bound. What is asserted is that none of these was refused for
		// reaching out of the tree, which is the same distinction
		// TestShellAnOptionIsNotAPath makes for grep matching nothing.
		if strings.Contains(r.Err.Error(), "outside") {
			t.Errorf("cat %v was refused: %v", args, r.Err)
		}
	}
}

// TestShellAnOptionIsNotAPath checks that the bound reads an argument as a
// program reads it. An option that resembles a parent reference is an option,
// and a flag carrying a path separator is the case that matters, since
// -I./dir names a directory and does not name it as an argument.
func TestShellAnOptionIsNotAPath(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	// grep takes a pattern and a directory, and the pattern here is a wildcard
	// rather than a path: no shell is read, so it is passed through as written
	// and reaches the program as a pattern the program expands itself.
	r := shellCall(t, set, map[string]any{"command": "grep", "args": []string{"*.go", "."}})

	if r.Err != nil {
		if strings.Contains(r.Err.Error(), "outside") {
			t.Errorf("a pattern was read as a path leaving the tree: %v", r.Err)
			return
		}
		// grep exits 1 when it matches nothing, which is the expected answer
		// here and is reported as a failure carrying the status.
		if !strings.Contains(r.Err.Error(), "grep failed") {
			t.Errorf("grep failed for another reason: %v", r.Err)
		}
	}
}

// TestShellDoesNotAskAboutAnArgumentLeavingTheTree checks that the refusal
// happens before the reader is asked, on the same reasoning as a program
// outside the list: a call that would be refused outright is not something to
// interrupt somebody about.
func TestShellDoesNotAskAboutAnArgumentLeavingTheTree(t *testing.T) {
	dir := t.TempDir()
	asked := false
	set := NewShell(dir, approveFunc(func(string, []string, string) bool {
		asked = true
		return true
	}))

	r := shellCall(t, set, map[string]any{"command": "rm", "args": []string{"-rf", "../elsewhere"}})

	if r.Err == nil {
		t.Fatal("an argument leaving the tree was not refused")
	}
	if asked {
		t.Error("the reader was asked about a call that would have been refused anyway")
	}
}
