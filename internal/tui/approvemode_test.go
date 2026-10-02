package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// The mode settles what happens to a call the file does not and no earlier
// answer has. These cover the three modes and what they do not do.

func TestASessionBeginsAsking(t *testing.T) {
	s := &Session{approvals: newApprovalState()}

	if got := s.approvalLabel(); got != "ask" {
		t.Errorf("a new session reports %q, want ask", got)
	}
}

func TestAllowRunsWithoutAQuestion(t *testing.T) {
	dir := t.TempDir()
	s, reader := askingSession(t, "http://127.0.0.1:1", dir, true)
	s.setApprovalMode(modeAllow)

	if !s.Approve("echo", []string{"hi"}, dir) {
		t.Error("a mode of allow refused a program on the allowlist")
	}
	if got := reader.questions(); len(got) != 0 {
		t.Errorf("a mode of allow put %d questions to the reader", len(got))
	}
}

func TestRefuseRefusesWithoutAQuestion(t *testing.T) {
	dir := t.TempDir()
	s, reader := askingSession(t, "http://127.0.0.1:1", dir, false)
	s.setApprovalMode(modeRefuse)

	if s.Approve("echo", []string{"hi"}, dir) {
		t.Error("a mode of refuse approved a program")
	}
	if got := reader.questions(); len(got) != 0 {
		t.Errorf("a mode of refuse put %d questions to the reader", len(got))
	}
}

// The allowlist is not the mode. A mode of allow says nothing about what may be
// proposed, so a program outside the list is still refused by name.
func TestAllowDoesNotWidenTheAllowlist(t *testing.T) {
	dir := t.TempDir()
	s, _ := askingSession(t, "http://127.0.0.1:1", dir, true)
	s.setApprovalMode(modeAllow)

	set := tools.NewShell(dir, tools.AlwaysAllow())
	raw, err := json.Marshal(map[string]any{"command": "rm", "args": []string{"-rf", "x"}})
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	r := set.Run(openrouter.ToolCall{
		Function: openrouter.ToolCallFunction{Name: "shell", Arguments: string(raw)},
	})

	if r.Err == nil {
		t.Fatal("a program outside the allowlist ran under a mode of allow")
	}
	if !strings.Contains(r.Err.Error(), "not permitted") {
		t.Errorf("the refusal did not name the problem: %v", r.Err)
	}
}

// A remembered answer is meaningless once the reader has stopped asking, so
// changing the mode clears them. A reader moving from refusing to allowing would
// otherwise find every program they had refused still refused.
func TestChangingTheModeClearsWhatWasRemembered(t *testing.T) {
	st := newApprovalState()
	st.record("go", false)
	st.record("make", true)

	st.setMode(modeAllow)

	if _, answered := st.remembered("go"); answered {
		t.Error("a refusal survived a change of mode")
	}
	if _, answered := st.remembered("make"); answered {
		t.Error("a grant survived a change of mode")
	}
}

func TestTheModeIsReportedInTheStatusField(t *testing.T) {
	s := &Session{
		approvals: newApprovalState(),
		tools:     &toolSet{},
	}
	for _, mode := range []approvalMode{modeAsk, modeAllow, modeRefuse} {
		s.setApprovalMode(mode)
		if got := s.approvalLabel(); got != mode.String() {
			t.Errorf("mode %v reported as %q", mode, got)
		}
	}
}

// A file rule is the reader having answered in advance, so it settles a call
// whatever the mode says. A mode of refuse is a decision about what to ask, not
// a revocation of what was permitted.
func TestAFileRuleSettlesTheCallWhateverTheMode(t *testing.T) {
	dir := t.TempDir()
	s, _ := askingSession(t, "http://127.0.0.1:1", dir, true)
	s.setApprovalMode(modeRefuse)
	s.Configure(&config.Config{Tools: []config.ApprovalRule{
		{Path: dir, Commands: []string{"echo"}},
	}})

	if !s.Approve("echo", nil, dir) {
		t.Error("a mode of refuse revoked a rule the file had permitted")
	}
}

// The mode is refused while a model is working, since a turn in flight is
// holding a question and changing the answer under it settles a call the reader
// never saw.
func TestApproveIsRefusedWhileAModelWorks(t *testing.T) {
	s := &Session{
		conv:      NewConversation(),
		approvals: newApprovalState(),
		tools:     &toolSet{},
	}
	s.mainConv = s.conv
	s.turn = &turnState{}

	if s.cmdApprove([]string{"allow"}) {
		t.Error("the command asked the session to end")
	}
	if got := s.approvals.mode; got != modeAsk {
		t.Errorf("the mode became %v while a model was working", got)
	}
}

func TestApproveWithNoArgumentReportsTheMode(t *testing.T) {
	s := &Session{
		conv:      NewConversation(),
		approvals: newApprovalState(),
		tools:     &toolSet{dir: t.TempDir()},
	}
	s.mainConv = s.conv
	s.setApprovalMode(modeAllow)

	if s.cmdApprove(nil) {
		t.Error("the command asked the session to end")
	}
}

func TestApproveRefusesAModeItDoesNotKnow(t *testing.T) {
	s := &Session{
		conv:      NewConversation(),
		approvals: newApprovalState(),
		tools:     &toolSet{dir: t.TempDir()},
	}
	s.mainConv = s.conv

	s.cmdApprove([]string{"perhaps"})
	if got := s.approvals.mode; got != modeAsk {
		t.Errorf("an unknown mode changed the mode to %v", got)
	}
}
