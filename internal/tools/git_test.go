package tools

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireGit returns the path of the git program, skipping the test where there
// is none. The tests that need it are about what this package sends to git, and
// there is nothing to check on a host that cannot run it.
func requireGit(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath(gitProgram)
	if err != nil {
		t.Skipf("%s is not on the PATH: %v", gitProgram, err)
	}
	return path
}

// gitFixture returns a repository holding one commit, and the tools for it.
func gitFixture(t *testing.T) (*Set, string) {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	runGit(t, dir, "add", "file")
	runGit(t, dir, "commit", "-q", "-m", "the first commit")

	s, err := NewGit(dir)
	if err != nil {
		t.Fatalf("building the git tools over %s: %v", dir, err)
	}
	return s, dir
}

// NewGitOrSkip returns the git tools over a repository, or nil where git is not
// on the PATH, for the tests about the schemas rather than about git.
func NewGitOrSkip(t *testing.T) *Set {
	t.Helper()
	if _, err := exec.LookPath(gitProgram); err != nil {
		return nil
	}
	s, _ := gitFixture(t)
	return s
}

// runGit runs git in a directory and fails the test where it does not, so that
// a fixture which is not the repository it was meant to be is not mistaken for
// a fault in the code under test.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitProgram, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// TestGitRunsAPermittedSubcommand checks that a command the set is meant to run
// reaches git and its output comes back, since a tool that refused everything
// would pass every refusal test.
func TestGitRunsAPermittedSubcommand(t *testing.T) {
	s, dir := gitFixture(t)

	got := mustText(t, call(t, s, gitTool, `{"args":["log","--oneline"]}`))
	if !strings.Contains(got, "the first commit") {
		t.Errorf("the log was %q, which does not hold the commit made", got)
	}

	got = mustText(t, call(t, s, gitTool, `{"args":["rev-parse","--show-toplevel"]}`))
	if strings.TrimSpace(got) != resolvePath(t, dir) {
		t.Errorf("the root was reported as %q, want %q", got, resolvePath(t, dir))
	}
}

// TestGitStatusReadsTheTree checks the other common form, with the separator
// placed by the tool rather than written by the caller.
func TestGitStatusReadsTheTree(t *testing.T) {
	s, dir := gitFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "untracked"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	got := mustText(t, call(t, s, gitTool, `{"args":["status","--short"]}`))
	if !strings.Contains(got, "untracked") {
		t.Errorf("the status was %q, which does not hold the untracked file", got)
	}
}

// TestGitPermitsTheSubcommandsThisRepositoryNeeds checks the three added to
// the allowlist, since the set is what bounds what a model may reach and a
// name missing from it is a subcommand the model cannot use at all.
//
// The argument list is checked rather than the command being run. A push in a
// fixture with no remote fails on its own terms rather than on anything the
// tool did, so running it would test git rather than the allowlist.
func TestGitPermitsTheSubcommandsThisRepositoryNeeds(t *testing.T) {
	for _, args := range [][]string{
		{"commit", "-m", "a commit"},
		{"push"},
		{"push", "--force"},
		{"worktree", "list"},
		{"worktree", "add", "sub", "HEAD"},
		{"worktree", "remove", "sub"},
		{"worktree", "prune"},
	} {
		argv, err := gitArgv(args)
		if err != nil {
			t.Errorf("git %s was refused: %v", strings.Join(args, " "), err)
			continue
		}
		if argv[0] != args[0] {
			t.Errorf("git %s ran as %s", strings.Join(args, " "), strings.Join(argv, " "))
		}
	}
}

