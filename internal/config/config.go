// Package config loads the read-only JSON configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultURLBase is used when the file does not set OPENROUTER_URL_BASE.
const DefaultURLBase = "https://openrouter.ai/api/v1"

// RequiredMode is the only file mode the client accepts. A more permissive
// mode would expose the API key to other accounts on the system, so it is a
// hard failure rather than a warning.
const RequiredMode os.FileMode = 0o600

// fileNames are searched in order. The first match wins and the search stops.
var fileNames = []string{
	".openrouter-cli.json",
	filepath.Join(".config", "openrouter-cli", "openrouter-cli.json"),
}

// rawFile is the on-disk shape. Keys are named after the equivalent
// environment variables so a value is transferable between the two.
type rawFile struct {
	APIKey  string `json:"OPENROUTER_API_KEY"`
	URLBase string `json:"OPENROUTER_URL_BASE"`
	Model   string `json:"OPENROUTER_MODEL"`
	Bell    bool   `json:"OPENROUTER_BELL"`
	// SetupKey keeps its name from the open decision about the setup state.
	SetupKey bool `json:"setup_complete"`
	// Mouse asks for mouse reporting. It is a preference rather than a
	// setting, since the decision is settled per session by where the
	// session is running and by whether the reader can select text.
	Mouse bool `json:"OPENROUTER_MOUSE"`
	// Tools are the approval rules, read from OPENROUTER_TOOLS. A rule
	// permits some programs in a directory without asking, which is the
	// only thing in the file that grants a capability the client would
	// otherwise ask about on every call.
	Tools []ApprovalRule `json:"OPENROUTER_TOOLS"`
}

// Config is the resolved configuration.
type Config struct {
	APIKey string
	// Model is the model requests are sent to when the session has not chosen
	// one. It is empty when the file does not set it, which is not an error:
	// the model may be chosen at runtime instead.
	Model string
	// Bell reports whether the terminal bell is rung when a reply arrives. It
	// is a preference rather than a mode: nothing else in the client changes
	// because of it.
	Bell    bool
	URLBase string
	// Path is the file the values were read from, empty when none was found.
	Path string
	// Skipped reports that setup was carried out and deliberately skipped,
	// which is distinct from a completed setup holding a usable credential.
	Skipped bool
	// Mouse reports that the file asks for mouse reporting. It is honoured
	// only outside tmux, since inside tmux the drag that begins a selection
	// is the one gesture a reader is most likely to want.
	Mouse bool
	// Tools are the approval rules the file carries, in the order they were
	// written. The order is kept rather than reduced to one resolved set,
	// since which rule matched is what a reader needs to be able to see.
	Tools []ApprovalRule
}

// Permits reports whether the configuration permits a command for a directory
// without asking.
//
// The nearest enclosing rule decides, and a directory inside a permitted one
// is permitted with a rule of its own. A session assembled without a
// configuration still asks about everything.
func (c *Config) Permits(command, dir string) bool {
	if c == nil {
		return false
	}
	for _, name := range PermittedCommands(c.Tools, dir) {
		if name == strings.TrimSpace(command) {
			return true
		}
	}
	return false
}

// ErrNotFound reports that no configuration file exists at any search path.
var ErrNotFound = errors.New("no configuration file found")

// ErrNoAPIKey reports that the file exists but carries no usable key.
//
// It is distinct from a read failure so that the interface may open and report
// the absence rather than refusing to start. A key that was never entered is an
// ordinary state, whereas a file that could not be parsed is a fault.
type ErrNoAPIKey struct {
	// Path is the file that was read.
	Path string
	// Bell is the bell preference, carried through so a caller standing in a
	// configuration does not lose it.
	Bell bool
	// Model is the model the file prefers, carried through so that the
	// caller can stand in a configuration without losing the preference.
	Model string
	// Mouse is the mouse preference the file carries, carried through on the
	// same reasoning as the model: a file that cannot be read for its key has
	// still been read.
	Mouse bool
	// URLBase is the endpoint the file asks for. It is carried through for the
	// same reason: a reader whose key has not been entered yet is still talking
	// to whichever endpoint the file names.
	URLBase string
	// Tools are the approval rules the file carries. They travel with the
	// error on the same reasoning as the model and the endpoint: a rule is
	// not a credential, and dropping it would make a file that was read look
	// as though it had not been.
	Tools []ApprovalRule
}

