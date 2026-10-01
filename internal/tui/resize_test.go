package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// resizeFrame is a frame with something in every part, so that a frame of the
// wrong height shows up as a missing prompt rather than as an empty row.
func resizeFrame() Frame {
	return Frame{
		Title: "openrouter-cli",
		Reply: []string{"a reply", "another reply"},
		Input: "hi",
	}
}

// The window size must be re-read rather than cached from startup. A terminal
// resized while the client runs was drawn to the size it had when it opened,
// which left the prompt below the bottom of the window.
//
// The size is checked against whatever the query reports, so the test needs a
// terminal it can resize. A regular file reports a fixed size, which would pass
// whether the value was cached or re-read.
func TestSizeFollowsRealTerminalResize(t *testing.T) {
	master, setSize := openResizablePTY(t)

	setSize(24, 80)
	s := &Screen{out: master}
	height, width := s.Size()
	if height != 24 || width != 80 {
		t.Fatalf("Size() = %d, %d, want 24, 80", height, width)
	}

	// A cached size would still report 24, 80 here.
	setSize(12, 40)
	height, width = s.Size()
	if height != 12 || width != 40 {
		t.Errorf("after resize Size() = %d, %d, want 12, 40", height, width)
	}
}

// A frame drawn after a resize must fit the new window, since a frame of the
// old height is written past the bottom and scrolls the header out of view.
func TestFrameFitsAfterRealResize(t *testing.T) {
	master, setSize := openResizablePTY(t)
	setSize(24, 80)

	s := &Screen{out: master}
	setSize(12, 40)
	height, width := s.Size()
	lines := Render(resizeFrame(), height, width)
	if len(lines) > height {
		t.Errorf("frame is %d rows, want at most %d", len(lines), height)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "> hi") {
		t.Error("the prompt is missing after a resize")
	}
}

// A resize must be corrected by the next repaint without waiting for a
// keypress. The spinner repaints on a timer, so a resize between two of its
// frames is picked up by the next one.
func TestResizePickedUpWithoutAKeypress(t *testing.T) {
	master, setSize := openResizablePTY(t)
	setSize(24, 80)

	s := &Screen{out: master}
	setSize(30, 100)
	height, width := s.Size()
	if height != 30 || width != 100 {
		t.Fatalf("Size() = %d, %d, want 30, 100", height, width)
	}
	if lines := Render(resizeFrame(), height, width); len(lines) != height {
		t.Errorf("frame is %d rows, want %d", len(lines), height)
	}
}

// A size that cannot be read must keep the last one rather than reporting
// nothing. A zero-sized frame draws nothing at all, which is worse than a
// stale one.
func TestSizeKeepsLastValueWhenUnreadable(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("opening /dev/null: %v", err)
	}
	defer f.Close()

	s := &Screen{out: f, height: 24, width: 80}
	height, width := s.Size()
	if height != 24 || width != 80 {
		t.Errorf("Size() = %d, %d, want the last known 24, 80", height, width)
	}
}

// A size that reads as zero is nonsense and must not be adopted, since a frame
// drawn to it would be empty.
func TestSizeIgnoresNonsense(t *testing.T) {
	master, setSize := openResizablePTY(t)
	setSize(24, 80)

	s := &Screen{out: master}
	height, width := s.Size()
	if height != 24 || width != 80 {
		t.Fatalf("Size() = %d, %d, want 24, 80", height, width)
	}

	// A terminal reporting a window of no columns would empty the frame.
	setSize(0, 0)
	height, width = s.Size()
	if height != 24 || width != 80 {
		t.Errorf("after a zero size Size() = %d, %d, want the last known 24, 80",
			height, width)
	}
}

// A session must read the size the same way, so that the frame it draws after
// a resize is the new size rather than the one it opened at.
func TestSessionReadsTheCurrentSize(t *testing.T) {
	master, setSize := openResizablePTY(t)
	setSize(24, 80)

	s := &Session{
		conv:    NewConversation(),
		screen:  &Screen{out: master},
		spinner: NewSpinner(),
		windows: newContextLength(),
		ctx:     t.Context(),
		cancel:  func() {},
		client:  openrouter.New("http://127.0.0.1:0", "k"),
	}

	setSize(15, 60)
	height, width := s.screen.Size()
	if height != 15 || width != 60 {
		t.Fatalf("Size() = %d, %d, want 15, 60", height, width)
	}
	if lines := Render(resizeFrame(), height, width); len(lines) > height {
		t.Errorf("frame is %d rows, want at most %d", len(lines), height)
	}
}
