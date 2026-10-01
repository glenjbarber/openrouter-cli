package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

func modelsForTest() []openrouter.Model {
	return []openrouter.Model{
		{ID: "anthropic/claude-sonnet-4"},
		{ID: "openai/gpt-4o"},
		{ID: "openai/gpt-4o-mini", PromptPrice: "0", CompletionPrice: "0"},
		{ID: "stealth/space-bunny-alpha"},
	}
}

// The filter narrows as it is typed, character by character.
func TestModelFilterNarrows(t *testing.T) {
	s := &Session{}
	s.modelList = modelsForTest()

	s.modelFilter = "openai"
	s.pane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "openai/gpt-4o") {
		t.Errorf("frame is missing a matching model:\n%s", body)
	}
	if strings.Contains(body, "anthropic") {
		t.Errorf("frame kept a model that does not match:\n%s", body)
	}
}

// The filter is matched without regard to case, since a model identifier is
// typed in whatever case the user happens to use.
func TestModelFilterIsCaseInsensitive(t *testing.T) {
	s := &Session{}
	s.modelList = modelsForTest()
	s.modelFilter = "GPT"

	s.pane()
	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "openai/gpt-4o") {
		t.Errorf("frame is missing a case-insensitive match:\n%s", body)
	}
}

// A filter that matches nothing must say so rather than showing an empty pane.
func TestModelFilterReportsNoMatch(t *testing.T) {
	s := &Session{}
	s.modelList = modelsForTest()
	s.modelFilter = "nothing here"

	s.pane()
	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "nothing matches") {
		t.Errorf("frame does not report the empty result:\n%s", body)
	}
}

// The free listing narrows before the filter is applied, so a filter is matched
// only against the free models.
func TestFreeFilterNarrowsFirst(t *testing.T) {
	s := &Session{}
	s.modelList = modelsForTest()
	s.modelKeep = func(m openrouter.Model) bool { return m.Free() }
	s.modelFilter = "openai"

	s.pane()
	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "openai/gpt-4o-mini") {
		t.Errorf("frame is missing the free model:\n%s", body)
	}
	if strings.Contains(body, "openai/gpt-4o\n") {
		t.Errorf("frame listed a charged model:\n%s", body)
	}
}

