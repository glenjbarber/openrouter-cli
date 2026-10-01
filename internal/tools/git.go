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
	"slices"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// gitTool is the name the git tool is offered under. The model writes the
// arguments after git, so the name carries the program rather than a verb.
const gitTool = "git"

// gitProgram is the program run, taken from the PATH.
const gitProgram = "git"

// GitTimeout bounds a single git command.
//
// A timeout is required rather than trusted to git finishing, since the process
// is outside this one: nothing else stops a command that blocks on a lock held
// by another process, and a call that never returns is the hang a reader sees.
const GitTimeout = 30 * time.Second

// gitWaitDelay is how long the wait continues after a command was stopped at
// its deadline, before the output pipe is closed and the wait returns.
//
// It is a delay rather than nothing because a killed git can leave a process of
// its own holding the pipe, and a wait that returns only when the pipe closes
// would outlast the deadline by however long that process runs.
const gitWaitDelay = 500 * time.Millisecond

// MaxOutput is the largest output a git command is allowed to produce.
//
// A `git log` on a large repository prints more than a conversation can carry,
// and the whole of it would be spent on a log a model asked the shape of. The
// cap is reported rather than cut, since a log ending mid-commit is a record
// of commits that is read as though it were the whole history.
const MaxOutput = 1 << 20

// gitParameters is the schema the git tool is offered with.
const gitParameters = `{
  "type": "object",
  "properties": {
    "args": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "description": "The git arguments after git, such as [\"status\"] or [\"log\", \"--oneline\"]. Only the read-only subcommands are permitted, and anything else is refused before git is run. Put a separator before a path, as in [\"status\", \"--\", \"--upload-pack=x\"]."
    },
    "path": {
      "type": "string",
      "description": "A directory inside the repository to run the command in. It is the root of the repository when it is not given."
    }
  },
  "required": ["args"],
  "additionalProperties": false
}`

// gitPermitted is the set of subcommands the tool runs.
//
// It is an allowlist rather than a list of the subcommands that write, since a
// blocklist is defeated by every subcommand nobody thought of and by every
// future one added to git. There is no approval mechanism in this pass, so a
// subcommand reaching past this set would change a repository without a reader
// having been asked, and a read-only tool that cannot read is better than one
// that can also write.
var gitPermitted = map[string]bool{
	"status":      true,
	"log":         true,
	"diff":        true,
	"show":        true,
	"ls-files":    true,
	"rev-parse":   true,
	"branch":      true,
	"blame":       true,
	"describe":    true,
	"shortlog":    true,
	"whatchanged": true,
	"tag":         true,
	"stash":       true,
	"remote":      true,
	"reflog":      true,
	"cat-file":    true,
	"grep":        true,
}

// gitSecond is the argument a subcommand is entered through where the same
// word also writes.
//
// `stash list` reads and `stash pop` moves work, `remote -v` reads and
// `remote add` changes a repository, and to a model the two are the same word,
// so what follows it is what decides. Nothing is permitted after it, since
// neither of those forms takes another argument.
var gitSecond = map[string][]string{
	"stash":  {"list", "show"},
	"remote": {"-v", "show", "get-url"},
}

// gitPathspec is the set of subcommands whose trailing arguments are paths.
var gitPathspec = map[string]bool{
	"status":      true,
	"log":         true,
	"diff":        true,
	"show":        true,
	"ls-files":    true,
	"blame":       true,
	"describe":    true,
	"shortlog":    true,
	"whatchanged": true,
	"grep":        true,
}

// gitNameOnly is the set of subcommands that create what a bare word names,
// which is why a bare word is refused after them.
//
// `git branch new` creates a branch, `git tag v1` creates a tag and
// `git reflog expire` discards entries. The forms of these that only read pass
// everything as an option, so refusing a bare word costs a model one retry
// rather than a repository that changed while it was looking.
var gitNameOnly = map[string]bool{
	"branch": true,
	"tag":    true,
	"reflog": true,
}

