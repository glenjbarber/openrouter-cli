package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// isolateHome points the loader at a temporary home directory and clears the
// XDG variable, so that a test never reads or writes the maintainer's own
// configuration. Every test that calls Load or InstallDefault uses it.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

// writeAt writes body to path at an exact mode, bypassing the mode the
// process umask would otherwise apply.
func writeAt(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// WriteFile filters the mode through the umask, so it is set again here.
	// A test asserting a permissive mode would otherwise be asserting the
	// umask rather than the code.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
}

// The credential is read from the file and from nowhere else. A variable of the
// same name in the environment is ignored even when it is set, which is what
// stops a stale export from defeating the file.
func TestAPIKeyIsNotReadFromTheEnvironment(t *testing.T) {
	isolateHome(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-from-the-environment")

	_, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound with no file present", err)
	}
	if strings.Contains(err.Error(), "from-the-environment") {
		t.Errorf("err = %q, want no part of the environment credential", err)
	}
}

// A file holding no key is still missing one even when the environment carries
// a value, so the session reports the absence rather than quietly using
// whatever the environment happened to hold.
func TestEnvironmentKeyDoesNotFillInAKeylessFile(t *testing.T) {
	home := isolateHome(t)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-from-the-environment")
	writeAt(t, filepath.Join(home, DefaultFileName), `{"OPENROUTER_MODEL":"a/b"}`, 0o600)

	_, err := Load()
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
}

// The search stops at the first match and the results are not merged, so a file
// at the primary path wins outright and nothing held only at a later path is
// picked up.
func TestSearchStopsAtTheFirstMatch(t *testing.T) {
	home := isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	primary := `{"OPENROUTER_MODEL":"first/model"}`
	writeAt(t, filepath.Join(home, DefaultFileName), primary, 0o600)

	dir := filepath.Join(xdg, "openrouter-cli")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	later := `{"OPENROUTER_API_KEY":"sk-or-v1-later","OPENROUTER_MODEL":"second/model"}`
	writeAt(t, filepath.Join(dir, "openrouter-cli.json"), later, 0o600)

	_, err := Load()
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want the first match to win with no key", err)
	}
	if noKey.Model != "first/model" {
		t.Errorf("Model = %q, want the first match alone", noKey.Model)
	}
	if strings.Contains(err.Error(), "sk-or-v1-later") {
		t.Errorf("err = %q, want no part of the later credential", err)
	}
}

// The first match wins with a credential as well, so a second file cannot
// supply one that the first did not.
func TestSearchDoesNotMergeWithTheSecondFile(t *testing.T) {
	home := isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	primary := `{"OPENROUTER_API_KEY":"sk-or-v1-first","OPENROUTER_MODEL":"first/model"}`
	writeAt(t, filepath.Join(home, DefaultFileName), primary, 0o600)

	dir := filepath.Join(xdg, "openrouter-cli")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	later := `{"OPENROUTER_API_KEY":"sk-or-v1-second","OPENROUTER_MODEL":"second/model"}`
	writeAt(t, filepath.Join(dir, "openrouter-cli.json"), later, 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-first" {
		t.Errorf("APIKey = %q, want the credential at the first path", cfg.APIKey)
	}
	if cfg.Model != "first/model" {
		t.Errorf("Model = %q, want the model at the first path", cfg.Model)
	}
}

// A configuration under XDG_CONFIG_HOME is found when the primary path holds
// nothing, so the variable is honoured in place of ~/.config.
func TestLoadHonoursXDGConfigHome(t *testing.T) {
	isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir := filepath.Join(xdg, "openrouter-cli")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	body := `{"OPENROUTER_API_KEY":"sk-or-v1-in-xdg"}`
	writeAt(t, filepath.Join(dir, "openrouter-cli.json"), body, 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-in-xdg" {
		t.Errorf("APIKey = %q, want the credential under XDG_CONFIG_HOME", cfg.APIKey)
	}
	if cfg.Path != filepath.Join(dir, "openrouter-cli.json") {
		t.Errorf("Path = %q, want the file under XDG_CONFIG_HOME", cfg.Path)
	}
}

// A relative XDG_CONFIG_HOME is ignored, since resolving one against the
// working directory would let a directory someone else wrote supply the
// credential file.
func TestLoadIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := isolateHome(t)
	t.Setenv("XDG_CONFIG_HOME", "relative-config-home")

	dir := filepath.Join(home, ".config", "openrouter-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{"OPENROUTER_API_KEY":"sk-or-v1-under-dot-config"}`
	writeAt(t, filepath.Join(dir, "openrouter-cli.json"), body, 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-under-dot-config" {
		t.Errorf("APIKey = %q, want the file below the home directory", cfg.APIKey)
	}
}

// With no file at any path the error names every path searched, so a reader can
// tell where the client looked for one.
func TestNotFoundNamesEverySearchedPath(t *testing.T) {
	home := isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	_, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	for _, want := range []string{
		filepath.Join(home, DefaultFileName),
		filepath.Join(xdg, "openrouter-cli", "openrouter-cli.json"),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to name %q", err, want)
		}
	}
}

// An unknown key is ignored, so a file written for a newer version stays
// readable by an older one.
func TestParseIgnoresUnknownKeys(t *testing.T) {
	path := writeConfig(t, `{"OPENROUTER_API_KEY":"k","FUTURE_SETTING":{"a":[1,2]}}`)

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.APIKey != "k" {
		t.Errorf("APIKey = %q, want it read past the unknown key", cfg.APIKey)
	}
}

