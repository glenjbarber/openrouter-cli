package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The marker must record the writing process, so that a marker left by a crash
// can be told from one held by a running session.
func TestSetCognitoWritesMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := setCognito(true); err != nil {
		t.Fatalf("setCognito: %v", err)
	}

	path := filepath.Join(home, cognitoMarker)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := strconv.Itoa(os.Getpid())
	if string(data) != want {
		t.Errorf("marker = %q, want the process identifier %q", string(data), want)
	}

	st, err := readCognito()
	if err != nil {
		t.Fatalf("readCognito: %v", err)
	}
	if !st.on || st.crash {
		t.Errorf("state = %+v, want on and not a crash", st)
	}
}

func TestSetCognitoRemovesMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := setCognito(true); err != nil {
		t.Fatalf("setCognito: %v", err)
	}
	if err := setCognito(false); err != nil {
		t.Fatalf("setCognito off: %v", err)
	}

	st, err := readCognito()
	if err != nil {
		t.Fatalf("readCognito: %v", err)
	}
	if st.on {
		t.Error("still on after being switched off")
	}
}

// Removing an absent marker is not an error, since switching the mode off twice
// should not fail.
func TestSetCognitoOffWhenAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := setCognito(false); err != nil {
		t.Errorf("setCognito off with no marker: %v", err)
	}
}

// A marker naming a process that is gone is a crash, and must be reported so
// that work is not recorded silently.
func TestReadCognitoDetectsCrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A process identifier that cannot be running.
	path := filepath.Join(home, cognitoMarker)
	if err := os.WriteFile(path, []byte("999999999"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := readCognito()
	if err != nil {
		t.Fatalf("readCognito: %v", err)
	}
	if !st.on {
		t.Error("on = false, want the marker honoured")
	}
	if !st.crash {
		t.Error("crash = false, want a stale marker reported as one")
	}
}

// Nothing may be recorded while the mode is in force.
func TestCognitoSuppressesRecording(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.cognito = true
	s.conv.setEphemeral()

	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")
	if s.conv.Turns() != 0 {
		t.Errorf("turns = %d, want recording suppressed", s.conv.Turns())
	}
	if s.conv.Recording() {
		t.Error("Recording = true in cognito mode")
	}
}

// A thread records nothing regardless, so the mode adds nothing while one is
// open.
func TestThreadIsUnrecordedByDefault(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.conv.SetModel("test/model")
	// History is recorded first, so the thread carries something and the
	// check below can tell an added exchange from the copied turns.
	s.conv.Record("earlier", "earlier reply")

	s.beginThread()
	if s.thread == nil {
		t.Fatal("the thread was not started")
	}
	if s.conv.Recording() {
		t.Error("Recording = true inside a thread, want it unrecorded")
	}

	// The thread carries the history it branched from, so the turns it holds
	// are not zero. What matters is that an exchange sent in it is not added.
	before := s.conv.Turns()
	if before == 0 {
		t.Fatal("the thread holds no history, so the check below proves nothing")
	}
	s.conv.Record("q", "a")
	if s.conv.Turns() != before {
		t.Errorf("turns = %d, want %d, so a thread recorded an exchange",
			s.conv.Turns(), before)
	}
}

// Leaving a thread must restore the main conversation exactly, with nothing
// from the thread carried over.
func TestEndThreadRestoresMain(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.conv.SetModel("test/model")
	s.conv.Record("before", "the reply")

	s.beginThread()
	s.conv.Record("in the thread", "thread reply")
	s.endThread()

	if s.thread != nil {
		t.Error("still in a thread after leaving it")
	}
	if s.conv != s.mainConv {
		t.Error("the main conversation was not restored")
	}
	for _, m := range s.conv.Pending("next") {
		if m.Content == "in the thread" {
			t.Error("a thread exchange reached the main conversation")
		}
	}
}

// Starting a second thread must be refused, since nesting would leave the
// outer thread unreachable.
func TestBeginThreadRefusesSecond(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.conv.SetModel("test/model")

	s.beginThread()
	first := s.thread
	s.beginThread()

	if s.thread != first {
		t.Error("the thread was replaced, want the existing one kept")
	}
}

func TestEndThreadWhenNotInOne(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.endThread()

	if s.thread != nil {
		t.Error("a thread exists after leaving one that was never entered")
	}
}