// Error implements the error interface.
func (e *ErrNoAPIKey) Error() string {
	return fmt.Sprintf("%s does not contain OPENROUTER_API_KEY", e.Path)
}

// Empty returns the configuration used when a file exists but carries no
// credential.
//
// The model is taken from the file even though the key is not, since a session
// with no key cannot reach a model anyway and losing the preference would make
// the file appear not to have been read.
func Empty(model string, bell bool) *Config {
	return &Config{
		URLBase: DefaultURLBase,
		Model:   strings.TrimSpace(model),
		Bell:    bell,
	}
}

// EmptyMouse returns a configuration that asks for mouse reporting without
// carrying a credential, so that a file which cannot be read for its key is
// not also a file whose other preferences are lost.
func EmptyMouse(model string, mouse, bell bool) *Config {
	cfg := Empty(model, bell)
	cfg.Mouse = mouse
	return cfg
}

// Load reads the configuration from the first file found at a search path.
//
// The process environment is deliberately not consulted. An OPENROUTER_API_KEY
// in the environment is ignored even when it is set, so a key present there
// cannot silently take effect in place of the file.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locating the home directory: %w", err)
	}

	paths := searchPaths(home)
	for _, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("examining %s: %w", path, err)
		}
		if info.IsDir() {
			continue
		}
		return parse(path)
	}
	return nil, fmt.Errorf("%w: searched %s", ErrNotFound, strings.Join(paths, " and "))
}

// searchPaths returns the candidate paths in the order they are searched. The
// first match wins and the results are not merged.
//
// The paths are shared with the installer rather than being written twice, so
// that a default written at the first path cannot shadow a configuration found
// at a later one.
// SearchPaths returns the configuration file locations in the order the
// loader checks them, so that a diagnostic can name where the client looks
// rather than one path the reader may not be using.
//
// It is exported so that the interface can point a reader at a file that
// exists. A reader whose file is under XDG was told to edit a path under the
// home directory that was never read, which is the one file that cannot be
// where the answer is.
func SearchPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return searchPaths(home)
}

func searchPaths(home string) []string {
	paths := make([]string, 0, len(fileNames))
	for _, name := range fileNames {
		// The XDG check comes first, since it matches the relative name and is
		// re-rooted by itself. Joining first would compare a home-qualified
		// path against a relative one and never match.
		if resolved, ok := xdgOverride(name); ok {
			paths = append(paths, resolved)
			continue
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(home, filepath.FromSlash(name))
		}
		paths = append(paths, name)
	}
	return paths
}

// xdgOverride redirects the XDG relative path to $XDG_CONFIG_HOME when that
// variable is set.
//
// A relative value is ignored. The specification requires the variable to
// hold an absolute path, and a relative one would be resolved against the
// working directory, so running the client in a directory someone else wrote
// would read a credential file out of it.
func xdgOverride(name string) (string, bool) {
	if name != fileNames[1] {
		return "", false
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" || !filepath.IsAbs(base) {
		return "", false
	}
	// The leading .config element is exactly what the variable replaces, so
	// what remains of the search path is the part below it. It is derived
	// rather than written out a second time, since two copies of one path
	// drift apart the moment either is edited.
	rel, err := filepath.Rel(filepath.FromSlash(".config"), fileNames[1])
	if err != nil {
		return "", false
	}
	return filepath.Join(base, rel), true
}

// parse reads and validates a single configuration file.
func parse(path string) (*Config, error) {
	if err := checkMode(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var raw rawFile
	// Unknown keys are ignored rather than rejected, so a file written for a
	// newer version stays readable by an older one.
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}

	// A key of nothing but whitespace is not a credential. The value itself is
	// passed on exactly as written, since altering what goes on the wire is
	// not this loader's decision, but a key that is blank once trimmed is
	// treated as unset so that the interface reports an absent key rather than
	// sending one the backend will reject.
	unset := strings.TrimSpace(raw.APIKey) == ""

	cfg := &Config{
		APIKey:  raw.APIKey,
		Model:   strings.TrimSpace(raw.Model),
		Bell:    raw.Bell,
		URLBase: resolveURLBase(raw.URLBase),
		Path:    path,
		Skipped: raw.SetupKey && unset,
		Mouse:   raw.Mouse,
		Tools:   raw.Tools,
	}

	if unset {
		// The setup state is reported as an error so that a caller can stand in
		// a configuration, and every value the file does carry travels with it.
		// A preference dropped here would make a file that was read look as
		// though it had not been.
		if raw.SetupKey {
			return cfg, nil
		}
		return nil, &ErrNoAPIKey{
			Path:    path,
			Bell:    raw.Bell,
			Model:   cfg.Model,
			Mouse:   raw.Mouse,
			URLBase: cfg.URLBase,
			Tools:   raw.Tools,
		}
	}
	return cfg, nil
}

