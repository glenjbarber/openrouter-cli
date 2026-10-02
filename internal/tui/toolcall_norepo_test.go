package tui

import (
	"os/exec"
	"strings"
	"testing"
)

// A directory in no repository is not offered the git tool, and the reader is
// not shown the fatal line git printed. The other tools are kept.
func TestNoGitToolOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not on the PATH: %v", err)
	}
	ts := toolsAt(t.TempDir(), nil)

	if got := ts.absence(); got != "" {
		t.Errorf("a directory in no repository reported a problem: %q", got)
	}
	for _, name := range ts.names() {
		if name == "git" {
			t.Errorf("the git tool was offered outside a repository: %v", ts.names())
		}
	}
	if len(ts.names()) == 0 {
		t.Error("the other tools were lost along with git")
	}
	for _, line := range ts.toolsListing() {
		if strings.Contains(line, "fatal") || strings.Contains(line, "(null)") {
			t.Errorf("the listing showed the raw git error: %q", line)
		}
	}
}