// gitRefused is the set of options that write a file, run a program, or
// redirect which repository or directory git works on.
//
// The subcommand allowlist does not cover these, because each is spelled the
// same way whichever read-only command carries it: `git log --output=notes`
// writes a file into the tree, and `git show --textconv` runs a filter named in
// a file inside the repository. They are refused by name before anything is
// run, since the argument is all there is to go on.
var gitRefused = map[string]string{
	"--output":       "it writes the output to a file",
	"-o":             "it writes the output to a file",
	"--textconv":     "it runs a filter named in the attributes file",
	"--ext-diff":     "it runs an external diff driver named in the configuration",
	"--upload-pack":  "it names a program for git to run",
	"--receive-pack": "it names a program for git to run",
	"--git-dir":      "it redirects which repository git reads and writes",
	"--work-tree":    "it redirects which directory git writes",
	"-C":             "it redirects which directory git runs in",
	"--namespace":    "it redirects which repository git reads",
	"--super-prefix": "it redirects the paths git resolves",
	"--exec-path":    "it redirects where git looks for programs",
}

// gitOptionValue is the set of options taking their value as a separate
// argument, so that a value is not mistaken for a path when the separator is
// placed. An option written as one argument is not in it: the value is part of
// the same argument and the split is made on the name.
//
// A name left out of the set costs a confusing message from git rather than a
// flag, since a value that is treated as a path puts the separator in the wrong
// place and git then reports a path it does not have.
var gitOptionValue = map[string]bool{
	"-n":                true,
	"--max-count":       true,
	"--skip":            true,
	"--since":           true,
	"--until":           true,
	"--after":           true,
	"--before":          true,
	"--author":          true,
	"--committer":       true,
	"--grep":            true,
	"-L":                true,
	"--format":          true,
	"--pretty":          true,
	"--pretty-format":   true,
	"--encoding":        true,
	"-S":                true,
	"-G":                true,
	"--pickaxe-regex":   true,
	"-U":                true,
	"--unified":         true,
	"--decorate-refs":   true,
	"--date":            true,
	"--abbrev":          true,
	"--glob":            true,
	"--exclude":         true,
	"--max-age":         true,
	"--min-parents":     true,
	"--skip-to":         true,
	"--since-as-filter": true,
}

// gitTools is what the git tool carries.
//
// It is resolved once, when the set is built, so that a call does not have to
// find the repository again and so that a session either offers the tool or
// reports that there is nothing to offer.
type gitTools struct {
	repo    string
	timeout time.Duration
}

// NewGit returns the read-only git tools for the repository holding dir.
//
// A directory in no repository is reported rather than offered: every command
// would fail the same way, and a tool that cannot run is one a model keeps
// calling.
func NewGit(dir string) (*Set, error) {
	return newGit(dir, GitTimeout)
}

// newGit takes the timeout rather than reading the constant, so that a test can
// give a command a deadline short enough to sit and wait for.
func newGit(dir string, timeout time.Duration) (*Set, error) {
	top, err := gitTop(dir, timeout)
	if err != nil {
		return nil, err
	}
	g := &gitTools{repo: top, timeout: timeout}
	s := New()
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name: gitTool,
			Description: "Run a read-only git command in the repository, such as " +
				"status, log, diff or show, and return what it printed. A " +
				"subcommand that would change the repository is refused.",
			Parameters: json.RawMessage(gitParameters),
		},
	}, g.call)
	return s, nil
}

// call runs one git request from the arguments a model sent.
func (g *gitTools) call(raw json.RawMessage) (string, error) {
	var args struct {
		Args *[]string `json:"args"`
		Path *string   `json:"path"`
	}
	if err := decode(raw, &args); err != nil {
		return "", err
	}
	if args.Args == nil {
		return "", missingArg("args")
	}
	argv, err := gitArgv(*args.Args)
	if err != nil {
		return "", err
	}
	dir, err := g.workDir(args.Path)
	if err != nil {
		return "", err
	}
	return g.run(dir, argv)
}

