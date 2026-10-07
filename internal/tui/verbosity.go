package tui

import (
	"fmt"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The level at which the model is asked to answer.
//
// The level is asked for in a system turn on every request rather than written
// into the reader's message, so that what the reader typed is what the reader
// sees and what is recorded, and so that changing the level does not become part
// of the conversation a later turn replays.
//
// The system turn is not recorded. It is an instruction about the answer rather
// than part of the exchange, and a save that carried it would replay it to a
// model answering at a level the reader had since changed.

// verbosityMarker is what a verbosity turn begins with.
//
// It is a marker rather than a whole sentence so that the turn can be told from
// a bootstrap document or a compaction summary, all of which are system turns
// too, and so that a resumed session can be recognised as carrying one.
const verbosityMarker = "[verbosity]"

// The names of the levels, in the order they are ordered by how much is asked
// for. The names are the numbers the reader types, and the descriptions are
// what the model is told, so that the two cannot drift apart.
var verbosityLevels = []struct {
	// name is what a refusal of the request means, in words.
	name string
	// asks is the instruction given to the model.
	asks string
}{
	{
		name: "single token",
		asks: "Answer with a single token and nothing else. No sentence, no " +
			"punctuation beyond what the token itself needs. When the question " +
			"is whether to do something, answer the decision alone: your \"y\" " +
			"got \"y\", your \"a\" got acted on.",
	},
	{
		name: "direct, one sentence",
		asks: "Answer in one sentence that gives the answer directly. No " +
			"elaboration, no examples, no caveats unless asked for.",
	},
	{
		name: "answer and one caveat",
		asks: "Give the answer, then one caveat that a reader acting on it " +
			"would want to know. One caveat only, and nothing beyond it.",
	},
	{
		name: "short, with a reason",
		asks: "Give a short answer and name the reason for it, so the reader " +
			"can tell a reasoned answer from a guess.",
	},
	{
		name: "cited files and lines",
		asks: "Give a structured answer, and cite the files and line numbers it " +
			"rests on, each located precisely enough to be opened.",
	},
	{
		name: "cited, and what was not checked",
		asks: "Give a structured answer with the files and lines cited, and " +
			"state what was not checked, what could not be confirmed, and any " +
			"contradiction found along the way.",
	},
	{
		name: "exhaustive audit",
		asks: "Answer exhaustively: every file touched, every alternative " +
			"considered, and every open question left. This is an audit rather " +
			"than a report, and a reader asking this is asking what was not " +
			"done as much as what was.",
	},
}

// DefaultVerbosity is the level a session starts at, and the level the
// configuration file falls back to when it names none or names one that is not
// a level.
//
// Three rather than one because one is a reply of a single token and six is an
// audit, and neither is what a reader wants for an ordinary exchange. Three
// answers the question and names the reason, which is the shape most questions
// turn out to want.
const DefaultVerbosity = 3

// clampVerbosity holds a level to the ones that exist.
//
// A level outside the range is brought to the nearest end rather than refused,
// since a reader who typed 7 meant a lot and a reader who typed -1 meant a
// little. Refusing would leave them with the default and no way to tell that
// what they asked for was not what they got.
func clampVerbosity(n int) int {
	if n < 0 {
		return 0
	}
	if n > len(verbosityLevels)-1 {
		return len(verbosityLevels) - 1
	}
	return n
}

// verbosityTurn is the system turn that asks for the level.
//
// It is named by its marker and states the level, so that a model reading a
// resumed conversation can tell what it was asked for without the reader having
// said so in any turn of it.
func verbosityTurn(level int) openrouter.Message {
	return openrouter.Message{
		Role: openrouter.RoleSystem,
		Content: verbosityMarker + " " + itoa(clampVerbosity(level)) + "\n\n" +
			"Answer at this verbosity level: " + verbosityLevels[clampVerbosity(level)].asks,
	}
}

// isVerbosity reports whether a system turn is the verbosity one.
//
// It is told from the marker rather than from its position, since a system turn
// may also be a bootstrap document or a compaction summary.
func isVerbosity(content string) bool {
	return strings.HasPrefix(content, verbosityMarker+" ")
}

// cmdVerbosity reports or sets the level at which the model is asked to answer.
//
// The session changes first, and the level is also recorded in the
// configuration file, on the same terms as /color and /bell: a reader who
// has said it once should not have to say it again on every run, and a file
// that cannot take the change is reported in one line with nothing from the
// file in it.
func (s *Session) cmdVerbosity(args []string) bool {
	if len(args) == 0 {
		s.appendLines(strings.Join(s.verbosityListing(), "\n"))
		return false
	}

	want := strings.ToLower(strings.Join(args, " "))
	level, ok := parseVerbosity(want)
	if !ok {
		s.addReply("(/verbosity takes a level from 0 to " +
			itoa(len(verbosityLevels)-1) + "; not " + want + ")")
		return false
	}

	s.mu.Lock()
	s.verbosity = level
	save := s.verbositySaver
	s.mu.Unlock()

	state := "answers are asked for at level " + itoa(level) + ", " +
		verbosityLevels[level].name + "."
	err := config.ErrNoConfigFile
	if save != nil {
		err = save(level)
	}
	if err != nil {
		state += " (not saved: " + strings.Join(strings.Fields(err.Error()), " ") + ")"
	}
	s.appendLines(state)
	return false
}

// SetVerbositySaver installs the function /verbosity calls to record the new
// level. A session with none changes for the session only and says so, on
// the same terms as SetColorSaver.
func (s *Session) SetVerbositySaver(save func(level int) error) {
	s.mu.Lock()
	s.verbositySaver = save
	s.mu.Unlock()
}

// parseVerbosity reads a level from what the reader typed.
//
// A name is taken as well as a number, since a reader who has read the listing
// knows what level four is called and may type that.
func parseVerbosity(want string) (int, bool) {
	for i, level := range verbosityLevels {
		if want == itoa(i) {
			return i, true
		}
		// A name is matched whole rather than by its opening words, since the
		// names are phrases and two of them begin the same way. A name that is
		// a word of a longer one is taken, since the extra word is what
		// distinguishes them and a reader who left it off meant the shorter.
		if want == level.name || want == firstWords(level.name) {
			return i, true
		}
	}
	return 0, false
}

// verbosityListing says what the level is and what the levels are.
func (s *Session) verbosityListing() []string {
	s.mu.Lock()
	level := s.verbosity
	s.mu.Unlock()

	lines := []string{fmt.Sprintf("verbosity: %d, %s", level, verbosityLevels[level].name)}
	for i, l := range verbosityLevels {
		mark := "  "
		if i == level {
			mark = "* "
		}
		lines = append(lines, fmt.Sprintf("%s%d  %s", mark, i, l.name))
	}
	return lines
}

// level asks for the level on every request, whether or not the reader has
// mentioned it.
//
// It is added rather than recorded, and it is added to the request rather than
// to the conversation, so that a reader who does not know it is set still gets
// it and a save of the conversation does not carry an instruction about how to
// answer rather than part of what was said.
func verbosityInsert(messages []openrouter.Message, level int) []openrouter.Message {
	out := make([]openrouter.Message, 0, len(messages)+1)
	// The turn goes after any system turn already there and before the
	// exchange, so that a bootstrap document's instructions stand and the level
	// refines them rather than replacing them.
	inserted := false
	for _, m := range messages {
		out = append(out, m)
		if !inserted && m.Role == openrouter.RoleSystem {
			out = append(out, verbosityTurn(level))
			inserted = true
		}
	}
	if !inserted {
		// A conversation with no system turn has nothing to follow, so the
		// turn leads rather than trails the reader's first question.
		out = append([]openrouter.Message{verbosityTurn(level)}, out...)
	}
	return out
}

// cfgVerbosity is the level the configuration file asks for, or the default
// where the file asks for none.
//
// A file naming a level outside the range is brought to the nearest end rather
// than refused, on the same reasoning as a reader typing one: what they meant
// was the most or the least, and refusing would leave them at the default with
// nothing saying so.
func cfgVerbosity(cfg *config.Config) int {
	if cfg == nil || cfg.Verbosity == nil {
		return DefaultVerbosity
	}
	return clampVerbosity(*cfg.Verbosity)
}

// firstWords is the leading word of a level name.
//
// The names are phrases and a reader types the word that identifies one rather
// than the whole phrase. It is matched only where it is unambiguous: two names
// beginning with the same word are not, and a reader who typed that word meant
// neither in particular.
func firstWords(name string) string {
	i := strings.IndexByte(name, ' ')
	if i < 0 {
		return name
	}
	first := name[:i]
	for _, other := range verbosityLevels {
		if other.name != name && strings.HasPrefix(other.name, first) {
			return ""
		}
	}
	return first
}