// TestGitWorktreeRefusesAWildcard checks the guard on the one form of the
// worktree workflow that reaches past the checkout it was named with, since
// `worktree remove *` is a pattern git matches every checkout against and a
// worktree that is removed is not put back by the next command.
//
// The refusal is checked through the tool rather than against gitArgv, since
// the guard lives where the repository root is known. A refusal after the fact
// would still have removed them.
func TestGitWorktreeRefusesAWildcard(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{
		`{"args":["worktree","remove","*"]}`,
		`{"args":["worktree","remove","--force","*"]}`,
		`{"args":["worktree","add","build/*"]}`,
		`{"args":["worktree","remove","sub/?"]}`,
		`{"args":["worktree","remove","[ab]"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "wildcard") {
			t.Errorf("%s was refused without naming the wildcard: %v", args, err)
		}
	}

	// The fixture holds one checkout, the repository itself. Rows are counted
	// rather than fields, since git prints the listing as a padded table and
	// counting the whitespace separated pieces would count the columns.
	worktrees := mustText(t, call(t, s, gitTool, `{"args":["worktree","list"]}`))
	if rows := strings.Count(strings.TrimRight(worktrees, "\n"), "\n") + 1; rows > 1 {
		t.Errorf("the fixture holds %d checkouts, want the one it was made with: %q",
			rows, worktrees)
	}
}

// TestGitWorktreeRefusesAPathLeavingTheRepository checks the containment, since
// the whole purpose of the subcommand is to name a directory and that is the
// one place it could name one outside the tree the tools are held to.
func TestGitWorktreeRefusesAPathLeavingTheRepository(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{
		`{"args":["worktree","remove","../elsewhere"]}`,
		`{"args":["worktree","add","/tmp/openrouter-should-not-exist"]}`,
		`{"args":["worktree","move","sub","../elsewhere"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "outside") {
			t.Errorf("%s was refused without naming the containment: %v", args, err)
		}
	}
	if _, err := os.Stat("/tmp/openrouter-should-not-exist"); err == nil {
		t.Error("a worktree was created outside the repository")
	}
}

// TestGitWorktreeRefusesForce checks the one option that discards rather than
// refuses, since a worktree holding changes a reader was working in is not
// restored by removing it.
func TestGitWorktreeRefusesForce(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{
		`{"args":["worktree","remove","--force","sub"]}`,
		`{"args":["worktree","remove","-f","sub"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "discards local changes") {
			t.Errorf("%s was refused without giving the reason: %v", args, err)
		}
	}
}

// TestGitRefusesASubcommandThatWrites checks the allowlist against the
// subcommands that would change the repository and are not on it, and that
// nothing was run: a refusal after the fact would still have made the change.
//
// commit, push and worktree are absent from the list because they are
// permitted. Every other writing subcommand is here, so the set is bounded by
// more than the three names that were added to it.
func TestGitRefusesASubcommandThatWrites(t *testing.T) {
	s, _ := gitFixture(t)
	before := mustText(t, call(t, s, gitTool, `{"args":["rev-parse","HEAD"]}`))

	for _, c := range []struct{ args, sub string }{
		{`{"args":["add","file"]}`, "add"},
		{`{"args":["fetch"]}`, "fetch"},
		{`{"args":["reset","--hard","HEAD"]}`, "reset"},
		{`{"args":["checkout","-b","another"]}`, "checkout"},
		{`{"args":["apply","/dev/null"]}`, "apply"},
		{`{"args":["gc"]}`, "gc"},
		{`{"args":["merge","feature"]}`, "merge"},
	} {
		err := mustFail(t, call(t, s, gitTool, c.args))
		if !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("git %s was refused without saying so: %v", c.sub, err)
		}
		if !strings.Contains(err.Error(), c.sub) {
			t.Errorf("the refusal for git %s did not name the subcommand: %v", c.sub, err)
		}
	}

	after := mustText(t, call(t, s, gitTool, `{"args":["rev-parse","HEAD"]}`))
	if before != after {
		t.Errorf("the repository moved from %q to %q", before, after)
	}
	branches := mustText(t, call(t, s, gitTool, `{"args":["branch","--list"]}`))
	if strings.Contains(branches, "another") {
		t.Errorf("a branch was created: %q", branches)
	}
}

// TestGitRefusesAnOptionBeforeTheSubcommand checks that a global option is
// refused by not being a subcommand, since one that redirected git at another
// repository would not be the tool the session offered.
func TestGitRefusesAnOptionBeforeTheSubcommand(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{
		`{"args":["-C","/","status"]}`,
		`{"args":["--git-dir=/somewhere/else/.git","status"]}`,
		`{"args":["-c","core.pager=cat","log"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("%s was refused without saying so: %v", args, err)
		}
	}
}

// TestGitRefusesAnOptionThatWritesOrRuns checks the option table against the
// forms that write a file or run a program, each of which is spelled the same
// way whichever read-only command carries it.
func TestGitRefusesAnOptionThatWritesOrRuns(t *testing.T) {
	s, dir := gitFixture(t)

	for _, args := range []string{
		`{"args":["log","--output=` + filepath.Join(dir, "written") + `"]}`,
		`{"args":["log","-o",` + quote(filepath.Join(dir, "written")) + `]}`,
		`{"args":["show","--textconv","HEAD"]}`,
		`{"args":["log","--ext-diff"]}`,
		`{"args":["log","--upload-pack=touch /tmp/openrouter-tools-should-not-exist"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("%s was refused without saying so: %v", args, err)
		}
	}
	if _, err := os.Stat("/tmp/openrouter-tools-should-not-exist"); err == nil {
		t.Error("a program named by an argument was run")
	}
	if _, err := os.Stat(filepath.Join(dir, "written")); err == nil {
		t.Error("a file named by an option was written")
	}
}

// TestGitRefusesASubcommandWithAName checks the subcommands that create what a
// bare word names, since the listing form is read and the naming form is not.
func TestGitRefusesASubcommandWithAName(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{
		`{"args":["branch","a-branch"]}`,
		`{"args":["tag","v1.0.0"]}`,
		`{"args":["reflog","expire","--all"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("%s was refused without saying so: %v", args, err)
		}
	}
	branches := mustText(t, call(t, s, gitTool, `{"args":["branch","--list"]}`))
	if strings.Contains(branches, "a-branch") {
		t.Errorf("a branch was created: %q", branches)
	}
	tags := mustText(t, call(t, s, gitTool, `{"args":["tag","--list"]}`))
	if strings.TrimSpace(tags) != "" {
		t.Errorf("a tag was created: %q", tags)
	}
}

// TestGitStashAndRemoteNeedTheReadingForm checks the two subcommands that are
// the same word whether they read or write, since a model chooses the word
// rather than the form.
func TestGitStashAndRemoteNeedTheReadingForm(t *testing.T) {
	s, _ := gitFixture(t)

	mustText(t, call(t, s, gitTool, `{"args":["stash","list"]}`))
	mustText(t, call(t, s, gitTool, `{"args":["remote","-v"]}`))

	for _, args := range []string{
		`{"args":["stash"]}`,
		`{"args":["stash","pop"]}`,
		`{"args":["stash","clear"]}`,
		`{"args":["remote"]}`,
		`{"args":["remote","add","origin","/somewhere"]}`,
		`{"args":["remote","set-url","origin","/somewhere"]}`,
	} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("%s was refused without saying so: %v", args, err)
		}
	}
}

// TestGitDoesNotReadAFlagShapedPathAsAFlag checks the separator, since a file
// named after an option is a file and not an option, and a model that made one
// would otherwise be running a program of its own naming.
func TestGitDoesNotReadAFlagShapedPathAsAFlag(t *testing.T) {
	s, dir := gitFixture(t)

	// The name holds no separator, since a name holding one is a path rather
	// than a file the tool would be asked to read as a name.
	name := "--upload-pack=touch_here"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	// The caller writes the separator, and the tool places its own where the
	// caller left it, so the path is a path.
	got := mustText(t, call(t, s, gitTool,
		`{"args":["ls-files","--others","--",`+quote(name)+`]}`))
	if strings.TrimSpace(got) != name {
		t.Errorf("ls-files returned %q, want %q", strings.TrimSpace(got), name)
	}
	// The same name without the separator is offered to git as an option,
	// which it refuses rather than runs, and the tool refuses it by name
	// before git is reached.
	err := mustFail(t, call(t, s, gitTool, `{"args":["ls-files","--others",`+quote(name)+`]}`))
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("a flag shaped path was not refused by name: %v", err)
	}
}

