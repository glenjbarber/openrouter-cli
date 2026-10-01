package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// maxToolRounds is how many requests one turn may make before it is stopped.
//
// A model with tools can ask for the same file for ever, and the reader can
// stop it at any time, so the reader is not the only bound. A cap is what stops
// a loop that does not converge: without one a model asking for the same thing
// in every round spends the allowance and returns for ever, which reads as a
// hang rather than as a fault. The figure is a number of requests rather than
// a time, since what a loop costs is requests.
const maxToolRounds = 8

// The labels the pane draws a call under.
//
// A label rather than the name alone, since a reader scanning the pane reads
// which part of the session acted and not which function was called. They are
// what the tools were built from, which is also what /tools reports, so the two
// cannot name the same tool differently.
const (
	fsToolLabel  = "fs"
	gitToolLabel = "git"
)

// toolSet is the tools a session offers, and what the pane needs to say about
// them.
//
// The set is built once at startup and held for the session, since a root is
// taken from the working directory once and a set rebuilt per turn would open a
// descriptor per turn to contain the same directory.
type toolSet struct {
	// set is what a request offers and what a call is run by.
	set *tools.Set
	// labels names the part of the set a tool came from, since the name
	// alone does not say whether it touched the filesystem or the repository.
	labels map[string]string
	// dir is the directory the tools are contained to, reported by /tools so
	// that a reader can see what a model is able to reach.
	dir string
	// problem says what could not be built and what it costs. It is empty
	// when everything was built. An absence is reported rather than treated
	// as a reason not to start: the interface opens and says so, in the
	// manner of a session with no API key, since a reader is better served by
	// a chat client with no tools than by nothing at all.
	problem string
}

// workingDir is the directory the tools are contained to.
//
// It is the process working directory, read once at startup, which is the
// directory the reader launched the client from. There is no command that
// changes it, so the root does not move: a tool contained to the tree the
// client was opened in is contained to that tree for the whole session, and a
// root that moved would be a root the reader never chose.
//
// A directory that cannot be read is reported rather than refused, and the
// session opens without tools. A reader whose working directory has gone away
// still gets a working chat client, told what is missing rather than left with
// an interface that will not start.
func workingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// toolsAt returns the tools contained to dir.
//
// The filesystem tools are built on an os.Root rather than on a path compared
// for a prefix, since a prefix check on a cleaned path is defeated by a
// symlink inside the tree pointing out of it. The root is the open descriptor,
// and the directory is opened once so that nothing after it can be swapped for
// something else.
func toolsAt(dir string) *toolSet {
	t := &toolSet{
		set:    tools.New(),
		labels: map[string]string{},
		dir:    dir,
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.problem = fmt.Sprintf("no filesystem tools: %v, so the model cannot read "+
			"or write anything and is asked questions alone", err)
		return t
	}
	t.merge(fsToolLabel, tools.NewFilesystem(root))

	// A directory that is not a repository is ordinary rather than a fault,
	// and it costs one tool rather than the session, so it is reported and the
	// filesystem tools are kept.
	git, err := tools.NewGit(dir)
	if err != nil {
		t.problem = fmt.Sprintf("no git tool: %v, so the model cannot read the "+
			"repository", err)
		return t
	}
	t.merge(gitToolLabel, git)
	return t
}

// merge copies the tools of one set into the combined set.
//
// The bodies are run through the set they came from rather than through the
// combined one, since a set does not hand out the functions it holds. That is
// what the run method is for: the combined set is the catalogue a request
// offers, not a second copy of every body.
func (t *toolSet) merge(label string, from *tools.Set) {
	for _, spec := range from.Specs() {
		name := spec.Function.Name
		t.labels[name] = label
		t.set.Add(spec, func(args json.RawMessage) (string, error) {
			r := from.Run(openrouter.ToolCall{
				Function: openrouter.ToolCallFunction{
					Name:      name,
					Arguments: string(args),
				},
			})
			return r.Text, r.Err
		})
	}
}

