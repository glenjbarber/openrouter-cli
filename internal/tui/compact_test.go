package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The opening instructions must survive a compaction, since losing them changes
// how the model behaves without anything saying so.
func TestCompactKeepsInstructions(t *testing.T) {
	c := NewConversation()
	c.Seed("answer in the third person")
	c.Record("q1", "a1")
	c.Record("q2", "a2")

	c.Compact("a summary of the work")

	msgs := c.Pending("next")
	if msgs[0].Role != openrouter.RoleSystem ||
		!strings.Contains(msgs[0].Content, "answer in the third person") {
		t.Errorf("msgs[0] = %+v, want the instructions kept", msgs[0])
	}
}

// The summary is added as a system turn so it governs every later request.
func TestCompactAddsSummary(t *testing.T) {
	c := NewConversation()
	c.Record("q", "a")
	c.Compact("the summary")

	if !c.HasSummary() {
		t.Error("HasSummary = false, want true")
	}
	msgs := c.Pending("next")
	if !strings.Contains(msgs[0].Content, compactionMarker) {
		t.Errorf("msgs[0] = %q, want it marked as a summary", msgs[0].Content)
	}
}

// A second compaction must replace the first rather than summarising a
// summary, which would compound the loss on every pass.
func TestCompactReplacesEarlierSummary(t *testing.T) {
	c := NewConversation()
	c.Record("q1", "a1")
	c.Compact("first summary")
	c.Record("q2", "a2")
	c.Compact("second summary")

	msgs := c.Pending("next")
	marked := 0
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, compactionMarker) {
			marked++
			if !strings.Contains(m.Content, "second summary") {
				t.Errorf("summary = %q, want the later one", m.Content)
			}
		}
	}
	if marked != 1 {
		t.Errorf("marked summaries = %d, want exactly 1", marked)
	}
}

// A summary must not absorb the instructions, since Compact keeps the original
// and a summary of them would compete with it.
func TestForSummaryExcludesInstructions(t *testing.T) {
	c := NewConversation()
	c.Seed("be terse")
	c.Record("q1", "a1")
	c.Record("q2", "a2")
	c.Record("q3", "a3")

	msgs := c.forSummary()
	for _, m := range msgs {
		if m.Role == openrouter.RoleSystem {
			t.Errorf("forSummary carried a system turn: %+v", m)
		}
	}
	// Three records give six turns; the system turn is dropped and the most
	// recent two are held out, leaving four.
	if len(msgs) != 4 {
		t.Errorf("len(msgs) = %d, want 4, holding the recent turns out", len(msgs))
	}
}

// A conversation too short to gain from compacting is left alone, since
// compacting two turns produces something no shorter than itself.
func TestShouldCompactNeedsEnoughTurns(t *testing.T) {
	if shouldCompact(1000, 2, 900) {
		t.Error("shouldCompact = true for two turns, want false")
	}
	if !shouldCompact(1000, 8, 900) {
		t.Error("shouldCompact = false past the threshold, want true")
	}
}

// The threshold is a share of the window, so the same token count compacts
// against a smaller window and not against a larger one.
func TestShouldCompactScalesWithWindow(t *testing.T) {
	tokens := 100_000
	if !shouldCompact(128_000, 8, tokens) {
		t.Error("shouldCompact = false at 78% of 128k, want true")
	}
	if shouldCompact(200_000, 8, tokens) {
		t.Error("shouldCompact = true at 50% of 200k, want false")
	}
}

// An unknown window must not disable compaction, which a zero would do.
func TestShouldCompactRejectsZeroWindow(t *testing.T) {
	if shouldCompact(0, 8, 900) {
		t.Error("shouldCompact = true with no window, want false")
	}
}

func TestEstimatedTokensGrows(t *testing.T) {
	c := NewConversation()
	if c.EstimatedTokens() != 0 {
		t.Errorf("EstimatedTokens = %d, want 0 when empty", c.EstimatedTokens())
	}
	c.Record("hello", "world")
	if c.EstimatedTokens() == 0 {
		t.Error("EstimatedTokens = 0 after a turn, want a positive figure")
	}
}

// A fallback window is used when the model list does not report one, since a
// compaction threshold needs a window to be a share of.
func TestLookupFallsBackWithoutClient(t *testing.T) {
	c := newContextLength()
	if got := c.lookup(nil, &Session{}, "some/model"); got != fallbackWindow {
		t.Errorf("lookup = %d, want the fallback %d", got, fallbackWindow)
	}
}

// An empty model must not be sent, since the backend refuses it.
func TestSummariseRejectsEmpty(t *testing.T) {
	_, err := Summarise(nil, nil, "some/model", nil)
	if err == nil {
		t.Fatal("Summarise accepted nothing to summarise, want an error")
	}
}
