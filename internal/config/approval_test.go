package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPermittedCommandsTakesTheNearestEnclosingRule(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "work")
	child := filepath.Join(project, "repo", "internal")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("making the tree: %v", err)
	}

	rules := []ApprovalRule{
		{Path: root, Commands: []string{"go"}},
		{Path: project, Commands: []string{"go", "make"}},
	}

	got := PermittedCommands(rules, child)
	if !slices.Equal(got, []string{"go", "make"}) {
		t.Errorf("the nearest rule did not decide: got %v", got)
	}
	// The rule above the nearest one is not merged in, or no rule could ever
	// narrow anything granted above it.
	if slices.Contains(got, "git") {
		t.Errorf("rules were merged rather than the nearest deciding: %v", got)
	}
}

func TestPermittedCommandsACoveredChildNeedsNoRuleOfItsOwn(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("making the tree: %v", err)
	}

	rules := []ApprovalRule{{Path: root, Commands: []string{"go", "gofmt"}}}

	got := PermittedCommands(rules, child)
	if !slices.Equal(got, []string{"go", "gofmt"}) {
		t.Errorf("a child of a permitted directory was not permitted: %v", got)
	}
}

func TestPermittedCommandsADirectoryOutsideEveryRuleIsNotPermitted(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()

	got := PermittedCommands([]ApprovalRule{{Path: root, Commands: []string{"go"}}}, elsewhere)
	if got != nil {
		t.Errorf("a directory outside the rules was permitted: %v", got)
	}
}

func TestPermittedCommandsASiblingWithASharedPrefixIsNotCovered(t *testing.T) {
	// The failure a string prefix check makes, and the reason the comparison
	// is by path rather than by prefix: /tmp/work does not cover /tmp/workspace.
	base := t.TempDir()
	work := filepath.Join(base, "work")
	workspace := filepath.Join(base, "workspace")
	for _, dir := range []string{work, workspace} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("making %s: %v", dir, err)
		}
	}

	got := PermittedCommands([]ApprovalRule{{Path: work, Commands: []string{"go"}}}, workspace)
	if got != nil {
		t.Errorf("a sibling sharing a name prefix was permitted: %v", got)
	}
}

func TestPermittedCommandsTheBaseItselfIsCovered(t *testing.T) {
	dir := t.TempDir()

	got := PermittedCommands([]ApprovalRule{{Path: dir, Commands: []string{"go"}}}, dir)
	if !slices.Equal(got, []string{"go"}) {
		t.Errorf("the rule did not cover its own directory: %v", got)
	}
}

func TestPermittedCommandsTrimsTheNames(t *testing.T) {
	dir := t.TempDir()

	got := PermittedCommands([]ApprovalRule{{Path: dir, Commands: []string{" go ", "make", "  "}}}, dir)
	if !slices.Equal(got, []string{"go", "make"}) {
		t.Errorf("the names were not trimmed: %v", got)
	}
}

func TestPermittedCommandsARuleWithNoPathIsSkipped(t *testing.T) {
	dir := t.TempDir()

	got := PermittedCommands([]ApprovalRule{{Commands: []string{"go"}}}, dir)
	if got != nil {
		t.Errorf("a rule naming no directory was applied: %v", got)
	}
}

func TestPermitsReadsTheNearestRule(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "repo")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("making the tree: %v", err)
	}

	cfg := &Config{Tools: []ApprovalRule{
		{Path: root, Commands: []string{"go"}},
		{Path: child, Commands: []string{"make"}},
	}}

	if !cfg.Permits("make", child) {
		t.Error("the nearest rule did not permit its command")
	}
	if cfg.Permits("go", child) {
		t.Error("the rule above was merged in, so no rule can narrow anything")
	}
	if !cfg.Permits("go", root) {
		t.Error("the outer rule did not apply at its own directory")
	}
}

func TestPermitsWithoutAConfigurationIsFalse(t *testing.T) {
	var cfg *Config

	if cfg.Permits("go", t.TempDir()) {
		t.Error("a session with no configuration permitted something")
	}
}

func TestTheFileCarriesApprovalRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openrouter-cli.json")
	body := `{
	  "OPENROUTER_API_KEY": "k",
	  "OPENROUTER_TOOLS": [
	    {"path": ".", "commands": ["go", "gofmt"]},
	    {"path": "/srv/project", "commands": ["make"]}
	  ]
	}`
	if err := os.WriteFile(path, []byte(body), RequiredMode); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(cfg.Tools) != 2 {
		t.Fatalf("the rules were not read: %+v", cfg.Tools)
	}
	if !cfg.Tools[0].Grants("go") {
		t.Error("the first rule does not grant go")
	}
	if cfg.Tools[0].Grants("rm") {
		t.Error("the first rule granted something it did not name")
	}
}

func TestARuleIsReadUnderTheNamesTheSchemaGives(t *testing.T) {
	// The keys are lower case, since a rule is not an environment variable
	// and does not need to be transferable into one. The test exists because
	// a mismatch between the tag and the field is a rule that is silently
	// never read.
	var rule ApprovalRule
	if err := json.Unmarshal([]byte(`{"path":"/x","commands":["go"]}`), &rule); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if rule.Path != "/x" || !rule.Grants("go") {
		t.Errorf("the rule did not decode: %+v", rule)
	}
}

func TestRulesTravelWithAMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openrouter-cli.json")
	body := `{
	  "setup_complete": true,
	  "OPENROUTER_TOOLS": [{"path": ".", "commands": ["go"]}]
	}`
	if err := os.WriteFile(path, []byte(body), RequiredMode); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// The file was read even though it carries no credential, so the rules it
	// carries were read too. A rule dropped here would be a file that looks
	// as though it had not been read.
	if len(cfg.Tools) != 1 {
		t.Errorf("the rules were dropped with the key: %+v", cfg.Tools)
	}
	if !cfg.Permits("go", dir) {
		t.Error("the rule did not apply to the directory it was written in")
	}
}