// TestGitPlacesTheSeparatorBeforeTheFirstPath checks the placement the tool
// makes on its own, including after an option carrying its value separately,
// since a value mistaken for a path puts the separator in the wrong place.
func TestGitPlacesTheSeparatorBeforeTheFirstPath(t *testing.T) {
	for _, args := range []struct {
		give string
		want []string
	}{
		{`["log","--oneline","file"]`, []string{"log", "--oneline", "--", "file"}},
		{`["log","-n","2","file"]`, []string{"log", "-n", "2", "--", "file"}},
		{`["log","--max-count=2","file"]`, []string{"log", "--max-count=2", "--", "file"}},
		{`["log","--","file"]`, []string{"log", "--", "file"}},
		{`["status","--short","--","--upload-pack=x"]`, []string{"status", "--short", "--", "--upload-pack=x"}},
		{`["log"]`, []string{"log"}},
		{`["log","--oneline"]`, []string{"log", "--oneline"}},
		{`["cat-file","-p","HEAD"]`, []string{"cat-file", "-p", "HEAD"}},
		{`["stash","list"]`, []string{"stash", "list"}},
		{`["remote","-v"]`, []string{"remote", "-v"}},
		{`["status","--short","sub"]`, []string{"status", "--short", "--", "sub"}},
	} {
		argv, err := gitArgv(parseArgs(t, `{"args":`+args.give+`}`))
		if err != nil {
			t.Errorf("gitArgv(%s) failed: %v", args.give, err)
			continue
		}
		if strings.Join(argv, " ") != strings.Join(args.want, " ") {
			t.Errorf("gitArgv(%s) is %v, want %v", args.give, argv, args.want)
		}
	}
}

