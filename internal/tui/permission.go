package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// `/permission` grants, removes and reports the programs a model may run in a
// directory without being asked.
//
// The rules live in a file beside the configuration rather than in it, since the
// configuration holds the credential and is treated as read-only outside setup.
// A reader editing rules should not have to open a file whose mode they have to
// get right, and a command rewriting one could damage the key in it.

// cmdPermission reports, grants or removes a permission.
//
// A directory may be given, in which case it is the one the session was opened
// in. Naming one is how a permission is granted for a directory the client
// cannot reach: the tools are contained to the working directory, so a rule
// naming somewhere else settles nothing until a session is opened there.
func (s *Session) cmdPermission(args []string) bool {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}

	dir, rest, err := s.permissionDir(args)
	if err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}

	rules, err := s.permissionRules()
	if err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}

	switch action {
	case "add", "grant":
		if len(rest) == 0 {
			s.addReply("(/permission add needs at least one program, " +
				"such as /permission add go, or /permission add go make; " +
				"name a directory to cover another, as in " +
				"/permission add ~/work go)")
			return false
		}
		s.writePermission(rules, config.AddRule(rules, dir, rest), dir,
			"permits "+strings.Join(rest, ", ")+" in "+dir)
	case "remove", "rm", "delete":
		kept, had := config.RemoveRule(rules, dir)
		if !had {
			s.addReply("(no rule covers " + dir + ", so there is nothing to remove)")
			return false
		}
		// A rule covering a directory the reader named is dropped whole, which
		// takes every program it permitted. Naming them would be a narrower
		// edit than the command offers, and a rule that permits three programs
		// and refuses one has no way to say so.
		s.writePermission(kept, kept, dir, "removed the rule covering "+dir)
	default:
		s.appendLines(strings.Join(s.permissionListing(rules), "\n"))
	}
	return false
}

// permissionDir is the directory a command is about.
//
// The session's own working directory is the default, since a reader granting a
// permission has almost always granted it for the place they are standing. The
// path is resolved so that the rule written is one the loader will match
// against, rather than a relative path whose meaning depends on where the
// session runs from.
func (s *Session) permissionDir(args []string) (string, []string, error) {
	if len(args) == 0 {
		// The working directory the client was opened in, rather than the
		// process one. They are the same in a running client, since nothing
		// moves it, and the tools are contained to the former, so a rule
		// written against the latter would settle nothing.
		if s.tools != nil && s.tools.dir != "" {
			return s.tools.dir, nil, nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return "", nil, err
		}
		return wd, nil, nil
	}
	// A directory is taken only when one is named, which is told by it being
	// a path rather than by being the first argument. The first argument is
	// usually a program, since the common form names none, and treating it as
	// a directory wrote a rule for a path that was never there and granted
	// nothing.
	if !isDirectoryArg(args[0]) {
		here, _, err := s.permissionDir(nil)
		return here, args, err
	}

	dir := args[0]
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", nil, err
		}
		dir = abs
	}
	if top, err := filepath.EvalSymlinks(dir); err == nil {
		dir = top
	}
	return dir, args[1:], nil
}

// isDirectoryArg reports whether an argument names a directory rather than a
// program.
//
// A path is one that carries a separator, one that exists, and one that names a
// directory that exists. The first two catch the ordinary case of a reader
// naming somewhere, and the third catches a relative name that is a directory
// here without a separator in it.
//
// A name that is a program and also happens to be a directory in the working
// directory is read as a program, since that is what it almost always is in this
// command and a reader wanting the directory can name it with a separator.
func isDirectoryArg(arg string) bool {
	if strings.ContainsAny(arg, "/\\") {
		return true
	}
	if info, err := os.Stat(arg); err == nil {
		return info.IsDir()
	}
	return false
}

// permissionRules is every rule the session knows about.
func (s *Session) permissionRules() ([]config.ApprovalRule, error) {
	return config.LoadRules(s.cfg)
}

// writePermission saves the rules and says what changed.
func (s *Session) writePermission(before, after []config.ApprovalRule, dir, what string) {
	if err := config.WriteRules(after); err != nil {
		s.addReply("(" + err.Error() + ")")
		return
	}
	// The in-memory rules are replaced too, or the session would go on deciding
	// from what it read at startup and a permission granted now would not take
	// effect until the next run.
	s.setRules(after)
	s.updateStatus()
	s.appendLines(what + ".")
	s.appendLines("  written to " + ruleFileWhere() + "; a rule covers the " +
		"directories beneath it, so a session anywhere under " + dir +
		" is covered by it")
}

// ruleFileWhere names the file the rules are in, for a message that points a
// reader at it.
func ruleFileWhere() string {
	path, err := config.RuleFile()
	if err != nil {
		return "the rules file"
	}
	return path
}

// permissionListing reports every rule and what it settles for here.
func (s *Session) permissionListing(rules []config.ApprovalRule) []string {
	if len(rules) == 0 {
		return []string{
			"no rules are set, so every program is put to you before it runs",
			"  /permission add go, or go make, permits them in this directory",
			"  written to " + ruleFileWhere(),
		}
	}

	// The directory the listing is about is the one the tools are contained
	// to, not the process one. They are the same in a running client, and a
	// listing that marked rules against a different directory would show a
	// rule as applying here when it settles nothing.
	wd := ""
	if s.tools != nil {
		wd = s.tools.dir
	}
	lines := []string{plural(len(rules), "rule") + " set:"}
	for _, rule := range rules {
		marker := "  "
		if wd != "" {
			if here := config.PermittedCommands(
				[]config.ApprovalRule{rule}, wd); len(here) > 0 {
				marker = "* "
			}
		}
		lines = append(lines, marker+rule.Path+": "+strings.Join(rule.Commands, ", "))
	}
	if wd != "" {
		if here := config.PermittedCommands(rules, wd); len(here) > 0 {
			sorted := append([]string(nil), here...)
			sort.Strings(sorted)
			lines = append(lines, "  permitted here: "+strings.Join(sorted, ", "))
		}
	}
	lines = append(lines, "  a * marks a rule covering this directory",
		"  written to "+ruleFileWhere())
	return lines
}

// setRules replaces the rules the session decides from.
//
// It is separate from the configuration, which is read once at startup and
// never written. A rule added by /permission has to take effect now rather than
// at the next run, or a reader who granted a permission would be asked anyway
// until they restarted.
func (s *Session) setRules(rules []config.ApprovalRule) {
	s.mu.Lock()
	s.rules = rules
	s.mu.Unlock()
}

// rulesFor returns the rules a call in dir is decided against.
func (s *Session) rulesFor(dir string) []config.ApprovalRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rules
}
