package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// saveSession returns a session whose home directory is a temporary one, so
// that a save lands somewhere the test owns rather than in the reader's own
// directory of sessions.
func saveSession(t *testing.T) *Session {
	t.Helper()
	return saveSessionIn(t, t.TempDir())
}

// saveSessionIn is saveSession with a home directory the caller names, which is
// what a test that saves in one session and loads in another needs.
func saveSessionIn(t *testing.T, home string) *Session {
	t.Helper()
	t.Setenv("HOME", home)
	s, _ := auditSession(t, "")
	s.editor = NewLineEditor(strings.NewReader("\n"))
	return s
}

// pathUnder reports where a save under this name would land.
func pathUnder(t *testing.T, name string) string {
	t.Helper()
	p, err := saved.Path(name)
	if err != nil {
		t.Fatalf("Path(%q): %v", name, err)
	}
	return p
}

// A save writes the conversation to a file of its own and says where.
func TestSaveWritesTheConversation(t *testing.T) {
	s := saveSession(t)
	s.conv.SetModel("test/model")
	s.conv.Record("the question", "the answer")

	if s.command("/save work") {
		t.Fatal("the command ended the session")
	}
	if _, err := os.Stat(pathUnder(t, "work")); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	got, err := saved.Read(pathUnder(t, "work"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Model != "test/model" {
		t.Errorf("the model is %q, want test/model", got.Model)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("the file holds %d messages, want the two of the exchange", len(got.Messages))
	}
	if got.Messages[0].Content != "the question" || got.Messages[1].Content != "the answer" {
		t.Errorf("the turns are %+v, want the question and the answer in order", got.Messages)
	}
}

// The mode promises that nothing is recorded, on disk as well as in memory, so
// a save is refused while it is on rather than writing a file and reporting
// that nothing was recorded.
func TestSaveIsRefusedInCognito(t *testing.T) {
	s := saveSession(t)
	s.cognito = true
	s.conv.Record("the question", "the answer")

	s.command("/save work")

	if _, err := os.Stat(pathUnder(t, "work")); !os.IsNotExist(err) {
		t.Errorf("the save created a file, want it refused: %v", err)
	}
	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "cognito") {
		t.Errorf("the pane says %q, want it to say why the save was refused", pane)
	}
}

// A thread records nothing, so there is nothing in one to save.
func TestSaveIsRefusedInAThread(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the question", "the answer")
	s.beginThread()

	s.command("/save work")

	if _, err := os.Stat(pathUnder(t, "work")); !os.IsNotExist(err) {
		t.Errorf("the save created a file, want it refused: %v", err)
	}
	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "thread") {
		t.Errorf("the pane says %q, want it to say why the save was refused", pane)
	}
}

// A save asked for under a name already taken asks first, and a reader who does
// not say yes keeps the conversation already on disk.
func TestSaveUnderATakenNameAsksBeforeReplacing(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the first", "the first answer")
	s.command("/save work")

	// The answer to the prompt is a line from the editor, so a decline is
	// answered with something that is not yes.
	s.editor = NewLineEditor(strings.NewReader("n\n"))
	s.conv.Record("the second", "the second answer")
	s.command("/save work")

	got, err := saved.Read(pathUnder(t, "work"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Messages[0].Content != "the first" {
		t.Errorf("the file holds %q, want the first session left alone", got.Messages[0].Content)
	}

	// The declined save is filed beside it under a name carrying the epoch,
	// since an answer of no should not throw the conversation away.
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(pathUnder(t, "work"))))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "work-") && strings.HasSuffix(e.Name(), ".db") {
			found = true
		}
	}
	if !found {
		t.Errorf("the directory holds %v, want a second file carrying an epoch", entries)
	}
}

