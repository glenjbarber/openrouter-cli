package tui

import (
	"sync"
	"time"
)

// spinnerFrames are the characters of the twiddle.
//
// The set is drawn from a terminal font that has them all, and each step is one
// column wide so that the frame does not jitter as it turns. A braille set is
// used rather than dots because the dots read as a loading circle at one
// character, and the braille figures are finer at the same width.
var spinnerFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

// spinnerInterval is how long each frame is shown.
//
// The figure is chosen to be slow enough not to be a distraction and fast
// enough to read as movement rather than as a stall. A terminal repainting a
// whole frame this often is cheap, but a spinner that updates faster than the
// eye resolves reads as a flicker.
const spinnerInterval = 90 * time.Millisecond

// Spinner drives the twiddle while work is in progress.
//
// It runs on its own goroutine, so the frame it writes to must be safe for
// concurrent use. The mutex belongs to the session rather than to the spinner,
// since the session also writes the frame from the request loop.
type Spinner struct {
	mu     sync.Mutex
	frame  int
	active bool
	stop   chan struct{}
	done   chan struct{}
}

// NewSpinner returns an idle spinner.
func NewSpinner() *Spinner { return &Spinner{} }

// Start begins turning the twiddle.
//
// The callback is called once per frame and must return quickly, since it
// repaints the interface. Starting an already-running spinner does nothing, so
// a caller need not track whether one is running.
func (s *Spinner) Start(onFrame func(frame string)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return
	}
	s.active = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	stop, done := s.stop, s.done

	// The first frame is delivered before the goroutine starts, so that
	// something is on screen the moment work begins rather than after the
	// first interval has elapsed.
	onFrame(spinnerFrames[s.current()])

	go func() {
		defer close(done)
		for {
			onFrame(spinnerFrames[s.next()])
			select {
			case <-stop:
				return
			case <-time.After(spinnerInterval):
			}
		}
	}()
}

// next returns the frame index and advances it, taking the lock to do so.
//
// The goroutine goes through this rather than reaching current directly. The
// index belongs to the spinner, and a caller that stops one and starts another
// runs on another goroutine, so an advance made without the lock would be an
// unordered write to it.
func (s *Spinner) next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current()
}

// current returns the frame index and advances it. The caller holds the lock.
func (s *Spinner) current() int {
	f := s.frame % len(spinnerFrames)
	s.frame = (s.frame + 1) % len(spinnerFrames)
	return f
}

// Stop halts the twiddle and waits for the goroutine to finish.
//
// Waiting matters: the goroutine writes to the frame, and returning before it
// has stopped would let it paint over a frame drawn after the work finished.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	stop, done := s.stop, s.done
	s.mu.Unlock()

	close(stop)
	<-done
}

// Active reports whether the twiddle is turning.
func (s *Spinner) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}