// TestGitRefusesTheSubcommandItCannotRun checks the cases where nothing is
// run at all, since a model reading the message has to be able to tell a
// refusal from a command git would not accept.
func TestGitRefusesTheSubcommandItCannotRun(t *testing.T) {
	for _, args := range []string{
		`[]`,
		`["nosuchsubcommand"]`,
		`["branch","--list","extra"]`,
		`["log","--output=notes"]`,
		`["log","-o","notes"]`,
		`["stash","pop"]`,
		`["remote","add","origin","/somewhere"]`,
		`["stash","list","--all"]`,
	} {
		argv, err := gitArgv(parseArgs(t, `{"args":`+args+`}`))
		if err == nil {
			t.Errorf("gitArgv(%s) returned %v, want a refusal", args, argv)
		}
	}
}

// TestGitWithoutARepositoryIsRefused checks that a directory in no repository
// is reported rather than offered, since every command would fail the same way
// and a model would keep calling.
func TestGitWithoutARepositoryIsRefused(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	_, err := NewGit(dir)
	if err == nil {
		t.Fatal("the tools were offered for a directory in no repository")
	}
	if !errors.Is(err, ErrNotRepository) {
		t.Errorf("the refusal was not ErrNotRepository: %v", err)
	}
	if strings.Contains(err.Error(), "fatal") || strings.Contains(err.Error(), "(null)") {
		t.Errorf("the refusal carried the raw text git printed: %v", err)
	}
}

// TestGitOnAMissingDirectoryIsRefused checks the other shape of the open, since
// a path that is not there is not a directory in no repository.
func TestGitOnAMissingDirectoryIsRefused(t *testing.T) {
	_, err := NewGit(filepath.Join(t.TempDir(), "no-such-directory"))
	if err == nil {
		t.Fatal("the tools were offered for a directory that is not there")
	}
}

// TestGitNeedsItsArgs checks the required argument by name, and that the
// optional one is not required.
func TestGitNeedsItsArgs(t *testing.T) {
	s, _ := gitFixture(t)

	for _, args := range []string{`{}`, `{"path":"."}`} {
		err := mustFail(t, call(t, s, gitTool, args))
		if !strings.Contains(err.Error(), `"args"`) {
			t.Errorf("the refusal for %s did not name the missing argument: %v", args, err)
		}
	}
}

// TestGitOnANonZeroExitCarriesStderr checks that a command git refused is
// reported with what git said, since an empty failure tells a model nothing
// about which of its arguments was wrong.
func TestGitOnANonZeroExitCarriesStderr(t *testing.T) {
	s, _ := gitFixture(t)

	err := mustFail(t, call(t, s, gitTool,
		`{"args":["rev-parse","--verify","refs/heads/no-such-branch"]}`))
	if !strings.Contains(err.Error(), "git rev-parse failed") {
		t.Errorf("the refusal did not name the command: %v", err)
	}
	if !strings.Contains(err.Error(), "Needed a single revision") {
		t.Errorf("the refusal did not carry what git said: %v", err)
	}
}

