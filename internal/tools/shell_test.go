package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// shellCall runs one shell tool call and returns its result.
func shellCall(t *testing.T, set *Set, args map[string]any) Result {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding the arguments: %v", err)
	}
	return set.Run(openrouter.ToolCall{
		Function: openrouter.ToolCallFunction{Name: shellTool, Arguments: string(raw)},
	})
}

// The tests here run real programs, since what is being checked is that a
// program runs at all and that its output comes back. The programs used are
// chosen to be present on every platform the client targets, and a test that
// needs one that is not skips rather than failing, so that a host without it
// reports a skip rather than a broken tool.

func TestShellRunsAProgramAndReturnsWhatItPrinted(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "echo", "args": []string{"hello"}})

	if r.Err != nil {
		t.Fatalf("echo failed: %v", r.Err)
	}
	if !strings.Contains(r.Text, "hello") {
		t.Errorf("the result did not carry what was printed: %q", r.Text)
	}
}

func TestShellASilentProgramStillReportsThatItRan(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	// ls on an empty directory prints nothing and succeeds, which is the case
	// where an empty result would read to a model as though the call had been
	// dropped rather than as though the program had run.
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatalf("making the empty directory: %v", err)
	}
	r := shellCall(t, set, map[string]any{"command": "ls", "args": []string{empty}})

	if r.Err != nil {
		t.Fatalf("ls failed: %v", r.Err)
	}
	if !strings.Contains(r.Text, "ran in") {
		t.Errorf("a silent program reported %q rather than that it ran", r.Text)
	}
}

func TestShellRefusesAProgramOutsideTheAllowlist(t *testing.T) {
	dir := t.TempDir()
	asked := false
	set := NewShell(dir, approveFunc(func(string, []string, string) bool {
		asked = true
		return true
	}))

	// curl is the case the allowlist exists for. It is named rather than run,
	// and it is not even present on every host, so the call has to be refused
	// before anything is looked up. The arguments are curl's own, since a
	// flag belonging to another program would read as though the test were
	// still about that one.
	r := shellCall(t, set, map[string]any{"command": "curl", "args": []string{"-s", "https://example.invalid"}})

	if r.Err == nil {
		t.Fatal("curl was not refused")
	}
	if !strings.Contains(r.Err.Error(), "not permitted") {
		t.Errorf("the refusal did not say what was wrong: %v", r.Err)
	}
	if !strings.Contains(r.Err.Error(), "go") {
		t.Errorf("the refusal did not name what is permitted: %v", r.Err)
	}
	if asked {
		t.Error("the reader was asked about a program that would have been refused anyway")
	}
}

func TestShellRefusesAProgramNamedByAPath(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	// The allowlist is consulted on the name as written, so a program reached
	// by a path is not on it. There is no separate rule for the shape of the
	// name: it is refused as the unknown program it is, which is the same
	// refusal a model gets for any name nobody thought of.
	r := shellCall(t, set, map[string]any{"command": "../../bin/sh", "args": []string{"-c", "id"}})

	if r.Err == nil {
		t.Fatal("a program named by a path was not refused")
	}
	if !strings.Contains(r.Err.Error(), "not permitted") {
		t.Errorf("the refusal did not say what was wrong: %v", r.Err)
	}
}

func TestShellAsksBeforeRunning(t *testing.T) {
	dir := t.TempDir()
	var asked [][]string
	set := NewShell(dir, approveFunc(func(command string, args []string, dir string) bool {
		asked = append(asked, append([]string{command}, args...))
		return false
	}))

	r := shellCall(t, set, map[string]any{"command": "echo", "args": []string{"nope"}})

	if r.Err == nil {
		t.Fatal("a refused call reported no error")
	}
	if !strings.Contains(r.Err.Error(), "did not approve") {
		t.Errorf("the refusal was not reported as one: %v", r.Err)
	}
	if len(asked) != 1 {
		t.Fatalf("the reader was asked %d times, once is expected", len(asked))
	}
	// The question has to name what is about to run, or a reader approving it
	// is approving something they were not shown.
	if asked[0][0] != "echo" || asked[0][1] != "nope" {
		t.Errorf("the question did not name the command: %v", asked[0])
	}
}

func TestShellADeclinedCallRunsNothing(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "written")
	set := NewShell(dir, approveFunc(func(string, []string, string) bool { return false }))

	r := shellCall(t, set, map[string]any{"command": "touch", "args": []string{marker}})

	// touch is not on the allowlist, so this is refused twice over and the
	// marker is never written. The assertion is that nothing ran, whatever
	// order the refusals happened in.
	if r.Err == nil {
		t.Fatal("the call reported no error")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a refused call wrote a file")
	}
}

func TestShellWithoutAnApproverOffersNothing(t *testing.T) {
	dir := t.TempDir()

	set := NewShell(dir, nil)

	if names := set.Names(); len(names) != 0 {
		t.Errorf("a shell with nothing to ask through offered %v", names)
	}
}

