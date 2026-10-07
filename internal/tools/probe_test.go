package tools

import (
	"path/filepath"
	"testing"
)

func TestProbeResolve(t *testing.T) {
	dir := t.TempDir()
	root := resolvedRoot(dir)
	glob := filepath.Join(dir, "*.go")
	t.Logf("root=%q", root)
	t.Logf("resolveExisting(%q) = %q", glob, resolveExisting(glob))
	t.Logf("escapesArg(%q, %q) = %v", "*.go", root, escapesArg("*.go", root))
	t.Logf("escapesArg(%q, %q) = %v", "../x", root, escapesArg("../x", root))
	t.Logf("escapesArg(%q, %q) = %v", "file", root, escapesArg("file", root))
}
