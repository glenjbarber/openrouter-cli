package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The name the shell tool is offered under. It is a single tool rather than one
// per program, so that a program added to the allowlist needs no new schema and
// no new name in the catalogue.
const shellTool = "shell"

// ShellTimeout bounds a single command.
//
// A timeout is required rather than trusted to the program finishing, since the
// process is outside this one. A command that blocks returns an error rather
// than waiting for a reader who cannot see that it has, which is the same
// reason the git tool carries a deadline. The figure is longer than the git
// one, since a build legitimately takes longer than a status.
const ShellTimeout = 2 * time.Minute

// shellWaitDelay is how long the wait continues after a command was stopped at
// its deadline, before the output pipe is closed and the wait returns. It is
// the same value and for the same reason as the git tool: a killed program can
// leave a child of its own holding the pipe.
const shellWaitDelay = 500 * time.Millisecond

// shellPermitted is the set of programs the tool will run at all.
//
// It is an allowlist rather than a list of the programs that do damage, on the
// same reasoning as the git subcommands: a blocklist is defeated by every
// program nobody thought of, and by every one added to the host later. Unlike
// git, a program on this list is not thereby allowed to run. Each call asks
// the reader first, so the list bounds what may be proposed rather than what
// may happen.
var shellPermitted = []string{
	"go",
	"gofmt",
	"make",
	"git",
	"ls",
	"cat",
	"pwd",
	"echo",
	"grep",
	"find",
	"wc",
	"head",
	"tail",
	"sed",
	"awk",
	"ps",
}

// shellPermittedMap is shellPermitted as a lookup, so that the two cannot drift
// apart as separate lists would.
var shellPermittedMap = func() map[string]bool {
	m := make(map[string]bool, len(shellPermitted))
	for _, name := range shellPermitted {
		m[name] = true
	}
	return m
}()

// shellParameters is the schema the shell tool is offered with.
const shellParameters = `{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The program to run, such as go. It must be one of: go, gofmt, make, git, ls, cat, pwd, echo, grep, find, wc, head, tail, sed, awk, ps. Anything else is refused before it runs."
    },
    "args": {
      "type": "array",
      "items": {"type": "string"},
      "description": "The arguments after the program, such as [\"build\", \"./...\"]. They are passed to the program as an array and are never read by a shell, so a pipe, a redirect or a chain of commands is not expressible here."
    },
    "path": {
      "type": "string",
      "description": "A directory inside the working directory to run the command in. It is the working directory itself when it is not given."
    }
  },
  "required": ["command"],
  "additionalProperties": false
}`

// Approver decides whether one command may run.
//
// The tool asks and does not decide, and the answer to the question is reached
// on the other side of this interface. The interface is passed in rather than
// reached for, since a tool that drew its own prompt would need the interface
// that drew the conversation, and this package keeps execution away from it on
// purpose. A nil approver refuses every call, so a session assembled without
// one offers no shell rather than one that cannot ask.
type Approver interface {
	// Approve reports whether the command may run in dir. It is called once
	// per call and may be called again for a later call of the same program.
	Approve(command string, args []string, dir string) bool
}

// approveFunc adapts a function to the Approver interface.
type approveFunc func(command string, args []string, dir string) bool

// Approve calls the function.
func (f approveFunc) Approve(command string, args []string, dir string) bool {
	return f(command, args, dir)
}

// AlwaysAllow approves every call.
//
// It exists for the tests of the tools themselves, which are about the running
// of a program rather than about asking a reader, and for a reader who has
// said so in the configuration file.
func AlwaysAllow() Approver { return approveFunc(func(string, []string, string) bool { return true }) }

// shellTools is the state the shell tool holds.
type shellTools struct {
	// dir is the working directory the tool is contained to.
	dir string
	// approver is asked before each command.
	approver Approver
	// timeout bounds one command.
	timeout time.Duration
}

// NewShell returns the shell tool contained to dir, asking approver first.
//
// A nil dir or a nil approver yields a set holding no tools, since a shell that
// cannot ask would be the one thing this client must never offer: it runs
// arbitrary programs on the host with the reader's own privileges.
func NewShell(dir string, approver Approver) *Set {
	return newShell(dir, approver, ShellTimeout)
}

// newShell takes the timeout rather than reading the constant, so that a test
// can give a command a deadline short enough to sit and wait for.
func newShell(dir string, approver Approver, timeout time.Duration) *Set {
	s := New()
	if dir == "" || approver == nil {
		return s
	}
	sh := &shellTools{dir: dir, approver: approver, timeout: timeout}
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name: shellTool,
			Description: "Run a program in the working directory and return what it " +
				"printed, such as go build ./... or make check. The reader is asked " +
				"before each program runs, and a program outside the list is refused. " +
				"The arguments are given to the program directly and are never read by " +
				"a shell.",
			Parameters: json.RawMessage(shellParameters),
		},
	}, sh.call)
	return s
}

// callArguments is what a model sends for one command.
type callArguments struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Path    *string  `json:"path"`
}

