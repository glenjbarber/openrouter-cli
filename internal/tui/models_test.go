package tui

import (
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
