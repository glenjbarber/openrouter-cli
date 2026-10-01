package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Adopting the marker must stop the recording, not merely announce that it has
// stopped. A session that honours the mode in the pane while still keeping
// every exchange is worse than one that ignores the marker, since the reader is
// told the work is being discarded.
func TestAdoptCognitoRecordsNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := setCognito(true); err != nil {
		t.Fatalf("setCognito: %v", err)
	}

	s, _ := auditSession(t, "")
	if err := s.AdoptCognito(); err != nil {
		t.Fatalf("AdoptCognito: %v", err)
	}

	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")
	if s.conv.Turns() != 0 {
		t.Errorf("turns = %d, want the adopted conversation to record nothing", s.conv.Turns())
	}
	if s.conv.Recording() {
		t.Error("Recording = true after the marker was adopted")
	}
}

// A marker left by a crash is reported rather than honoured without comment,
// since a reader who believes nothing is being recorded, and is, would lose the
// work with nothing said.
func TestAdoptCognitoReportsACrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, cognitoMarker)
	if err := os.WriteFile(path, []byte("999999999"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, _ := auditSession(t, "")
	if err := s.AdoptCognito(); err != nil {
		t.Fatalf("AdoptCognito: %v", err)
	}
	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "did not exit cleanly") {
		t.Errorf("pane = %q, want the marker reported as one left by a crash", joined)
	}
}

// A marker that cannot be read is an error rather than an absence. Treating it
// as absent would begin recording a session the reader had asked to record
// nothing.
func TestAdoptCognitoRefusesUnreadableMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A directory at the marker path cannot be read as a file, and could not
	// be written as one either.
	if err := os.Mkdir(filepath.Join(home, cognitoMarker), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	s, _ := auditSession(t, "")
	if err := s.AdoptCognito(); err == nil {
		t.Fatal("AdoptCognito with an unreadable marker returned no error")
	}
}

// Without a marker the session records as it always has, and says nothing
// about a mode that is not in force.
func TestAdoptCognitoSilentWithoutAMarker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, _ := auditSession(t, "")
	if err := s.AdoptCognito(); err != nil {
		t.Fatalf("AdoptCognito: %v", err)
	}
	if s.cognito {
		t.Error("cognito = true with no marker in force")
	}
	s.conv.Record("q", "a")
	if s.conv.Turns() != 2 {
		t.Errorf("turns = %d, want the exchange recorded", s.conv.Turns())
	}
	if len(s.frame.Reply) != 0 {
		t.Errorf("pane = %q, want nothing said about a mode that is off", s.frame.Reply)
	}
}

// The marker records the process that wrote it and nothing else. Nothing about
// the conversation may be written beside it.
func TestCognitoMarkerHoldsOnlyTheProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, _ := auditSession(t, "")
	s.conv.Seed("instructions that must not be written anywhere")
	s.conv.Record("a question", "an answer")
	if err := setCognito(true); err != nil {
		t.Fatalf("setCognito: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, cognitoMarker))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()); string(data) != want {
		t.Errorf("marker = %q, want only the process identifier %q", string(data), want)
	}
	for _, leaked := range []string{"instructions", "question", "answer"} {
		if strings.Contains(string(data), leaked) {
			t.Errorf("marker carries %q, want no conversation text anywhere", leaked)
		}
	}
}