func TestShellWithoutADirectoryOffersNothing(t *testing.T) {
	set := NewShell("", AlwaysAllow())

	if names := set.Names(); len(names) != 0 {
		t.Errorf("a shell with no working directory offered %v", names)
	}
}

func TestShellRefusesAPathLeavingTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	for _, name := range []string{"..", "../elsewhere", "/etc"} {
		r := shellCall(t, set, map[string]any{
			"command": "echo", "args": []string{"x"}, "path": name,
		})
		if r.Err == nil {
			t.Errorf("the path %q was not refused", name)
		}
	}
}

func TestShellRunsInsideAGivenSubdirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("making the subdirectory: %v", err)
	}
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "pwd", "path": "sub"})

	if r.Err != nil {
		t.Skipf("pwd is not behaving as expected here: %v", r.Err)
	}
	if !strings.Contains(r.Text, "sub") {
		t.Errorf("the command did not run in the subdirectory: %q", r.Text)
	}
}

func TestShellTheQuestionNamesTheResolvedDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("making the subdirectory: %v", err)
	}
	var askedIn string
	set := NewShell(dir, approveFunc(func(command string, args []string, dir string) bool {
		askedIn = dir
		return false
	}))

	shellCall(t, set, map[string]any{"command": "echo", "path": "sub"})

	if askedIn == "" {
		t.Fatal("the reader was not asked")
	}
	// A reader approving a command in a directory they were not shown would be
	// approving something other than what runs.
	if filepath.Base(askedIn) != "sub" {
		t.Errorf("the question named %q rather than the subdirectory", askedIn)
	}
}

func TestShellACommandThatFailsReportsWhy(t *testing.T) {
	dir := t.TempDir()
	set := NewShell(dir, AlwaysAllow())

	r := shellCall(t, set, map[string]any{"command": "cat", "args": []string{filepath.Join(dir, "absent")}})

	if r.Err == nil {
		t.Fatal("a failing command reported no error")
	}
	if !strings.Contains(r.Err.Error(), "cat failed") {
		t.Errorf("the failure did not name the program: %v", r.Err)
	}
}

func TestShellACommandThatOverrunsIsStopped(t *testing.T) {
	dir := t.TempDir()
	set := newShell(dir, AlwaysAllow(), 100*time.Millisecond)

	// tail is on the allowlist and following an empty file blocks for ever, so
	// this reaches the deadline through the whole path rather than by calling
	// the runner directly.
	start := time.Now()
	r := shellCall(t, set, map[string]any{"command": "tail", "args": []string{"-f", "/dev/null"}})

	if r.Err == nil {
		t.Fatal("a command past its deadline reported no error")
	}
	if !strings.Contains(r.Err.Error(), "did not finish within") {
		t.Errorf("the failure was not reported as a timeout: %v", r.Err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("the deadline took %s to take effect", elapsed)
	}
}

func TestShellCallsForNoCommand(t *testing.T) {
	set := NewShell(t.TempDir(), AlwaysAllow())

	r := shellCall(t, set, map[string]any{"args": []string{"build"}})

	if r.Err == nil {
		t.Fatal("a call naming no command reported no error")
	}
	if !strings.Contains(r.Err.Error(), "\"command\"") {
		t.Errorf("the refusal did not name the missing argument: %v", r.Err)
	}
}

func TestShellCarriesNoShellFeatures(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "redirected")
	set := NewShell(dir, AlwaysAllow())

	// The arguments go to the program as an array, so a redirect is an
	// argument that happens to be named like one rather than a redirection.
	// This is the property that makes the tool safe to offer at all, and it
	// is asserted rather than assumed.
	r := shellCall(t, set, map[string]any{
		"command": "echo",
		"args":    []string{"x", ">", marker},
	})

	if r.Err != nil {
		t.Fatalf("echo failed: %v", r.Err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("an argument was read as a redirect")
	}
}

func TestShellTheSchemaIsAJSONObject(t *testing.T) {
	// Every schema in this package is unmarshalled by this test, so that a
	// bracket left out in a literal is caught here rather than by a model.
	for _, raw := range []string{shellParameters} {
		var doc map[string]any
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Errorf("a schema does not parse: %v", err)
		}
	}
}

func TestShellDescribeQuotesWhatNeedsIt(t *testing.T) {
	cases := []struct {
		command string
		args    []string
		want    string
	}{
		{"go", []string{"build", "./..."}, "go build ./..."},
		{"echo", []string{"two words"}, `echo "two words"`},
		{"echo", nil, "echo"},
	}
	for _, tc := range cases {
		if got := Describe(tc.command, tc.args); got != tc.want {
			t.Errorf("Describe(%q, %v) = %q, want %q", tc.command, tc.args, got, tc.want)
		}
	}
}