// specs returns what a request offers, or nothing at all when there are none.
//
// A nil set means a session assembled without one, which is a test and a
// client built before this work. Both send the request they sent before, which
// is the point of the field being omitted when it is empty.
func (t *toolSet) specs() []openrouter.Tool {
	if t == nil || t.set == nil {
		return nil
	}
	return t.set.Specs()
}

// run executes a call, answering with an error when there is no set to run it.
//
// A call cannot be left unanswered whatever happens, since a turn waiting for a
// result that never arrives is reported by a reader as a hang.
func (t *toolSet) run(c openrouter.ToolCall) tools.Result {
	if t == nil || t.set == nil {
		return tools.Result{Call: c, Err: errors.New("this session offers no tools")}
	}
	return t.set.Run(c)
}

// label returns the part of the set a tool came from.
func (t *toolSet) label(name string) string {
	if t == nil {
		return ""
	}
	return t.labels[name]
}

// names returns the tools offered, in the order a request offers them.
func (t *toolSet) names() []string {
	if t == nil || t.set == nil {
		return nil
	}
	return t.set.Names()
}

// absence returns why no tools are offered, or an empty string where they are.
func (t *toolSet) absence() string {
	if t == nil {
		return "this session was started without a tool set"
	}
	return t.problem
}

// turnTools returns the tools a request in this conversation may offer.
//
// The gate is one condition rather than three. An in-cognito session and a
// thread both mark the active conversation ephemeral, so a turn in either sends
// a request with no tools key at all: nothing is named for either mode, and a
// mode added later is covered by the same condition.
//
// The rule is that a tool acts on the reader's behalf, and a mode whose promise
// is that nothing is recorded cannot hand the model a hand that acts. A tool
// call is words with effects, exactly as much as a reply is words.
func (s *Session) turnTools(conv *Conversation) []openrouter.Tool {
	if conv == nil || !conv.Recording() {
		return nil
	}
	return s.tools.specs()
}

// drawToolCall reports one call in the pane.
//
// One line per call, and the result itself is never drawn. A read of a large
// file would bury the conversation under the file, and the reader asked the
// model a question rather than for a file listing. The model is given the whole
// result; the reader is given what happened and how much of it there was.
//
// The line is plain text with nothing set around it, since a selection out of
// the pane has to yield the text with no escape sequence in it. A failure is
// drawn as the reason on the same line, since a call that failed is not a call
// that is still running and a second row for the error would read as one.
func (s *Session) drawToolCall(r tools.Result) {
	name := r.Call.Function.Name
	label := s.tools.label(name)
	if label == "" {
		// A call the session cannot account for is marked rather than given
		// no label, so that an unlabelled line is visibly not one of the
		// tools rather than a line the renderer dropped something from.
		label = "?"
	}
	s.addReply(fmt.Sprintf("[%s] %s %s -> %s", label, name,
		callSummary(r.Call), toolOutcome(r)))
	s.draw()
}

// toolOutcome renders the right hand side of a call line.
//
// A result is reported by its size rather than by its text, since the text is
// what was withheld. A failure is reported by the reason, which is the one thing
// about a call that failed that the reader cannot work out for themselves.
func toolOutcome(r tools.Result) string {
	if r.Err != nil {
		return toolFailure(r.Err)
	}
	return toolSize(len(r.Text))
}

// toolContent returns what goes back to the model for one result.
//
// The whole text of a result is sent, since the model asked for it and cannot
// act on a summary. A failure is sent as well, since a model told only that
// something was refused has nothing to choose its next call from. A body that
// failed partway keeps what it produced, since a partial answer is more use to
// a model than an error on its own.
func toolContent(r tools.Result) string {
	switch {
	case r.Err == nil:
		return r.Text
	case r.Text == "":
		return r.Err.Error()
	default:
		return r.Text + "\n" + r.Err.Error()
	}
}

// toolFailure renders why a call failed.
//
// A reason from the system is shown in its own words rather than in the words
// the tool wrapped it in. The wrapper repeats the operation and the path, and
// the line this goes on names both already, so what is left worth one row is
// the reason itself. A failure a tool wrote itself is shown whole, since
// there the wording is the only description of what went wrong.
func toolFailure(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return cutLine(errno.Error())
	}
	return cutLine(err.Error())
}

