package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// permissionSession builds a session with a rules file in a temporary home, so
// that a test never writes to the reader's own.
func permissionSession(t *testing.T) (*Session, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	conv := NewConversation()
	s := &Session{
		conv:      conv,
		mainConv:  conv,
		approvals: newApprovalState(),
		tools:     &toolSet{dir: t.TempDir()},
	}
	return s, home
}

func TestAPermissionIsWrittenAndReadBack(t *testing.T) {
	s, _ := permissionSession(t)
	dir := t.TempDir()

	s.cmdPermission([]string{"add", dir, "go", "make"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules back: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("the rules read back as %+v, want one", rules)
	}
	if !rules[0].Grants("go") || !rules[0].Grants("make") {
		t.Errorf("the rule does not grant what it was given: %+v", rules[0])
	}
	if rules[0].Grants("rm") {
		t.Error("the rule granted something it was not given")
	}
}

// A rule written for a directory covers the directories beneath it, so a
// session running inside a project is covered by a rule written for it.
func TestAPermissionCoversTheDirectoriesBeneathIt(t *testing.T) {
	s, _ := permissionSession(t)
	parent := t.TempDir()
	child := filepath.Join(parent, "repo", "internal")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("making the tree: %v", err)
	}

	s.cmdPermission([]string{"add", parent, "go"})

	if permitted := config.PermittedCommands(s.rulesFor(child), child); len(permitted) != 1 {
		t.Errorf("a child directory is not covered: %v", permitted)
	}
}

func TestAPermissionTakesEffectWithoutARestart(t *testing.T) {
	s, _ := permissionSession(t)
	dir := s.tools.dir

	if s.Approve("go", []string{"build", "./..."}, dir) {
		t.Error("a program ran with no rule and no question")
	}

	s.cmdPermission([]string{"add", dir, "go"})

	if !s.Approve("go", []string{"build", "./..."}, dir) {
		t.Error("the rule just granted did not settle the call")
	}
}

// A second rule for the same directory replaces the first, since a rule is
// about a place and two rules for one place would have no way to say which
// applies.
func TestAddingAgainReplacesTheRuleForThatDirectory(t *testing.T) {
	s, _ := permissionSession(t)
	dir := t.TempDir()

	s.cmdPermission([]string{"add", dir, "go", "make"})
	s.cmdPermission([]string{"add", dir, "gofmt"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("a directory carries %d rules, want one", len(rules))
	}
	if rules[0].Grants("go") {
		t.Error("the first rule survived being replaced")
	}
	if !rules[0].Grants("gofmt") {
		t.Error("the second rule did not take")
	}
}

func TestRemovingAPermissionTakesItAway(t *testing.T) {
	s, _ := permissionSession(t)
	dir := t.TempDir()

	s.cmdPermission([]string{"add", dir, "go"})
	s.cmdPermission([]string{"remove", dir})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("the rule survived being removed: %+v", rules)
	}
	if s.Approve("go", nil, dir) {
		t.Error("a removed permission still settles a call")
	}
}

func TestRemovingWhatIsNotThereSaysSo(t *testing.T) {
	s, _ := permissionSession(t)

	s.cmdPermission([]string{"remove", t.TempDir()})

	// The message is in the reply, which the test does not read, so what is
	// checked is that nothing was written for a rule that was not there.
	if rules, _ := config.LoadRules(nil); len(rules) != 0 {
		t.Errorf("removing nothing wrote %+v", rules)
	}
}

// The file is written at 0600, since it names the directories a model may run
// programs in.
func TestTheRulesFileIsNotWorldReadable(t *testing.T) {
	s, _ := permissionSession(t)

	s.cmdPermission([]string{"add", t.TempDir(), "go"})

	path, err := config.RuleFile()
	if err != nil {
		t.Fatalf("locating the file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the file is not there: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file is %04o, want 0600", mode)
	}
}

func TestTheRulesFileHoldsReadableJSON(t *testing.T) {
	s, _ := permissionSession(t)

	s.cmdPermission([]string{"add", t.TempDir(), "go", "make"})

	path, _ := config.RuleFile()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	var written struct {
		Tools []config.ApprovalRule `json:"OPENROUTER_TOOLS"`
	}
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("the file is not readable JSON: %v", err)
	}
	if len(written.Tools) != 1 {
		t.Errorf("the file holds %d rules, want one", len(written.Tools))
	}
}