// A long catalogue is cut with the remainder reported, rather than silently
// dropped.
func TestModelListIsCapped(t *testing.T) {
	s := &Session{}
	for i := 0; i < 60; i++ {
		s.modelList = append(s.modelList, openrouter.Model{ID: "m" + string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}

	s.pane()
	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "and") {
		t.Errorf("the overflow was not reported:\n%s", body)
	}
}

// Backspace removes one character of the filter.
func TestModelFilterBackspace(t *testing.T) {
	s := &Session{}
	s.modelFilter = "openai"

	s.modelFilter = trimLastRuneString(s.modelFilter)
	if s.modelFilter != "opena" {
		t.Errorf("filter = %q, want one character removed", s.modelFilter)
	}
}

// The filter closes on escape and the pane is restored.
func TestModelFilterEscapeCloses(t *testing.T) {
	s := &Session{}
	s.modelList = modelsForTest()
	s.modelFilter = "openai"
	s.frame.Reply = []string{"something"}

	s.mu.Lock()
	s.modelList = nil
	s.modelFilter = ""
	s.frame.Reply = nil
	s.mu.Unlock()
	if s.modelList != nil {
		t.Error("the list is still open after escape")
	}
	if s.modelFilter != "" {
		t.Errorf("filter = %q, want it cleared", s.modelFilter)
	}
	if s.frame.Reply != nil {
		t.Error("the pane was not restored")
	}
}

func TestTrimLastRuneString(t *testing.T) {
	if got := trimLastRuneString("abc"); got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
	if got := trimLastRuneString(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	// A multibyte character is removed whole.
	if got := trimLastRuneString("café"); got != "caf" {
		t.Errorf("got %q, want %q", got, "caf")
	}
}

// modelFilterSession returns a session with the catalogue open, and a screen it
// can be drawn to, since completing the filter repaints.
//
// The output is a file rather than a buffer because the frame asks the screen
// for its size, which a buffer cannot answer. The size is set rather than read
// for the same reason, and a size query that fails keeps the figure.
func modelFilterSession(t *testing.T, models []openrouter.Model, keep func(openrouter.Model) bool) *Session {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}
	return &Session{
		conv:      NewConversation(),
		screen:    &Screen{out: out, height: 24, width: 80},
		modelList: models,
		modelKeep: keep,
	}
}

// head returns the first line of the listing.
func head(t *testing.T, s *Session) string {
	t.Helper()
	if len(s.frame.Reply) == 0 {
		t.Fatal("the pane is empty")
	}
	return s.frame.Reply[0]
}

// Tab completes the filter to the first model it matches.
func TestModelFilterTabCompletes(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"

	s.modelListKey(keyTab)

	if s.modelFilter != "openai/gpt-4o" {
		t.Errorf("filter = %q, want the first match", s.modelFilter)
	}
	if !strings.Contains(strings.Join(s.frame.Reply, "\n"), "filter: openai/gpt-4o_") {
		t.Errorf("the listing does not show the completed filter:\n%v", s.frame.Reply)
	}
}

// A second Tab advances, and the cycle wraps at the end of what matched.
func TestModelFilterTabCycles(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"

	s.modelListKey(keyTab)
	s.modelListKey(keyTab)
	if s.modelFilter != "openai/gpt-4o-mini" {
		t.Errorf("filter = %q, want the second match", s.modelFilter)
	}

	s.modelListKey(keyTab)
	if s.modelFilter != "openai/gpt-4o" {
		t.Errorf("filter = %q, want the cycle to wrap", s.modelFilter)
	}
}

// The candidates come from the filter as it was typed rather than from the
// filter as it has been completed. Completing to a whole identifier would
// otherwise leave a filter matching only itself, and the cycle would be one
// Tab long.
func TestModelFilterTabCyclesFromTheTypedFilter(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"

	s.modelListKey(keyTab)
	s.modelListKey(keyTab)

	if got := head(t, s); !strings.Contains(got, "[2 of 2]") {
		t.Errorf("heading = %q, want the place in the cycle", got)
	}
}

// The place in the cycle is stated in the heading, so that a reader cycling
// through a set can tell where in it they are.
func TestModelFilterTabStatesItsPlace(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"
	s.pane()

	if got := head(t, s); strings.Contains(got, "[") {
		t.Errorf("heading = %q before any completion, want no cycle stated", got)
	}
	s.modelListKey(keyTab)
	if got := head(t, s); !strings.Contains(got, "[1 of 2]") {
		t.Errorf("heading = %q, want the first of the set", got)
	}
}

// Typing after a completion ends it, so that the next Tab begins a new cycle
// from what is now typed rather than continuing the old one.
func TestModelFilterTypingEndsTheCycle(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"

	s.modelListKey(keyTab)
	if s.modelFilter != "openai/gpt-4o" {
		t.Fatalf("filter = %q, want the first match", s.modelFilter)
	}

	// A backspace takes the completed identifier back one character, and the
	// cycle with it. Completing from there must begin again rather than
	// resume, since the identifier it completed to matches only itself.
	s.modelListKey(keyBackspace)
	if s.modelCycle != nil {
		t.Fatal("the cycle survived a change to the filter")
	}

	s.modelListKey(keyTab)
	if s.modelFilter != "openai/gpt-4o" {
		t.Errorf("filter = %q, want the first of a new cycle", s.modelFilter)
	}
	if got := head(t, s); !strings.Contains(got, "[1 of 2]") {
		t.Errorf("heading = %q, want a new cycle from the first", got)
	}
}

// A typed character ends a cycle as a backspace does, since both are the reader
// narrowing rather than the reader walking.
func TestModelFilterTypingACharacterEndsTheCycle(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "openai"

	s.modelListKey(keyTab)
	s.modelListKey('z')

	if s.modelCycle != nil {
		t.Error("the cycle survived a typed character")
	}
	if s.modelFilter != "openai/gpt-4oz" {
		t.Errorf("filter = %q, want the character appended", s.modelFilter)
	}
}

// A Tab on a filter that matches nothing changes nothing, since the listing
// already reports that nothing matches and there is nothing to complete.
func TestModelFilterTabWithNoMatchChangesNothing(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)
	s.modelFilter = "nothing here"
	s.pane()

	s.modelListKey(keyTab)

	if s.modelFilter != "nothing here" {
		t.Errorf("filter = %q, want it left as it was", s.modelFilter)
	}
	if s.modelCycle != nil {
		t.Error("a cycle was begun over nothing")
	}
	if !strings.Contains(strings.Join(s.frame.Reply, "\n"), "nothing matches") {
		t.Errorf("the listing does not report the empty result:\n%v", s.frame.Reply)
	}
}

// A Tab on an empty filter cycles the whole listing, since an empty filter
// matches every model the endpoint offers.
func TestModelFilterTabOnEmptyFilter(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), nil)

	s.modelListKey(keyTab)

	if s.modelFilter != "anthropic/claude-sonnet-4" {
		t.Errorf("filter = %q, want the first model in the catalogue", s.modelFilter)
	}
}

// The free listing completes over the free models alone, since the cut is
// applied before the filter is.
func TestFreeModelTabCyclesFreeModelsOnly(t *testing.T) {
	s := modelFilterSession(t, modelsForTest(), func(m openrouter.Model) bool { return m.Free() })
	s.modelFilter = "openai"

	s.modelListKey(keyTab)

	if s.modelFilter != "openai/gpt-4o-mini" {
		t.Errorf("filter = %q, want the only free model matching", s.modelFilter)
	}
}

// The cycle covers the models the pane shows rather than the whole catalogue,
// since a candidate that was never on the screen is one the reader cannot pick
// out from another.
func TestModelFilterTabCyclesOnlyWhatIsShown(t *testing.T) {
	var many []openrouter.Model
	for i := 0; i < 60; i++ {
		many = append(many, openrouter.Model{ID: fmt.Sprintf("m%02d", i)})
	}
	s := modelFilterSession(t, many, nil)

	s.modelListKey(keyTab)

	if got := head(t, s); !strings.Contains(got, fmt.Sprintf("[1 of %d]", maxShown)) {
		t.Errorf("heading = %q, want a cycle over the %d shown", got, maxShown)
	}
	for _, m := range many[maxShown:] {
		for _, id := range s.modelCycle {
			if id == m.ID {
				t.Errorf("the cycle holds %s, which the pane does not show", m.ID)
			}
		}
	}
}