// gitArgv turns the arguments a model gave into the arguments git is run with.
//
// The subcommand is the first argument and nothing may precede it, so a global
// option such as -C or --git-dir is refused by being neither: a tool that
// pointed git at another repository would not be the tool the session offered.
func gitArgv(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("no git arguments were given, and the first of them is the subcommand")
	}
	sub := args[0]
	if !gitPermitted[sub] {
		return nil, fmt.Errorf("git %s is not permitted: this tool runs the read-only "+
			"subcommands %s, and no approval has been settled for the rest",
			sub, permittedList())
	}
	rest := args[1:]

	if second, ok := gitSecond[sub]; ok {
		if len(rest) == 0 || !slices.Contains(second, rest[0]) {
			return nil, fmt.Errorf("git %s is not permitted: only git %s %s reads, and "+
				"every other form of it changes the repository",
				sub, sub, strings.Join(second, " or "))
		}
		if len(rest) > 1 {
			return nil, fmt.Errorf("git %s %s takes no argument after that",
				sub, rest[0])
		}
		return []string{sub, rest[0]}, nil
	}

	opts, paths, err := gitSplit(rest)
	if err != nil {
		return nil, err
	}
	switch {
	case gitPathspec[sub]:
		// The separator is placed before the paths, so that a file whose
		// name begins with a hyphen is read as a path rather than as an
		// option. It is git's own convention, so a model that writes it
		// itself lands in the same place.
		argv := append([]string{sub}, opts...)
		if len(paths) > 0 {
			argv = append(argv, "--")
			argv = append(argv, paths...)
		}
		return argv, nil
	case gitNameOnly[sub] && len(paths) > 0:
		return nil, fmt.Errorf("git %s is not permitted with a name: it creates one, "+
			"and only the forms that take no name are read-only", sub)
	default:
		// The rest carry a revision rather than a path, as in
		// `cat-file -p HEAD`, and a separator there is an error of its own.
		return append(append([]string{sub}, opts...), paths...), nil
	}
}

// gitSplit divides what follows a subcommand into the arguments git reads as
// options and the arguments it reads as paths, refusing the options the tool
// does not run.
//
// Everything from the first argument that is not an option onwards is a path,
// which is where git itself draws the line, and an option taking its value as a
// separate argument carries that value with it.
func gitSplit(rest []string) (opts, paths []string, err error) {
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if arg == "--" {
			// What follows the separator is a path whatever it looks like,
			// so it is not checked against the refused options: a file
			// may be named after one of them.
			return opts, rest[i+1:], nil
		}
		if !strings.HasPrefix(arg, "-") {
			return opts, rest[i:], nil
		}
		if name := gitOptionName(arg); gitRefused[name] != "" {
			return nil, nil, fmt.Errorf("git option %s is not permitted: %s",
				name, gitRefused[name])
		}
		opts = append(opts, arg)
		if gitOptionValue[arg] && i+1 < len(rest) {
			i++
			opts = append(opts, rest[i])
		}
	}
	return opts, nil, nil
}

// gitOptionName is the name of an option, without the value a `--name=value`
// form carries.
//
// The value is split off so that an option written as one argument is refused
// by the same name as one written as two, since the refusal is by name and
// would otherwise miss the form git documents.
func gitOptionName(arg string) string {
	name, _, found := strings.Cut(arg, "=")
	if !found {
		return arg
	}
	return name
}

// permittedList spells the permitted subcommands for a refusal message. They
// are sorted so that the message is the same on every run, since a model
// reading it has to recognise the name it wants in it.
func permittedList() string {
	names := make([]string, 0, len(gitPermitted))
	for name := range gitPermitted {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// escapes reports whether a path leaves the directory it is joined to.
//
// The check is on the cleaned path rather than on what a stat of it says,
// because a stat answers whether the path is there before it answers whether it
// points outward, and the two questions have to be kept apart. A cleaned path
// that begins with a parent reference, or that is absolute, points out of the
// tree by construction; a symlink is caught separately, by comparing the
// resolved path with the resolved root.
func escapes(name string) bool {
	if filepath.IsAbs(name) {
		return true
	}
	cleaned := filepath.Clean(name)
	return cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator))
}