// call is the body of the shell tool.
//
// The order of the checks is the order the failures are worth reporting in. The
// program is checked against the allowlist before anything is asked of the
// reader, since a program that would be refused outright is not something to
// interrupt somebody about. The path is resolved next, so that the question
// asked names the directory the command would run in. The reader is asked last,
// once there is something specific to ask about.
func (sh *shellTools) call(raw json.RawMessage) (string, error) {
	var args callArguments
	if err := decode(raw, &args); err != nil {
		return "", err
	}

	program := strings.TrimSpace(args.Command)
	if program == "" {
		return "", errors.New("the \"command\" argument is required and was not given")
	}
	if !shellPermittedMap[program] {
		return "", fmt.Errorf("%s is not permitted: this tool runs %s, and the reader "+
			"is asked about each one", program, permittedPrograms())
	}
	dir, err := sh.workDir(args.Path)
	if err != nil {
		return "", err
	}

	// The question is asked with the resolved directory rather than the one
	// that was asked for, since a reader approving a command in a directory
	// they were not shown would be approving something other than what runs.
	if !sh.approver.Approve(program, args.Args, dir) {
		return "", fmt.Errorf("the reader did not approve %s", Describe(program, args.Args))
	}

	return sh.run(program, dir, args.Args)
}

// Describe spells a command for a question, a refusal, or a pane line, so that
// all three name the same thing in the same words. The arguments are joined with
// a space and quoted where they carry one, since a command shown as
// `go build ./...` is readable and a command shown as an array is not.
//
// It is exported rather than spelled again at each site, since a reader who
// approves `go build ./...` and is then shown the call as an array has been
// shown something other than what they agreed to.
func Describe(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	var b strings.Builder
	b.WriteString(command)
	for _, arg := range args {
		b.WriteByte(' ')
		if strings.ContainsAny(arg, " \t\"'\\$`&|;<>()*?[]{}#~!") {
			b.WriteString(fmt.Sprintf("%q", arg))
		} else {
			b.WriteString(arg)
		}
	}
	return b.String()
}

// permittedPrograms spells the permitted programs for a refusal message. The
// order is the order they are declared rather than sorted, since the
// declaration groups the build tools first and a model reading the refusal
// looks for a name near what it wanted.
func permittedPrograms() string {
	return strings.Join(shellPermitted, ", ")
}

// workDir resolves the directory a command runs in.
//
// The check is the same as the git tool makes, for the same reason: the
// subprocess is handed a real path rather than an os.Root, so the resolved path
// is compared against the resolved working directory as well as the cleaned one
// being compared for a parent reference. A path leaving the tree is refused
// before the filesystem is asked about it, so that a refusal does not depend on
// whether the path happens to exist.
func (sh *shellTools) workDir(path *string) (string, error) {
	name := "."
	if path != nil && *path != "" {
		name = *path
	}
	if name == "." {
		return sh.dir, nil
	}

	if escapes(name) {
		return "", fmt.Errorf("refused: %q is outside the working directory at %s", name, sh.dir)
	}

	if _, err := os.Stat(filepath.Join(sh.dir, name)); err != nil {
		return "", pathError("cannot work in", name, err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(sh.dir, name))
	if err != nil {
		return "", pathError("cannot work in", name, err)
	}
	top, err := filepath.EvalSymlinks(sh.dir)
	if err != nil {
		return "", fmt.Errorf("the working directory %s cannot be resolved: %w", sh.dir, err)
	}
	rel, err := filepath.Rel(top, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refused: %q is outside the working directory at %s", name, sh.dir)
	}
	return resolved, nil
}

// run executes the program and returns what it printed.
//
// The arguments are given to the process as an array rather than to a shell,
// since a shell reads an argument as a command and these come from a model.
// That is the whole reason the tool does not offer pipes, redirects or &&:
// each is a feature of a shell rather than of a program, and offering them
// here would mean running one.
func (sh *shellTools) run(program, dir string, argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sh.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, program, argv...)
	cmd.Dir = dir
	// The environment is inherited rather than trimmed. A build needs PATH,
	// HOME for the module cache, and the variables the host sets for a
	// compiler, and a tool that ran with an empty environment would fail in
	// ways a reader would report as the tool being broken. What it does not do
	// is add anything: GIT_TERMINAL_PROMPT and friends are set by the caller
	// where the program needs them.
	cmd.Env = os.Environ()

	stdout := capped{limit: MaxOutput}
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = shellWaitDelay

	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s did not finish within %s and was stopped", program, sh.timeout)
	}
	if stdout.over {
		return "", fmt.Errorf("%s printed more than the %d byte limit and was not "+
			"returned, so it is reported rather than cut short; ask for less of it",
			program, int64(MaxOutput))
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s failed: %s", program, detail)
	}

	// A program that succeeded and said nothing still reports what it did,
	// since an empty result reads to a model as though the call was dropped.
	if stdout.buf.Len() == 0 {
		return fmt.Sprintf("%s ran in %s and printed nothing.", Describe(program, argv), dir), nil
	}
	return stdout.buf.String(), nil
}