// resolveURLBase applies the endpoint override rule.
//
// An override carrying a path is used verbatim, so a self-hosted deployment
// that serves the endpoints from somewhere other than /api/v1 keeps working.
// The suffix is appended only to a bare scheme and host, such as
// http://localhost:3000, which names an endpoint rather than a prefix.
//
// A value that does not parse as a URL is passed on as written, so that the
// backend reports it rather than this client guessing at what was meant.
func resolveURLBase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultURLBase
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if strings.Trim(u.Path, "/") != "" {
		return raw
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1"
	return u.String()
}

// checkMode enforces the 0600 requirement.
//
// The check is on the file alone. A parent directory at 0755 is ordinary and is
// not itself a credential exposure.
func checkMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("examining %s: %w", path, err)
	}
	mode := info.Mode().Perm()
	if mode == RequiredMode {
		return nil
	}
	return fmt.Errorf(
		"%s has mode %04o, but %04o is required: the file holds an API key, "+
			"and any other mode can expose it to other accounts on this system. "+
			"Run: chmod 0600 %s",
		path, mode, RequiredMode, path,
	)
}

// ApprovalRule permits some commands in a directory without asking.
//
// The rule is written for a directory rather than for a single call, since the
// question it answers is about a place rather than about a moment: a reader who
// trusts `go` in a project does not want to answer for every build in it, and
// one who does not is asked each time.
type ApprovalRule struct {
	// Path is the directory the rule covers, and every directory beneath it.
	// A relative path is taken against the directory the rule names as its
	// base, which is how a rule written as "." covers the session it was
	// written in.
	Path string
	// Commands are the programs permitted under Path. A command named by a
	// directory is not permitted, since the rule is about a program rather
	// than about a path that might reach one.
	Commands []string
}

// Grants reports whether the rule permits a command.
//
// The comparison is on the program name as written, with the same trimming the
// shell tool applies, so that a rule carrying a stray space around a name
// matches rather than being a rule that silently permits nothing.
func (r ApprovalRule) Grants(command string) bool {
	command = strings.TrimSpace(command)
	for _, name := range r.Commands {
		if strings.TrimSpace(name) == command {
			return true
		}
	}
	return false
}

// Config fields for approval are resolved from the rule list at load time, so
// that a rule matching the session is known before a tool is built rather than
// at the point of a call.

