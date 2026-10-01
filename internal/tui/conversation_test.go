package tui

import (
	"strings"
	"testing"
)

// The user turn must travel with the request. Recording it only after the reply
// arrived would send an empty history, which the backend refuses.
func TestPendingCarriesTheUserTurn(t *testing.T) {
	c := NewConversation()
	msgs := c.Pending("hello")
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "hello" || msgs[0].Role != "user" {
		t.Errorf("msgs[0] = %+v, want the user turn", msgs[0])
	}
}

// A pending turn is not recorded, so a failed request leaves no half exchange
// for the next request to replay.
func TestPendingDoesNotRecord(t *testing.T) {
	c := NewConversation()
	c.Pending("hello")
	if got := c.Pending("hello"); len(got) != 1 {
		t.Errorf("len(msgs) = %d, want 1, so the pending turn was recorded", len(got))
	}
}

func TestRecordKeepsBothTurns(t *testing.T) {
	c := NewConversation()
	c.Record("q", "a")
	c.Record("q2", "a2")

	msgs := c.Pending("third")
	if len(msgs) != 5 {
		t.Fatalf("len(msgs) = %d, want 5", len(msgs))
	}
	if msgs[2].Role != "user" || msgs[3].Role != "assistant" {
		t.Errorf("roles = %q/%q, want user then assistant", msgs[2].Role, msgs[3].Role)
	}
}

// Clearing the conversation keeps the bootstrap instructions, since losing them
// would silently change how the model behaves.
func TestResetKeepsTheSeed(t *testing.T) {
	c := NewConversation()
	c.Seed("be terse")
	c.Record("q", "a")

	c.Reset()
	msgs := c.Pending("next")
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want the seed plus the new turn", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "be terse" {
		t.Errorf("msgs[0] = %+v, want the seed preserved", msgs[0])
	}
}

// A second seed replaces the first, so that changing the document does not
// leave the earlier one in force.
func TestSeedReplacesTheEarlierOne(t *testing.T) {
	c := NewConversation()
	c.Seed("first")
	c.Seed("second")

	msgs := c.Pending("hi")
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want the seed plus the new turn", len(msgs))
	}
	if msgs[0].Content != "second" {
		t.Errorf("msgs[0].Content = %q, want %q", msgs[0].Content, "second")
	}
}

func TestSeedIgnoresEmpty(t *testing.T) {
	c := NewConversation()
	c.Seed("   ")
	if got := c.Pending("hi"); len(got) != 1 {
		t.Errorf("len(msgs) = %d, want 1, so an empty seed was recorded", len(got))
	}
}

// A multi-line reply occupies one row per line, so it cannot push the frame
// down the screen.
func TestAppendLinesSplitsOnNewlines(t *testing.T) {
	s := &Session{conv: NewConversation()}
	s.appendLines("one\ntwo\nthree")

	if len(s.frame.Reply) != 3 {
		t.Fatalf("len(Reply) = %d, want 3", len(s.frame.Reply))
	}
	if s.frame.Reply[1] != "two" {
		t.Errorf("Reply[1] = %q, want %q", s.frame.Reply[1], "two")
	}
}

func TestAppendLinesTrimsTrailingNewline(t *testing.T) {
	s := &Session{conv: NewConversation()}
	s.appendLines("one\n")
	if len(s.frame.Reply) != 1 {
		t.Errorf("len(Reply) = %d, want 1", len(s.frame.Reply))
	}
}

// A partial reply is shown in place of the pane rather than appended per token.
func TestRenderShowsPartial(t *testing.T) {
	lines := Render(Frame{Reply: []string{"> hi"}, Partial: "typing"}, 8, 40)
	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "typing") {
		t.Errorf("frame = %q, want the partial text", body)
	}
	if !strings.Contains(body, "> hi") {
		t.Errorf("frame = %q, want the question kept", body)
	}
}

func TestModelSelection(t *testing.T) {
	c := NewConversation()
	if c.Model() != "" {
		t.Errorf("Model = %q, want empty before a choice", c.Model())
	}
	c.SetModel("  openai/gpt-4o  ")
	if c.Model() != "openai/gpt-4o" {
		t.Errorf("Model = %q, want the trimmed value", c.Model())
	}
}

func TestOrDash(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want a dash", got)
	}
	if got := orDash("x"); got != "x" {
		t.Errorf("orDash(\"x\") = %q, want it unchanged", got)
	}
}

// The counters accumulate across the session, since a session is the unit a
// reader cares about rather than a single exchange.
func TestAddTokensAccumulates(t *testing.T) {
	c := NewConversation()
	c.AddTokens(10, 20)
	c.AddTokens(5, 7)

	if c.TokensIn() != 15 {
		t.Errorf("TokensIn = %d, want 15", c.TokensIn())
	}
	if c.TokensOut() != 27 {
		t.Errorf("TokensOut = %d, want 27", c.TokensOut())
	}
}

// A reported zero is not a count, so it must not clear an accumulated total.
func TestAddTokensIgnoresZero(t *testing.T) {
	c := NewConversation()
	c.AddTokens(10, 20)
	c.AddTokens(0, 0)

	if c.TokensIn() != 10 || c.TokensOut() != 20 {
		t.Errorf("tokens = %d/%d, want 10/20", c.TokensIn(), c.TokensOut())
	}
}

func TestTokenCount(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "0"}, {999, "999"}, {1000, "1.0k"}, {1500, "1.5k"},
		{1_000_000, "1.0M"}, {2_500_000, "2.5M"},
	} {
		if got := tokenCount(tc.in); got != tc.want {
			t.Errorf("tokenCount(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
