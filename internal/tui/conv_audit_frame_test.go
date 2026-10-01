package tui

import "testing"

// Every writer of the frame takes the lock the paint path reads it under. A
// writer that does not is a race against the twiddle, which paints from its own
// goroutine while a command runs on the one that owns the conversation.
//
// The reader here is the paint path reduced to the field under test, since the
// paint path copies the whole frame under the lock for exactly this reason.
func TestFrameWritersTakeTheLockThePaintPathReadsUnder(t *testing.T) {
	s, _ := auditSession(t, "")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20000; i++ {
			s.mu.Lock()
			_ = len(s.frame.Reply)
			s.mu.Unlock()
		}
	}()

	for i := 0; i < 2000; i++ {
		s.beginThread()
		s.endThread()
		s.compact(true)
	}
	<-done
}