// An empty key is a missing key rather than a credential, since a request
// carrying it would be refused by the backend with a diagnostic naming a key
// that was never set.
func TestEmptyKeyIsAMissingKey(t *testing.T) {
	for name, body := range map[string]string{
		"empty":       `{"OPENROUTER_API_KEY":""}`,
		"no key":      `{"OPENROUTER_URL_BASE":"https://openrouter.ai/api/v1"}`,
		"null":        `{"OPENROUTER_API_KEY":null}`,
		"only spaces": `{"OPENROUTER_API_KEY":"   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, body)

			_, err := parse(path)
			var noKey *ErrNoAPIKey
			if !errors.As(err, &noKey) {
				t.Fatalf("err = %v, want *ErrNoAPIKey", err)
			}
		})
	}
}

// A key of the wrong JSON type is a fault rather than an ordinary state, so it
// is reported as a malformed file and not as a missing key. The diagnostic
// names the field and its type, and carries no part of the file.
func TestWronglyTypedKeyIsAMalformedFile(t *testing.T) {
	path := writeConfig(t, `{"OPENROUTER_API_KEY":42}`)

	_, err := parse(path)
	if err == nil {
		t.Fatal("parse accepted a numeric key, want an error")
	}
	var noKey *ErrNoAPIKey
	if errors.As(err, &noKey) {
		t.Errorf("err = %v, want a malformed-file error rather than a missing key", err)
	}
}

// A diagnostic names the file and never the credential or the contents. The
// message is the first thing a reader pastes into a report.
func TestDiagnosticsCarryNoCredential(t *testing.T) {
	const secret = "sk-or-v1-must-not-be-printed"

	for name, tc := range map[string]struct {
		body string
		mode os.FileMode
	}{
		"permissive mode": {`{"OPENROUTER_API_KEY":"` + secret + `"}`, 0o644},
		"malformed json":  {`{"OPENROUTER_API_KEY":"` + secret + `"`, 0o600},
		"not json at all": {secret, 0o600},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultFileName)
			writeAt(t, path, tc.body, tc.mode)

			_, err := parse(path)
			if err == nil {
				t.Fatal("parse accepted the file, want an error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %q, want no part of the credential", err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("err = %q, want it to name the file %q", err, path)
			}
		})
	}
}

// The default document written at startup carries the endpoint and nothing
// else, since a placeholder key would be indistinguishable from a real one and
// would be sent to the backend.
func TestDefaultFileCarriesNoKey(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(DefaultFile), &doc); err != nil {
		t.Fatalf("the default document is not valid JSON: %v", err)
	}
	if _, ok := doc["OPENROUTER_API_KEY"]; ok {
		t.Error("the default document writes a key, want it left absent")
	}
	if len(doc) != 1 {
		t.Errorf("keys = %v, want the endpoint alone", doc)
	}
	base, _ := doc["OPENROUTER_URL_BASE"].(string)
	if base != DefaultURLBase {
		t.Errorf("OPENROUTER_URL_BASE = %q, want %q", base, DefaultURLBase)
	}
}

// The default written at startup resolves to the same endpoint the client uses
// when the file names none, so the file states the value in force rather than
// something the loader will change.
func TestDefaultFileResolvesToTheDefaultEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	writeAt(t, path, DefaultFile, 0o600)

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if noKey.URLBase != DefaultURLBase {
		t.Errorf("URLBase = %q, want %q", noKey.URLBase, DefaultURLBase)
	}
}

// The creation is exclusive, so a file appearing between the existence check
// and the write is not overwritten. Writers racing on one path must produce
// exactly one winner and exactly one default.
func TestInstallDefaultCreatesExclusively(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)

	const writers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	written := make([]bool, writers)
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			written[i], errs[i] = InstallDefaultAt(path)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := range written {
		if errs[i] != nil {
			t.Errorf("InstallDefaultAt: %v", errs[i])
			continue
		}
		if written[i] {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("winners = %d, want exactly one", winners)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != DefaultFile {
		t.Errorf("file = %q, want the default and nothing interleaved", string(data))
	}
}

// A default is written when nothing is found anywhere, and the file it writes
// is the one the search reaches first.
func TestInstallDefaultWritesAtThePrimaryPath(t *testing.T) {
	home := isolateHome(t)

	written, err := InstallDefault()
	if err != nil {
		t.Fatalf("InstallDefault: %v", err)
	}
	if !written {
		t.Fatal("written = false, want true with no file anywhere")
	}

	data, err := os.ReadFile(filepath.Join(home, DefaultFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != DefaultFile {
		t.Errorf("file = %q, want %q", string(data), DefaultFile)
	}

	info, err := os.Stat(filepath.Join(home, DefaultFileName))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != RequiredMode {
		t.Errorf("mode = %04o, want %04o", got, RequiredMode)
	}
}

// A directory at a later search path does not stop the default being written,
// and the file written then takes precedence over it on the next load.
func TestInstallDefaultWritesOverAShadowedDirectory(t *testing.T) {
	home := isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if err := os.MkdirAll(filepath.Join(xdg, "openrouter-cli", "openrouter-cli.json"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	written, err := InstallDefault()
	if err != nil {
		t.Fatalf("InstallDefault: %v", err)
	}
	if !written {
		t.Fatal("written = false, want the default written at the primary path")
	}

	_, err = Load()
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want the default found with no key", err)
	}
	if noKey.Path != filepath.Join(home, DefaultFileName) {
		t.Errorf("Path = %q, want the primary path", noKey.Path)
	}
}