// TestGitRunsInTheDirectoryItIsGiven checks the path argument, since a
// repository is made of directories and a model asking about one of them
// should not have to name it inside the arguments.
func TestGitRunsInTheDirectoryItIsGiven(t *testing.T) {
	s, dir := gitFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	got := mustText(t, call(t, s, gitTool, `{"args":["rev-parse","--show-prefix"],"path":"sub"}`))
	if strings.TrimSpace(got) != "sub/" {
		t.Errorf("the prefix was %q, want %q", strings.TrimSpace(got), "sub/")
	}
}

// TestGitRefusesAPathLeavingTheRepository checks the four shapes the filesystem
// tools refuse, since the git tool is contained the same way.
func TestGitRefusesAPathLeavingTheRepository(t *testing.T) {
	s, dir := gitFixture(t)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, path := range []string{"..", "sub/../..", "/tmp", link} {
		err := mustFail(t, call(t, s, gitTool,
			`{"args":["status"],"path":`+quote(path)+`}`))
		if !strings.Contains(err.Error(), "outside") {
			t.Errorf("running in %q was refused without naming the refusal: %v", path, err)
		}
	}
}

// TestGitRefusesOutputOverTheLimit checks the cap, since a log on a large
// repository is larger than a conversation and the figure is what tells a model
// to ask for less of it.
func TestGitRefusesOutputOverTheLimit(t *testing.T) {
	s, dir := gitFixture(t)

	big := filepath.Join(dir, "big")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	chunk := make([]byte, 64*1024)
	for written := 0; written <= MaxOutput; written += len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}
	f.Close()
	runGit(t, dir, "add", "big")
	runGit(t, dir, "commit", "-q", "-m", "a large file")

	err = mustFail(t, call(t, s, gitTool, `{"args":["cat-file","-p","HEAD:big"]}`))
	if !strings.Contains(err.Error(), "1048576") {
		t.Errorf("the refusal did not state the limit: %v", err)
	}
	if !strings.Contains(err.Error(), "printed more than") {
		t.Errorf("the refusal did not say the output was too large: %v", err)
	}
}

// TestGitStopsACommandThatOverrunsTheDeadline checks the timeout, since the
// process is outside this one and nothing else stops a command that blocks.
func TestGitStopsACommandThatOverrunsTheDeadline(t *testing.T) {
	real := requireGit(t)

	fake := fakeGit(t, real)
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")

	s, err := newGit(dir, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("building the git tools over %s: %v", dir, err)
	}
	start := time.Now()
	err = mustFail(t, call(t, s, gitTool, `{"args":["log","--oneline"]}`))
	if !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("the refusal did not say the command was stopped: %v", err)
	}
	if waited := time.Since(start); waited > 30*time.Second {
		t.Errorf("the command was waited on for %s, which is the hang the timeout exists to avoid", waited)
	}
}

// fakeGit returns a directory holding a program named git that stands still on
// the log subcommand and hands everything else to the real one.
func fakeGit(t *testing.T, real string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = log ]; then\n" +
		"	sleep 30\n" +
		"	exit 0\n" +
		"fi\n" +
		"exec '" + strings.ReplaceAll(real, "'", `'\''`) + "' \"$@\"\n"
	path := filepath.Join(dir, gitProgram)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return dir
}

// parseArgs reads the arguments of a call out of the JSON a test wrote them in,
// so that the cases about the argument list are written as one line each.
func parseArgs(t *testing.T, args string) []string {
	t.Helper()
	var call struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(args), &call); err != nil {
		t.Fatalf("the arguments %s are not JSON: %v", args, err)
	}
	return call.Args
}

// resolvePath returns the path of a fixture directory as git spells it, since
// the temporary directory is reached through a symlink on some hosts and git
// reports the path it resolved.
func resolvePath(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolving %s: %v", dir, err)
	}
	return resolved
}