// toolSize renders the size of a result.
func toolSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// callArgWidth is how much of an argument a call line carries.
//
// The argument is there so that the reader can tell which file was read, not so
// that the pane holds the arguments. A write carries the whole content of the
// file in its arguments, and drawing it would bury the conversation in the very
// thing the line is about.
const callArgWidth = 48

// callSummary renders the argument a reader wants from a call.
//
// The path is preferred, then the argument list, because those are what the
// tools in this pass take: a git call is identified by its subcommand and a
// filesystem call by its path. Arguments are read as they arrived rather than
// against a schema, since the model chose the names and a summary that dropped
// an argument it did not know about would misdescribe the call.
func callSummary(c openrouter.ToolCall) string {
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(c.Function.Arguments), &args); err != nil {
		return cutLine(c.Function.Arguments)
	}
	if raw, ok := args["path"]; ok {
		var path string
		if json.Unmarshal(raw, &path) == nil {
			return fitColumns(path, callArgWidth)
		}
	}
	if raw, ok := args["args"]; ok {
		var list []string
		if json.Unmarshal(raw, &list) == nil {
			return fitColumns(strings.Join(list, " "), callArgWidth)
		}
	}
	return ""
}

// cutLine folds a piece of text onto one row and cuts it to a width.
//
// A tool line is one row, and text carrying a newline would break it into
// several without saying that they were one thing.
func cutLine(s string) string { return fitColumns(strings.Join(strings.Fields(s), " "), callArgWidth) }

// fitColumns keeps a string within a width, marking it when it is cut.
//
// The marker is the one the renderer uses, so that a cut here and a cut there
// look alike.
func fitColumns(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len([]rune(s)) <= width {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:width-len(ellipsis)]), " ") + ellipsis
}

// toolsListing renders what a reader is told when they ask what the model has.
//
// The root is named first, since a tool that is contained somewhere is a
// question a reader asks before asking what the tools do.
func (t *toolSet) toolsListing() []string {
	names := t.names()
	if len(names) == 0 {
		return []string{"no tools are offered to the model: " + t.absence()}
	}
	dir := t.dir
	if dir == "" {
		dir = "the working directory"
	}
	lines := []string{fmt.Sprintf("the model is given %d tools, contained to %s:",
		len(names), dir)}
	for _, spec := range t.specs() {
		lines = append(lines, fmt.Sprintf("  [%s] %s %s %s",
			t.label(spec.Function.Name), spec.Function.Name,
			schemaArgs(spec.Function.Parameters), spec.Function.Description))
	}
	return lines
}

// schemaProperty is one argument as a tool schema describes it.
type schemaProperty struct {
	Type  string `json:"type"`
	Items *struct {
		Type string `json:"type"`
	} `json:"items"`
}

// schemaArgs renders the arguments a tool takes as they would be written.
//
// The schema is read rather than restated, since a list of arguments written
// out here is a second copy that falls behind the schema the model is actually
// sent. A required argument is listed first and an optional one is marked with
// a question mark, since the reader is looking for what they have to give.
func schemaArgs(parameters json.RawMessage) string {
	var schema struct {
		Properties map[string]schemaProperty `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(parameters, &schema); err != nil {
		// A schema that cannot be read is shown as it stands rather than
		// left out, since a tool shown with no arguments reads as a tool
		// that takes none.
		return cutLine(string(parameters))
	}
	required := make(map[string]bool, len(schema.Required))
	for _, n := range schema.Required {
		required[n] = true
	}
	names := make([]string, 0, len(schema.Properties))
	for n := range schema.Properties {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, n := range names {
		p := schemaProperties(n, schema.Properties)
		if !required[n] {
			p += "?"
		}
		parts = append(parts, n+": "+p)
	}
	if len(parts) == 0 {
		return "()"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// schemaProperties renders the type of one argument.
func schemaProperties(name string, props map[string]schemaProperty) string {
	p := props[name]
	if p.Type == "array" && p.Items != nil && p.Items.Type != "" {
		return "[" + p.Items.Type + "]"
	}
	if p.Type == "" {
		return "?"
	}
	return p.Type
}
