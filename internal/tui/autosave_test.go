package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// autosaveSession builds a session that can write, with the timer off so that
// the test controls when a write happens.
func autosaveSession(t *testing.T) (*Session, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	conv := NewConversation()
	conv.SetModel("test/model")
	conv.Record("a question", "an answer")
	s := &Session{
		conv:      conv,
		mainConv:  conv,
		approvals: newApprovalState(),
		tools:     &toolSet{dir: t.TempDir()},
	}
	return s, filepath.Join(home, ".openrouter-cli", "sessions")
}

func TestAnAutosaveIsWrittenWhenAsked(t *testing.T) {
	s, dir := autosaveSession(t)

	path, err := s.writeAutosave()
	if err != nil {
		t.Fatalf("writing the autosave: %v", err)
	}

	if filepath.Dir(path) != dir {
		t.Errorf("the autosave went to %s, want %s", filepath.Dir(path), dir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file is not there: %v", err)
	}
}

func TestTheLinkPointsAtTheNewestSave(t *testing.T) {
	s, dir := autosaveSession(t)

	path, err := s.writeAutosave()
	if err != nil {
		t.Fatalf("writing the autosave: %v", err)
	}

	wd, _ := os.Getwd()
	link := filepath.Join(dir, saved.AutoLinkName(wd)+".db")

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("the link is not there: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the newest is a file rather than a link")
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("reading the link: %v", err)
	}
	if target != path {
		t.Errorf("the link points at %s, want %s", target, path)
	}
}

func TestASecondSaveMovesTheLink(t *testing.T) {
	s, dir := autosaveSession(t)

	first, err := s.writeAutosave()
	if err != nil {
		t.Fatalf("the first autosave: %v", err)
	}
	second, err := s.writeAutosave()
	if err != nil {
		t.Fatalf("the second autosave: %v", err)
	}

	wd, _ := os.Getwd()
	link := filepath.Join(dir, saved.AutoLinkName(wd)+".db")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("reading the link: %v", err)
	}
	if target == first && second != first {
		t.Errorf("the link still points at the first save")
	}
	if target != second {
		t.Errorf("the link points at %s, want the newest %s", target, second)
	}
}

// The mode promises nothing is recorded, on disk as well as in memory, so a
// file holding the conversation would break it rather than record that it had.
func TestAnAutosaveIsRefusedInCognito(t *testing.T) {
	s, _ := autosaveSession(t)
	s.cognito = true

	if _, err := s.writeAutosave(); err == nil {
		t.Error("an autosave was written while cognito was on")
	}
}

func TestAnAutosaveIsRefusedInAThread(t *testing.T) {
	s, _ := autosaveSession(t)
	s.thread = &Ephemeral{}

	if _, err := s.writeAutosave(); err == nil {
		t.Error("an autosave was written in a thread")
	}
}

// The timer is turned off and on without leaving a goroutine behind, and
// turning it on twice does not start a second one.
func TestTheTimerStartsOnce(t *testing.T) {
	s, _ := autosaveSession(t)

	s.setAutosave(true)
	s.mu.Lock()
	first := s.autosaveStop
	s.mu.Unlock()
	if first == nil {
		t.Fatal("the timer did not start")
	}

	s.setAutosave(true)
	s.mu.Lock()
	second := s.autosaveStop
	s.mu.Unlock()
	if first != second {
		t.Error("turning the timer on again replaced it rather than leaving it")
	}

	s.setAutosave(false)
	s.mu.Lock()
	stopped := s.autosaveStop
	s.mu.Unlock()
	if stopped != nil {
		t.Error("the timer did not stop")
	}
}

func TestTheAutosaveIsOffUnlessAskedFor(t *testing.T) {
	s, _ := autosaveSession(t)

	s.mu.Lock()
	running := s.autosaveStop != nil
	s.mu.Unlock()

	if running {
		t.Error("the timer is running on a session that never asked for it")
	}
}

func TestTheIntervalIsLongEnoughToBeUseful(t *testing.T) {
	// A figure of seconds would write the same conversation over and over
	// while a reader pauses to think, and a reader would come back to a
	// directory full of files that all say the same thing.
	if AutosaveInterval < time.Minute {
		t.Errorf("the interval is %s, which fills the directory with copies",
			AutosaveInterval)
	}
}

func TestAutosaveReportsWhatItIsDoing(t *testing.T) {
	s, _ := autosaveSession(t)

	lines := s.autosaveListing()
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "autosave: off") {
		t.Errorf("the listing does not say it is off:\n%s", joined)
	}
	if !strings.Contains(joined, AutosaveInterval.String()) {
		t.Errorf("the listing does not say how often:\n%s", joined)
	}
}

func TestAutosaveRefusesAnArgumentItDoesNotKnow(t *testing.T) {
	s, dir := autosaveSession(t)

	s.cmdAutosave([]string{"perhaps"})

	if _, err := os.Stat(dir); err == nil {
		t.Error("an unknown argument wrote something")
	}
}
