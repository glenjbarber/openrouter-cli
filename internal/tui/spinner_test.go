package tui

import (
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// The twiddle must turn while work is in progress and stop when it ends.
func TestSpinnerRunsAndStops(t *testing.T) {
	s := NewSpinner()

	var mu sync.Mutex
	seen := map[string]bool{}
	s.Start(func(frame string) {
		mu.Lock()
		seen[frame] = true
		mu.Unlock()
	})

	// The first frame is delivered synchronously, so the spinner is observable
	// without waiting on the interval.
	mu.Lock()
	first := len(seen)
	mu.Unlock()
	if first == 0 {
		t.Error("no frame was delivered before the interval elapsed")
	}

	if !s.Active() {
		t.Error("Active = false while the twiddle was running")
	}

	time.Sleep(spinnerInterval * 3)
	s.Stop()

	mu.Lock()
	during := len(seen)
	mu.Unlock()

	if during < 2 {
		t.Errorf("frames seen = %d, want the twiddle to have advanced", during)
	}
	if s.Active() {
		t.Error("Active = true after Stop")
	}

	// Once stopped, no further frame may arrive, since a late frame would
	// paint over the interface after the work finished.
	time.Sleep(spinnerInterval * 3)
	mu.Lock()
	after := len(seen)
	mu.Unlock()
	if after != during {
		t.Errorf("frames after Stop = %d, want %d", after, during)
	}
}

// Starting an already-running spinner must not start a second goroutine, or
// the twiddle would turn at double rate.
func TestSpinnerStartIsIdempotent(t *testing.T) {
	s := NewSpinner()

	var mu sync.Mutex
	count := 0
	cb := func(string) {
		mu.Lock()
		count++
		mu.Unlock()
	}

	s.Start(cb)
	s.Start(cb)
	s.Stop()

	mu.Lock()
	got := count
	mu.Unlock()
	// One frame is delivered inline by Start and one by the goroutine. Two
	// starts would make it four, since Start is what delivers the inline one.
	if got != 2 {
		t.Errorf("frames = %d, want 2, so Start ran only once", got)
	}
}

// Stopping an idle spinner must not block or panic, since it is called from a
// deferred path that may run when nothing started.
func TestSpinnerStopWhenIdle(t *testing.T) {
	NewSpinner().Stop()
}

// Every frame must be a single column wide, or the interface jitters as the
// twiddle turns.
func TestSpinnerFramesAreUniformWidth(t *testing.T) {
	for _, f := range spinnerFrames {
		if utf8.RuneCountInString(f) != 1 {
			t.Errorf("frame %q is %d columns, want 1", f, utf8.RuneCountInString(f))
		}
	}
}

// The frame index belongs to the spinner, and the goroutine that turns it runs
// alongside whichever caller stops and starts it. Advancing the index without
// the lock is an unordered write to it, which shows here as soon as one caller
// stops a twiddle while another starts the next.
func TestSpinnerIndexIsTakenUnderTheLock(t *testing.T) {
	s := NewSpinner()

	var mu sync.Mutex
	frames := 0
	onFrame := func(frame string) {
		mu.Lock()
		frames++
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.Start(onFrame)
				time.Sleep(time.Millisecond)
				s.Stop()
			}
		}()
	}
	wg.Wait()
	s.Stop()

	if !s.Active() && frames == 0 {
		t.Error("no frame was delivered at all")
	}
}
