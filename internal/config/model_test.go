package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHome points the loader at a temporary home directory, so that a test
// writes the runtime choices beside a configuration rather than beside the
// reader's own.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// A file that was never written is not an error, since a reader who has chosen
// nothing is asking for the configuration and nothing else.
func TestAMissingModelFileIsNotAnError(t *testing.T) {
	withHome(t)

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v, want a missing file to be nothing chosen", err)
	}
	if got.Model != "" || got.Provider != "" {
		t.Errorf("LoadChosen() = %+v, want nothing chosen", got)
	}
}

// A choice made at the keyboard survives a restart, which is the whole reason
// the file exists: /model is a choice about this session and a reader who has
// made it once should not have to make it again.
func TestAChosenModelSurvivesAReload(t *testing.T) {
	withHome(t)

	want := Chosen{Model: "openai/gpt-4o", Provider: "openrouter.ai"}
	if err := WriteChosen(want); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v", err)
	}
	if got != want {
		t.Errorf("LoadChosen() = %+v, want %+v", got, want)
	}
}

// A model chosen without a provider leaves the provider unset rather than
// inventing one, so that a reader who set one key is not given the other by
// the loader.
func TestAProviderIsNotInvented(t *testing.T) {
	withHome(t)

	if err := WriteChosen(Chosen{Model: "anthropic/claude"}); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v", err)
	}
	if got.Provider != "" {
		t.Errorf("Provider = %q, want it left unset", got.Provider)
	}
}

// The keys are named after the equivalent environment variables, so that a
// value is transferable between the file and the environment.
func TestTheKeysAreNamedAfterTheEnvironment(t *testing.T) {
	withHome(t)

	if err := WriteChosen(Chosen{Model: "openai/gpt-4o", Provider: "openrouter.ai"}); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(homeDir(t), ".openrouter-cli", "model.json"))
	if err != nil {
		t.Fatalf("reading the written file: %v", err)
	}

	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("the written file is not valid JSON: %v", err)
	}
	for _, key := range []string{"OPENROUTER_MODEL", "OPENROUTER_PROVIDER"} {
		if _, ok := written[key]; !ok {
			t.Errorf("the written file has no %s key: %s", key, data)
		}
	}
}

// The file sits beside a credential and is named by the same program, so it
// carries the same protection rather than a weaker one for being smaller.
func TestTheModelFileIsPrivate(t *testing.T) {
	withHome(t)

	if err := WriteChosen(Chosen{Model: "openai/gpt-4o"}); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}
	path, err := ModelFile()
	if err != nil {
		t.Fatalf("ModelFile() = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("examining %s: %v", path, err)
	}
	if mode := info.Mode().Perm(); mode != RequiredMode {
		t.Errorf("mode = %04o, want %04o", mode, RequiredMode)
	}
}

// A file written for a newer version stays readable by an older one, on the
// same terms as the configuration and the rules file.
func TestUnknownKeysAreIgnored(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".openrouter-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, "model.json")
	body := `{"OPENROUTER_MODEL":"openai/gpt-4o","SOMETHING_NEW":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v, want an unknown key ignored", err)
	}
	if got.Model != "openai/gpt-4o" {
		t.Errorf("Model = %q, want the known key still read", got.Model)
	}
}

// A file that cannot be parsed is a fault rather than a silence. Reporting it
// as nothing chosen would leave a reader believing the client had forgotten
// their model rather than that they had corrupted a file.
func TestACorruptModelFileIsReported(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".openrouter-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, "model.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	if _, err := LoadChosen(); err == nil {
		t.Error("LoadChosen() returned no error, want a corrupt file reported")
	}
}

// A provider is a label for the status bar and nothing more. Where requests
// are sent is settled by OPENROUTER_URL_BASE, so a name written for the screen
// must not repoint the client somewhere else.
func TestAProviderDoesNotRedirectTheClient(t *testing.T) {
	if got := resolveURLBase(""); got != DefaultURLBase {
		t.Errorf("resolveURLBase(\"\") = %q, want %q", got, DefaultURLBase)
	}
	if got := resolveURLBase("http://localhost:3000"); got != "http://localhost:3000/api/v1" {
		t.Errorf("resolveURLBase() = %q, want the /api/v1 suffix", got)
	}
}

// A provider written for the screen is not read as an endpoint, which is the
// whole reason the two keys are kept apart.
func TestAProviderIsNotReadAsAnEndpoint(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".openrouter-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, "model.json")
	body := `{"OPENROUTER_PROVIDER":"openrouter.ai"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v", err)
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want a provider not settling a model", got.Model)
	}
}

// A value that is nothing but whitespace is not a choice, since a reader who
// cleared the field has not chosen a model and have not chosen the absence of
// one either.
func TestWhitespaceIsNotAChoice(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".openrouter-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, "model.json")
	body := `{"OPENROUTER_MODEL":"   "}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v", err)
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want whitespace read as no choice", got.Model)
	}
}

// A write interrupted by a crash leaves the previous value rather than a
// truncated identifier, which would send every request to a model that does not
// exist.
func TestTheWriteLeavesNoPartialFile(t *testing.T) {
	withHome(t)

	if err := WriteChosen(Chosen{Model: "openai/gpt-4o"}); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}
	if err := WriteChosen(Chosen{Model: "anthropic/claude"}); err != nil {
		t.Fatalf("WriteChosen() = %v", err)
	}

	path, err := ModelFile()
	if err != nil {
		t.Fatalf("ModelFile() = %v", err)
	}
	if _, err := os.Stat(path + ".new"); err == nil {
		t.Errorf("%s.new is still there, want the temporary file renamed over", path)
	}

	got, err := LoadChosen()
	if err != nil {
		t.Fatalf("LoadChosen() = %v", err)
	}
	if got.Model != "anthropic/claude" {
		t.Errorf("Model = %q, want the later write", got.Model)
	}
}

// homeDir returns the temporary home a test is running under.
func homeDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("locating the home directory: %v", err)
	}
	return home
}

// The report a reader is pointed at names the file the loader will read.
func TestTheReportNamesTheFile(t *testing.T) {
	withHome(t)

	where := ModelFileWhere()
	if !strings.Contains(where, "model.json") {
		t.Errorf("ModelFileWhere() = %q, want it to name the file", where)
	}
}
