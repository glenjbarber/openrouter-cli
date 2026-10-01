package tui

import (
	"testing"
	"time"
)

// The conversation a delegate branched from is replaced while the delegate is
// still running, since a thread can start and the in-cognito mode can be
// turned on. The delegate goroutine must not read it to find out which model to
// ask, because the input goroutine is the one replacing it.
func TestDelegateDoesNotReadTheConversationWhileItRuns(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	s := blockingSession(t, auditStream, reached, release)
	s.conv.Record("a question", "an answer")

	s.startDelegate("what next")

	// The input goroutine goes on starting and leaving threads while the
	// delegate works, which is what replaces the conversation. No channel
	// passes between the two here, which is the point: the goroutine reading
	// the conversation while this runs is an unordered access to it.
	for i := 0; i < 200; i++ {
		s.beginThread()
		s.endThread()
	}

	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for s.delegatePending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.delegatePending(); got != 0 {
		t.Fatalf("%d delegates still running after the release", got)
	}
}
