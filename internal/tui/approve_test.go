package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// A session with nothing configured and nothing remembered, so that the
// approval decision can be made without a terminal in the way. The question
// itself is asked through the editor, which a test cannot drive, so these
// tests cover the decision rather than the prompt: what is remembered, and what
// the configuration settles on its own.

func newApprovalSession(t *testing.T, cfg *config.Config) *Session {
	t.Helper()
	dir := t.TempDir()
	s := &Session{
		approvals: newApprovalState(),
		cfg:       cfg,
		tools:     toolsAt(dir, nil),
	}
	s.tools.dir = dir
	t.Cleanup(func() { s.tools.set = nil })
	return s
}

func TestARefusedProgramIsNotAskedAboutAgain(t *testing.T) {
	s := newApprovalSession(t, nil)

	s.mu.Lock()
	s.approvals.record("rm", false)
	s.mu.Unlock()

	approved, answered := s.approvals.remembered("rm")
	if !answered || approved {
		t.Fatalf("a refusal was not remembered: approved=%v answered=%v", approved, answered)
	}
}

func TestAGrantedProgramIsNotAskedAboutAgain(t *testing.T) {
	s := newApprovalSession(t, nil)

	s.mu.Lock()
	s.approvals.record("go", true)
	s.mu.Unlock()

	approved, answered := s.approvals.remembered("go")
	if !answered || !approved {
		t.Fatalf("a grant was not remembered: approved=%v answered=%v", approved, answered)
	}
}

func TestAProgramNeverAnsweredIsAskedAbout(t *testing.T) {
	state := newApprovalState()

	if approved, answered := state.remembered("go"); approved || answered {
		t.Errorf("a program nobody decided about was treated as answered: %v %v", approved, answered)
	}
}

func TestALaterGrantOverridesAnEarlierRefusal(t *testing.T) {
	state := newApprovalState()

	state.record("go", false)
	state.record("go", true)

	approved, answered := state.remembered("go")
	// A reader who said no and then changed their mind has changed it, and the
	// question must not be answered from the earlier one.
	if !answered || !approved {
		t.Errorf("the later grant did not take effect: approved=%v answered=%v", approved, answered)
	}
}

func TestARecordedAnswerClearsTheOtherWay(t *testing.T) {
	state := newApprovalState()

	state.record("go", true)
	state.record("go", false)

	if state.granted["go"] {
		t.Error("a refusal left the grant in place, so the two disagree")
	}
	if !state.refused["go"] {
		t.Error("the refusal was not recorded")
	}
}

func TestAConfiguredRuleSettlesTheQuestionWithoutAsking(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Tools: []config.ApprovalRule{
		{Path: dir, Commands: []string{"go", "gofmt"}},
	}}
	// The session is built so that the rule is held the way a real one is,
	// and the assertion is on the configuration, since that is what settles
	// the question without a prompt.
	newApprovalSession(t, cfg)

	if !cfg.Permits("go", dir) {
		t.Fatal("the rule did not apply to the directory it was written for")
	}
	if cfg.Permits("rm", dir) {
		t.Error("the rule permitted a program it did not name")
	}
}

func TestARuleSettlesAChildDirectory(t *testing.T) {
	// The case the reader asked for: a rule written for a project settles a
	// session running beneath it, without a rule of its own.
	root := t.TempDir()
	child := filepath.Join(root, "repo", "internal")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	cfg := &config.Config{Tools: []config.ApprovalRule{{Path: root, Commands: []string{"go"}}}}

	if !cfg.Permits("go", child) {
		t.Error("a child of a permitted directory was not permitted")
	}
}

func TestNoConfigurationSettlesNothing(t *testing.T) {
	dir := t.TempDir()
	var cfg *config.Config

	if cfg.Permits("go", dir) {
		t.Error("a session with no configuration permitted something")
	}
}

// The tools set is built with a nil approver in these tests, so the shell is
// not offered. That is the property worth asserting: a session that cannot ask
// offers no shell at all, rather than one that runs without asking.

func TestASessionThatCannotAskOffersNoShell(t *testing.T) {
	s := newApprovalSession(t, nil)

	for _, name := range s.tools.names() {
		if name == "shell" {
			t.Error("a session with no approver offered the shell")
		}
	}
}