// A rule written by hand into the configuration file is still read, since the
// two are read as one set.
func TestARuleInTheConfigurationIsStillRead(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Tools: []config.ApprovalRule{
		{Path: dir, Commands: []string{"go"}},
	}}

	rules, err := config.LoadRules(cfg)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("the configuration rule was not read: %+v", rules)
	}
}

func TestTheListingNamesEveryRule(t *testing.T) {
	s, _ := permissionSession(t)
	elsewhere := t.TempDir()

	s.cmdPermission([]string{"add", elsewhere, "go"})

	lines := s.permissionListing(s.rulesFor(s.tools.dir))
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, elsewhere) {
		t.Errorf("the listing does not name the rule:\n%s", joined)
	}
	// A rule elsewhere is listed but does not settle anything here, and the
	// listing says which is which.
	if strings.Contains(joined, "permitted here") {
		t.Errorf("a rule for another directory is reported as permitted here:\n%s", joined)
	}
}

func TestTheListingSaysWhatItPermitsHere(t *testing.T) {
	s, _ := permissionSession(t)

	s.cmdPermission([]string{"add", s.tools.dir, "go"})

	joined := strings.Join(s.permissionListing(s.rulesFor(s.tools.dir)), "\n")
	if !strings.Contains(joined, "permitted here: go") {
		t.Errorf("the listing does not say what it permits here:\n%s", joined)
	}
}

// A directory is taken only when one is named. The first argument is usually a
// program, and treating it as a directory wrote a rule for a path that was
// never there and granted nothing at all.
func TestAddWithoutADirectoryGrantsThePrograms(t *testing.T) {
	s, _ := permissionSession(t)
	here := s.tools.dir

	s.cmdPermission([]string{"add", "go", "make"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("the rules read back as %+v, want one", rules)
	}
	// The rule has to cover the directory the session is in, or it settles
	// nothing for the reader who just wrote it.
	if permitted := config.PermittedCommands(rules, here); len(permitted) != 2 {
		t.Errorf("the rule does not cover this directory: %v", permitted)
	}
	for _, want := range []string{"go", "make"} {
		if !rules[0].Grants(want) {
			t.Errorf("the rule does not grant %q: %+v", want, rules[0])
		}
	}
}

// One program with no directory is the common form, and it must not be read as
// a directory called after the program.
func TestOneProgramGrantsThatProgram(t *testing.T) {
	s, _ := permissionSession(t)

	s.cmdPermission([]string{"add", "go"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("the rules read back as %+v, want one", rules)
	}
	if !rules[0].Grants("go") {
		t.Errorf("the rule does not grant go: %+v", rules[0])
	}
	// A rule written for a path called "go" beneath the working directory
	// would grant nothing here, and would not settle a call either.
	if permitted := config.PermittedCommands(rules, s.tools.dir); len(permitted) == 0 {
		t.Errorf("the rule covers nothing here: %+v", rules[0])
	}
}

// A directory named with a separator is still taken as one.
func TestANamedDirectoryIsStillTaken(t *testing.T) {
	s, _ := permissionSession(t)
	other := t.TempDir()

	s.cmdPermission([]string{"add", other, "go"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("the rules read back as %+v, want one", rules)
	}
	if permitted := config.PermittedCommands(rules, other); len(permitted) == 0 {
		t.Errorf("the rule does not cover the directory named: %+v", rules[0])
	}
}

// A directory named without a separator, by a relative name, is taken as one
// when it is a directory here.
func TestARelativeDirectoryIsTaken(t *testing.T) {
	s, _ := permissionSession(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("the working directory: %v", err)
	}
	if err := os.Chdir(s.tools.dir); err != nil {
		t.Fatalf("changing to the session directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Mkdir("sub", 0o755); err != nil {
		t.Fatalf("making the subdirectory: %v", err)
	}

	s.cmdPermission([]string{"add", "sub", "go"})

	rules, err := config.LoadRules(nil)
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("the rules read back as %+v, want one", rules)
	}
	if permitted := config.PermittedCommands(rules, filepath.Join(s.tools.dir, "sub")); len(permitted) == 0 {
		t.Errorf("the rule does not cover the subdirectory: %+v", rules[0])
	}
}
