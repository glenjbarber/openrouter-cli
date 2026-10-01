package complete

import "strings"

// Kind is what the completer found at a caret.
//
// It is reported rather than returned as a bare line so that the caller can
// tell a completed line from one that was left alone, since the two differ
// only in whether anything should replace what is being composed.
type Kind int

const (
	// NotApplicable means the caret is not at a position where completion
	// means anything, such as an argument to a command or the middle of a
	// sentence. No candidate is offered, since there is nothing to offer
	// one for.
	NotApplicable Kind = iota
	// NoMatch means a position that could be completed matched no candidate.
	// It is distinct from NotApplicable, since a reader who has mistyped the
	// first few letters of a command should be told so rather than told that
	// there is nothing to complete.
	NoMatch
	// Ambiguous means several candidates matched. The line is left as it is,
	// because choosing between them would pick a word the reader did not ask
	// for, and the candidates are reported instead.
	Ambiguous
	// Unique means exactly one candidate matched, and the line is returned
	// with it substituted in.
	Unique
)

// String names the outcome. It is what a report uses in place of a numeral,
// since a numeral in the pane would say nothing.
func (k Kind) String() string {
	switch k {
	case NotApplicable:
		return "not applicable"
	case NoMatch:
		return "no match"
	case Ambiguous:
		return "ambiguous"
	case Unique:
		return "unique"
	default:
		return "unknown"
	}
}

// Candidate is one thing a caret may be completed to.
//
// The description travels with the name rather than being looked up, so that
// a listing of candidates and the help cannot describe the same word in two
// different ways.
type Candidate struct {
	Name        string
	Description string
}

// Result is the outcome of a completion at one caret.
type Result struct {
	// Kind is what the completer found.
	Kind Kind
	// Set names what was offered, such as commands or options, so that a
	// report can say which it is listing rather than leaving the reader to
	// infer it from the candidates.
	Set string
	// Prefix is the token that was being completed, which is what a report
	// names when it says what matched and what did not.
	Prefix string
	// Line is the composed line with the completed token substituted in. It
	// is the whole line rather than the token alone, so that a caller
	// holding the line can take it as it stands. It equals the input line
	// unchanged unless Kind is Unique.
	Line string
	// Candidates are the matches, in the order the set declares them so that
	// a listing is the same on every run.
	Candidates []Candidate
}

// Applied reports whether the line was completed to a single candidate.
//
// It is what a caller checks before replacing the line it is holding, since a
// line is only worth replacing when a word was chosen for it.
func (r Result) Applied() bool { return r.Kind == Unique }

// Completer completes the token at a caret in a line being composed.
//
// The command candidates are supplied by the caller rather than declared here.
// The names are the interface vocabulary and live beside the code that
// dispatches them, and a second copy of them in this package would be the
// third list to drift from the first two.
type Completer struct {
	commands []Candidate
}

// New returns a Completer that offers the given commands alongside the
// configuration option names.
func New(commands []Candidate) Completer { return Completer{commands: commands} }

// Complete returns the completion of the token the caret sits in.
//
// The caret is a byte offset, as the line editor reports it. An offset
// outside the line is brought inside it rather than refused, since a caller
// that has lost track of the offset is still owed whatever can be worked out
// from the line itself.
func (c Completer) Complete(line string, caret int) Result {
	caret = clamp(caret, 0, len(line))
	pos, start, end := positionAt(line, caret)
	if pos == posNone {
		return Result{Kind: NotApplicable, Line: line}
	}

	set, name := c.commands, "commands"
	if pos == posOption {
		set, name = options, "options"
	}
	prefix := line[start:caret]

	res := Result{
		Set:        name,
		Prefix:     prefix,
		Line:       line,
		Candidates: filterPrefix(set, prefix),
	}
	switch len(res.Candidates) {
	case 0:
		res.Kind = NoMatch
	case 1:
		res.Kind = Unique
		// The whole token is replaced rather than only the part before the
		// caret, so that a caret in the middle of a token still leaves the
		// token whole. Nothing to either side of it is lost.
		res.Line = line[:start] + res.Candidates[0].Name + line[end:]
	default:
		res.Kind = Ambiguous
	}
	return res
}

// position is what may be completed at a caret. It is decided by where the
// caret sits rather than by what the text around it says, so that the same
// word is offered in one position and refused in another.
type position int

const (
	// posNone means nothing may be completed at the caret.
	posNone position = iota
	// posCommand means the caret is in the command name at the head of the
	// line.
	posCommand
	// posOption means the caret is in a bare single word, which is the only
	// place a configuration option name is completed.
	posOption
)

// positionAt reports what may be completed at the caret, and the bounds of the
// token the caret is in.
//
// The bounds are returned as well as the position because a completion
// replaces the whole token, not only the part of it before the caret.
func positionAt(line string, caret int) (pos position, start, end int) {
	// With nothing before the caret there is no token to complete, whether
	// the line is empty or the caret is at the head of a line already
	// holding text.
	if caret == 0 {
		return posNone, 0, 0
	}

	if line[0] == '/' {
		// The command name is the first word of the line. A caret up to the
		// end of that word is completing the name, and one past it is in an
		// argument, which is left alone: what a command takes as an argument
		// is its own business, and completing it would be guessing at it.
		first := len(line)
		if i := strings.IndexAny(line, " \t"); i >= 0 {
			first = i
		}
		if caret > first {
			return posNone, 0, 0
		}
		return posCommand, 0, first
	}

	// A configuration option is one word, so it is completed only while the
	// line is still a single word. Once the line carries a space it is a
	// message being written, and completing a bare word inside it would put
	// a setting where the reader was writing prose.
	if strings.ContainsAny(line, " \t") {
		return posNone, 0, 0
	}
	return posOption, 0, len(line)
}

// options are the keys the configuration file is written with, which are named
// after the environment variables they correspond to.
//
// They are completed only as a whole single word, so that the completer offers
// one of them rather than expanding a word the reader was part way through
// writing in a message.
var options = []Candidate{
	{"OPENROUTER_API_KEY", "the credential, sent as a bearer header, required"},
	{"OPENROUTER_URL_BASE", "the endpoint, overriding the default base URL"},
	{"OPENROUTER_MODEL", "the model a session starts with"},
	{"OPENROUTER_MOUSE", "ask for mouse reporting on every run"},
	{"OPENROUTER_BELL", "ring the terminal bell when a reply arrives"},
}

// filterPrefix returns the candidates whose names begin with prefix, without
// regard to case.
//
// The case is ignored because a reader types a word in whatever case they
// happen to use, which is the same reason the model filter matches without
// regard to it. The matches keep the order of the set, so that a listing is
// the same on every run.
func filterPrefix(set []Candidate, prefix string) []Candidate {
	lower := strings.ToLower(prefix)
	var out []Candidate
	for _, c := range set {
		if strings.HasPrefix(strings.ToLower(c.Name), lower) {
			out = append(out, c)
		}
	}
	return out
}

// clamp brings a value inside the range low to high, both inclusive.
func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
