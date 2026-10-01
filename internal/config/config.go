// Package config loads the read-only JSON configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	APIKey   string `json:"OPENROUTER_API_KEY"`
	URLBase  string `json:"OPENROUTER_URL_BASE"`
	SetupKey bool   `json:"setup_complete"`
}

// Config is the resolved configuration.
type Config struct {
	APIKey  string
	URLBase string
	// Path is the file the values were read from, empty when none was found.
	Path string
	// Skipped reports that setup was carried out and deliberately skipped,
	// which is distinct from a completed setup holding a usable credential.
	Skipped bool
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
}

// Error implements the error interface.
func (e *ErrNoAPIKey) Error() string {
	return fmt.Sprintf("%s does not contain OPENROUTER_API_KEY", e.Path)
}

// Empty returns a configuration with no key and the default endpoint, used when
// the file exists but carries no credential.
//
// It is returned by value so that a caller cannot retain a pointer into the
// loader state.
func Empty() *Config {
	return &Config{URLBase: DefaultURLBase}
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

	for _, name := range fileNames {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(home, filepath.FromSlash(name))
		}
		if resolved, ok := xdgOverride(name); ok {
			path = resolved
		}

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
	return nil, fmt.Errorf("%w: searched %s", ErrNotFound, filepath.Join(home, fileNames[0]))
}

// xdgOverride redirects the XDG relative path to $XDG_CONFIG_HOME when that
// variable is set.
func xdgOverride(name string) (string, bool) {
	if name != fileNames[1] {
		return "", false
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		return "", false
	}
	// Strip the leading ".config" component and re-root it.
	rel := filepath.FromSlash("openrouter-cli/openrouter-cli.json")
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

	cfg := &Config{
		APIKey:  raw.APIKey,
		URLBase: raw.URLBase,
		Path:    path,
		Skipped: raw.SetupKey && raw.APIKey == "",
	}

	if cfg.APIKey == "" {
		if raw.SetupKey {
			return cfg, nil
		}
		return nil, &ErrNoAPIKey{Path: path}
	}

	if cfg.URLBase == "" {
		cfg.URLBase = DefaultURLBase
	}
	return cfg, nil
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