// A reader who says yes is taken at their word, since they were asked.
func TestSaveUnderATakenNameOverwritesWhenSaid(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the first", "the first answer")
	s.command("/save work")

	s.editor = NewLineEditor(strings.NewReader("y\n"))
	s.conv.Record("the second", "the second answer")
	s.command("/save work")

	got, err := saved.Read(pathUnder(t, "work"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Messages) != 4 {
		t.Fatalf("the file holds %d messages, want both exchanges", len(got.Messages))
	}
	if got.Messages[2].Content != "the second" || got.Messages[3].Content != "the second answer" {
		t.Errorf("the file ends %+v, want the second exchange written over the first", got.Messages[2:])
	}
}

// A load puts the conversation back, with the figures that went with it.
func TestLoadRestoresTheConversation(t *testing.T) {
	home := t.TempDir()
	s := saveSessionIn(t, home)
	s.conv.SetModel("test/model")
	s.conv.Record("the question", "the answer")
	s.conv.AddTokens(10, 20)
	s.command("/save work")

	fresh := saveSessionIn(t, home)
	fresh.command("/load work")

	if got := fresh.conv.Turns(); got != 2 {
		t.Errorf("turns = %d, want the two messages of the one exchange", got)
	}
	if fresh.conv.Model() != "test/model" {
		t.Errorf("the model is %q, want test/model", fresh.conv.Model())
	}
	if fresh.conv.TokensIn() != 10 || fresh.conv.TokensOut() != 20 {
		t.Errorf("tokens are %d and %d, want 10 and 20", fresh.conv.TokensIn(), fresh.conv.TokensOut())
	}
}

// The turns are shown as well as restored, since a load that replaced the
// conversation silently would leave the reader unable to tell what they had
// resumed.
func TestLoadShowsTheTurnsItRestored(t *testing.T) {
	home := t.TempDir()
	s := saveSessionIn(t, home)
	s.conv.Record("the question", "the answer")
	s.command("/save work")

	fresh := saveSessionIn(t, home)
	fresh.command("/load work")

	pane := strings.Join(fresh.frame.Reply, "\n")
	for _, want := range []string{"the question", "the answer", "loaded"} {
		if !strings.Contains(pane, want) {
			t.Errorf("the pane is %q, want it to hold %q", pane, want)
		}
	}
}

// A model the reader chose is not replaced by the one the saved session was
// carried by, which is the rule the configuration file's model is held to.
func TestLoadLeavesAChosenModelAlone(t *testing.T) {
	home := t.TempDir()
	s := saveSessionIn(t, home)
	s.conv.SetModel("test/model")
	s.conv.Record("the question", "the answer")
	s.command("/save work")

	fresh := saveSessionIn(t, home)
	fresh.conv.SetModel("other/model")
	fresh.command("/load work")

	if got := fresh.conv.Model(); got != "other/model" {
		t.Errorf("the model is %q, want the one the reader chose", got)
	}
}

// A load replaces the conversation a request in flight is holding, so it is
// refused while a model works on the same terms as /new.
func TestLoadIsRefusedWhileAModelWorks(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the question", "the answer")
	s.command("/save work")
	s.turn = &turnState{}

	s.command("/load work")

	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "refused") {
		t.Errorf("the pane is %q, want the refusal", pane)
	}
	if s.conv.Turns() != 2 {
		t.Error("the conversation was replaced while a model was working on it")
	}
}

// A load with no name says so rather than reporting an empty pane, since the
// line looked like a command.
func TestLoadWithNoNameIsAnswered(t *testing.T) {
	s := saveSession(t)
	s.command("/load")
	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "/load needs") {
		t.Errorf("the pane is %q, want it to ask for a name", pane)
	}
}

// A name nothing was saved under is reported rather than loaded as an empty
// conversation, which would look like one the reader had.
func TestLoadOfANameThatIsNotThere(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the question", "the answer")
	s.command("/load nothing")

	if s.conv.Turns() != 2 {
		t.Error("the conversation was replaced by a failed load")
	}
	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "no such file") {
		t.Errorf("the pane is %q, want the file reported missing", pane)
	}
}

// A name carrying a separator is a path rather than a label, and is refused
// before anything is read or written.
func TestSaveWithAPathForANameIsRefused(t *testing.T) {
	s := saveSession(t)
	s.conv.Record("the question", "the answer")
	s.command("/save ../../elsewhere")
	if pane := strings.Join(s.frame.Reply, "\n"); !strings.Contains(pane, "unusable session name") {
		t.Errorf("the pane is %q, want the name refused", pane)
	}
}

// The conversation lines name the role, so a reader can tell who said what in
// a restored exchange without the interface drawing anything around it.
func TestConversationLinesNameTheRole(t *testing.T) {
	got := conversationLines([]openrouter.Message{
		{Role: openrouter.RoleSystem, Content: "be brief"},
		{Role: openrouter.RoleUser, Content: "the question"},
		{Role: openrouter.RoleAssistant, Content: "the answer"},
	})
	want := []string{"system: be brief", "user: the question", "assistant: the answer"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
}