// workDir resolves the directory a command runs in.
//
// The root refuses a path that leaves the repository, and the subprocess is
// handed a real path rather than an os.Root, since a working directory is given
// to the kernel rather than opened through it. The resolved path is therefore
// compared against the resolved repository as well, which covers the window
// between the two checks and costs one comparison.
func (g *gitTools) workDir(path *string) (string, error) {
	name := "."
	if path != nil && *path != "" {
		name = *path
	}
	if name == "." {
		return g.repo, nil
	}

	// The path is refused for leaving before anything is asked of the
	// filesystem, so that the refusal does not depend on whether the path
	// happens to exist. A stat asked first reports a path that is missing
	// before it reports a path that escapes, and the reader is then told a
	// directory is not there rather than that a directory outside the
	// repository was refused.
	if escapes(name) {
		return "", fmt.Errorf("refused: %q is outside the repository at %s", name, g.repo)
	}

	root, err := os.OpenRoot(g.repo)
	if err != nil {
		return "", fmt.Errorf("the repository at %s cannot be opened: %w", g.repo, err)
	}
	defer root.Close()

	if _, err := root.Stat(name); err != nil {
		return "", pathError("cannot work in", name, err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(g.repo, name))
	if err != nil {
		return "", pathError("cannot work in", name, err)
	}
	top, err := filepath.EvalSymlinks(g.repo)
	if err != nil {
		return "", fmt.Errorf("the repository at %s cannot be resolved: %w", g.repo, err)
	}
	rel, err := filepath.Rel(top, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refused: %q is outside the repository at %s", name, g.repo)
	}
	return resolved, nil
}

// run executes git and returns what it printed.
//
// The arguments are given to the process as an array rather than to a shell,
// since a shell reads an argument as a command and the arguments here come from
// a model. The command is killed when the deadline passes, so a command that
// blocks returns an error rather than waiting for a reader who cannot see that
// it has.
func (g *gitTools) run(dir string, argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitProgram, argv...)
	cmd.Dir = dir
	cmd.Env = gitEnv()

	stdout := capped{limit: MaxOutput}
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// A git killed at the deadline can leave a child of its own holding the
	// output pipe, and the wait would then outlast the deadline by however
	// long that child runs. The delay is what closes the pipe and returns.
	cmd.WaitDelay = gitWaitDelay

	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git %s did not finish within %s and was stopped",
			argv[0], g.timeout)
	}
	if stdout.over {
		return "", fmt.Errorf("git %s printed more than the %d byte limit and was not "+
			"returned, so it is reported rather than cut short; ask for less of it",
			argv[0], int64(MaxOutput))
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", argv[0], detail)
	}
	return stdout.buf.String(), nil
}

// gitEnv is the environment every command runs with.
//
// GIT_OPTIONAL_LOCKS=0 stops git taking the index lock to refresh the index,
// which `git status` would otherwise do, so a command that only reads does not
// write to the repository. GIT_TERMINAL_PROMPT=0 stops git asking for a
// credential on a terminal nobody is watching, which on a command that cannot
// reach a remote is a hang rather than a prompt.
func gitEnv() []string {
	return append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

// gitTop resolves the root of the repository holding dir, once.
func gitTop(dir string, timeout time.Duration) (string, error) {
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("cannot look for a repository in %s: %w", dir, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitProgram, "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git did not answer within %s, so the repository was not resolved",
			timeout)
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("no git repository: %s", detail)
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", fmt.Errorf("no git repository: %s reported no root", gitProgram)
	}
	return top, nil
}

// capped collects output up to a limit and records that it passed it.
//
// The bytes past the limit are dropped rather than trimmed into a half line,
// since a command whose output stops mid commit is read as though it were the
// whole of what was asked for. A command that keeps printing is bounded by the
// timeout rather than being stopped here, so that the cap is reported instead
// of being raced against a process still writing.
type capped struct {
	buf   bytes.Buffer
	limit int
	over  bool
}

// Write takes what fits and discards the rest, reporting every byte as written
// so that git is not told its output failed.
func (c *capped) Write(p []byte) (int, error) {
	room := c.limit - c.buf.Len()
	if room < len(p) {
		c.buf.Write(p[:room])
		c.over = true
		return len(p), nil
	}
	return c.buf.Write(p)
}
