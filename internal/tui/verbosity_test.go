package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The level is asked for on every request, and is not part of the conversation,
// since it is an instruction about how to answer rather than part of what was
// said.

func verbositySession() *Session {
	conv := NewConversation()
	return &Session{conv: conv, mainConv: conv, approvals: newApprovalState()}
}

// A request carries the level, whatever the reader has asked for.
func TestARequestCarriesTheLevel(t *testing.T) {
	conv := NewConversation()

	for level := range verbosityLevels {
		msgs := conv.PendingMessages([]openrouter.Message{
			{Role: openrouter.RoleUser, Content: "a question"},
		}, level)

		var found bool
		for _, m := range msgs {
			if isVerbosity(m.Content) {
				found = true
				if !strings.Contains(m.Content, itoa(level)) {
					t.Errorf("level %d asked for %q", level, m.Content)
				}
			}
		}
		if !found {
			t.Errorf("level %d was not asked for", level)
		}
	}
}

// The turn is not recorded, since a save carrying it would replay it to a model
// answering at a level the reader had since changed.
func TestTheLevelIsNotRecorded(t *testing.T) {
	conv := NewConversation()
	conv.Record("a question", "an answer")

	msgs := conv.PendingMessages([]openrouter.Message{
		{Role: openrouter.RoleUser, Content: "another"},
	}, 5)

	for _, m := range conv.Messages() {
		if isVerbosity(m.Content) {
			t.Errorf("the level was recorded: %q", m.Content)
		}
	}
	if len(msgs) <= len(conv.Messages()) {
		t.Errorf("the level was not added to the request: %d turns, %d recorded",
			len(msgs), len(conv.Messages()))
	}
}

// A bootstrap document's instructions stand, and the level refines them rather
// than replacing them, so the turn goes after it.
func TestTheLevelFollowsTheOpeningInstructions(t *testing.T) {
	conv := NewConversation()
	conv.Seed("the reader is on FreeBSD")

	msgs := conv.PendingMessages([]openrouter.Message{
		{Role: openrouter.RoleUser, Content: "a question"},
	}, 2)

	if len(msgs) < 3 {
		t.Fatalf("the request is %d turns: %+v", len(msgs), msgs)
	}
	if msgs[0].Content != "the reader is on FreeBSD" {
		t.Errorf("the document is not first: %q", msgs[0].Content)
	}
	if !isVerbosity(msgs[1].Content) {
		t.Errorf("the level does not follow the document: %+v", msgs)
	}
}

// A session with no document leads with the level rather than trailing it
// after the reader's question, where it would read as an answer to it.
func TestTheLevelLeadsWithNoDocument(t *testing.T) {
	conv := NewConversation()

	msgs := conv.PendingMessages([]openrouter.Message{
		{Role: openrouter.RoleUser, Content: "a question"},
	}, 3)

	if !isVerbosity(msgs[0].Content) {
		t.Errorf("the level does not lead: %+v", msgs)
	}
}

// The default is three, which answers the question and names the reason.
func TestTheDefaultLevelIsThree(t *testing.T) {
	if DefaultVerbosity != 3 {
		t.Errorf("the default is %d, want 3", DefaultVerbosity)
	}
	if got := cfgVerbosity(nil); got != DefaultVerbosity {
		t.Errorf("a session with no file starts at %d", got)
	}
}

// A file asking for a level is obeyed, and one asking for none is not.
func TestTheFileNamesTheStartingLevel(t *testing.T) {
	five := 5
	zero := 0

	if got := cfgVerbosity(&config.Config{Verbosity: &five}); got != 5 {
		t.Errorf("the file asked for 5 and the session started at %d", got)
	}
	// Zero is a level in its own right, so a file asking for it is not read as
	// a file that said nothing.
	if got := cfgVerbosity(&config.Config{Verbosity: &zero}); got != 0 {
		t.Errorf("the file asked for 0 and the session started at %d", got)
	}
	if got := cfgVerbosity(&config.Config{}); got != DefaultVerbosity {
		t.Errorf("a file saying nothing started at %d", got)
	}
}

// A level outside the range is brought to the nearest end rather than refused,
// since a reader who typed seven meant a lot.
func TestALevelOutsideTheRangeIsBroughtToTheEnd(t *testing.T) {
	if got := clampVerbosity(7); got != len(verbosityLevels)-1 {
		t.Errorf("level 7 became %d", got)
	}
	if got := clampVerbosity(-1); got != 0 {
		t.Errorf("level -1 became %d", got)
	}
}

// The command sets the level and refuses one it does not know.
func TestTheCommandSetsAndRefuses(t *testing.T) {
	s := verbositySession()

	s.cmdVerbosity([]string{"5"})
	if got := s.verbosityLevel(); got != 5 {
		t.Errorf("the command left the level at %d", got)
	}

	s.cmdVerbosity([]string{"perhaps"})
	if got := s.verbosityLevel(); got != 5 {
		t.Errorf("an unknown level changed it to %d", got)
	}
}

// The listing names every level and marks the one in force.
func TestTheListingNamesEveryLevel(t *testing.T) {
	s := verbositySession()
	s.cmdVerbosity([]string{"4"})

	joined := strings.Join(s.verbosityListing(), "\n")
	for i, level := range verbosityLevels {
		if !strings.Contains(joined, itoa(i)+"  "+level.name) {
			t.Errorf("the listing does not carry level %d: %s", i, joined)
		}
	}
	if !strings.Contains(joined, "* 4 ") {
		t.Errorf("the listing does not mark the level in force: %s", joined)
	}
}

// A level is read by its name as well as its number, since a reader who has
// read the listing may type what it is called.
func TestALevelIsReadByItsName(t *testing.T) {
	if got, ok := parseVerbosity("exhaustive"); !ok || got != 6 {
		t.Errorf("exhaustive read as %d, ok=%v", got, ok)
	}
	if got, ok := parseVerbosity("3"); !ok || got != 3 {
		t.Errorf("3 read as %d, ok=%v", got, ok)
	}
	if _, ok := parseVerbosity("nonsense"); ok {
		t.Error("nonsense was read as a level")
	}
}