// PermittedCommands returns the programs a rule covering dir permits.
//
// The rules are searched from the nearest directory outwards, and the first
// one that names dir decides. A child of a permitted directory is therefore
// permitted without a rule of its own, which is the case the reader asked for:
// a rule written for a project covers the tree beneath it rather than needing
// one entry per repository.
//
// The nearest rule wins rather than the union of all of them. A union would
// make a rule unable to say anything, since every rule anywhere under the
// working directory would grant everything any other rule grants, and there
// would be no way to narrow a permission granted above.
func PermittedCommands(rules []ApprovalRule, dir string) []string {
	resolved, err := filepath.Abs(dir)
	if err != nil {
		resolved = filepath.Clean(dir)
	}
	top := resolved
	if r, err := filepath.EvalSymlinks(top); err == nil {
		top = r
	}

	best := -1
	bestLen := -1
	for i, rule := range rules {
		base := rule.Path
		if strings.TrimSpace(base) == "" {
			continue
		}
		if !filepath.IsAbs(base) {
			base = filepath.Join(top, base)
		}
		base = filepath.Clean(base)
		if r, err := filepath.EvalSymlinks(base); err == nil {
			base = r
		}
		if !covers(base, top) {
			continue
		}
		// The longest matching prefix is the nearest directory, since a
		// directory beneath another is a longer prefix of the same path.
		if depth := len(base); depth > bestLen {
			best, bestLen = i, depth
		}
	}
	if best < 0 {
		return nil
	}
	names := rules[best].Commands
	out := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// covers reports whether dir is base or is beneath it.
//
// Both are resolved paths and both are compared as paths rather than as strings,
// since a prefix check on a string would treat /home/gjb/work as covering
// /home/gjb/workspace.
func covers(base, dir string) bool {
	if base == dir {
		return true
	}
	rel, err := filepath.Rel(base, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RuleFile is the file the approval rules are kept in, apart from the
// configuration.
//
// The configuration file is read-only outside setup, since it holds the
// credential and a command that rewrote it could damage that. The rules are
// not a credential and a reader editing them by hand should not have to open a
// file whose permissions they have to get right. They are kept beside it
// instead, written by /permission and read by the same loader.
//
// A missing file is not an error: a reader who has set nothing up is asking no
// programs to run without a question, which is the state a fresh install is in.
func RuleFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the home directory: %w", err)
	}
	return filepath.Join(home, ".openrouter-cli", "permissions.json"), nil
}

// LoadRules reads the approval rules from the file beside the configuration.
//
// The configuration file is consulted as well, so that a reader who wrote rules
// into it by hand keeps them and the two are read as one set. The rules in the
// file beside it come last, so a rule written by /permission is the one a
// reader most recently said.
func LoadRules(cfg *Config) ([]ApprovalRule, error) {
	var rules []ApprovalRule
	if cfg != nil {
		rules = append(rules, cfg.Tools...)
	}

	path, err := RuleFile()
	if err != nil {
		return rules, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rules, nil
		}
		return rules, fmt.Errorf("reading %s: %w", path, err)
	}

	// A file written for a newer version stays readable, on the same terms as
	// the configuration file: unknown keys are ignored rather than refused.
	var written struct {
		Tools []ApprovalRule `json:"OPENROUTER_TOOLS"`
	}
	if err := json.Unmarshal(data, &written); err != nil {
		return rules, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return append(rules, written.Tools...), nil
}

// WriteRules writes the approval rules to the file beside the configuration.
//
// The file is written whole rather than edited, since a rule is a list and a
// partial edit of one is a list with a hole in it. It is written at 0600 for
// the same reason the configuration file is: it names the directories a model
// may run programs in, and that is a thing no other account on the system has
// any business reading.
func WriteRules(rules []ApprovalRule) error {
	path, err := RuleFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	body, err := json.MarshalIndent(struct {
		Tools []ApprovalRule `json:"OPENROUTER_TOOLS"`
	}{Tools: rules}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	// The file is written beside itself and renamed over, so that a reader
	// reading it never sees half of one list. A write interrupted by a crash
	// leaves the previous list rather than a truncated one.
	tmp := path + ".new"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// RemoveRule drops the rule covering a directory, and reports whether one was
// there.
//
// A directory is covered by the nearest enclosing rule rather than by an exact
// match, since that is how a rule is applied. Removing the one covering a
// directory therefore removes a permission the reader may have granted at a
// higher level, which is said in the report rather than left for the reader to
// discover from the rule that is now gone.
func RemoveRule(rules []ApprovalRule, dir string) ([]ApprovalRule, bool) {
	resolved := dir
	if abs, err := filepath.Abs(dir); err == nil {
		resolved = abs
	}
	if top, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = top
	}

	kept := make([]ApprovalRule, 0, len(rules))
	removed := false
	for _, rule := range rules {
		base := rule.Path
		if strings.TrimSpace(base) == "" {
			kept = append(kept, rule)
			continue
		}
		if !filepath.IsAbs(base) {
			base = filepath.Join(resolved, base)
		}
		if filepath.Clean(base) == resolved {
			removed = true
			continue
		}
		kept = append(kept, rule)
	}
	return kept, removed
}

// AddRule sets the rules for a directory, replacing any rule covering it.
//
// The replacement is by directory rather than by name, since a rule is about a
// place: a second rule for the same directory would have no way to say which
// one applies, and the nearest-wins search would make it depend on the order
// they happened to be written in.
func AddRule(rules []ApprovalRule, dir string, commands []string) []ApprovalRule {
	kept, _ := RemoveRule(rules, dir)
	return append(kept, ApprovalRule{Path: dir, Commands: commands})
}
