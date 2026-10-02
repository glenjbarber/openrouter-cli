package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const trustSecret = "sk-test-not-a-real-key"

func trustFixture(t *testing.T) (file, dir string) {
	t.Helper()
	root := t.TempDir()
	file = filepath.Join(root, DefaultFileName)
	body := `{"OPENROUTER_API_KEY": "` + trustSecret + `", "future_key": [1, 2]}`
	if err := os.WriteFile(file, []byte(body), RequiredMode); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return file, dir
}

func ask(t *testing.T, file, dir, answer string, interactive bool) (bool, string) {
	t.Helper()
	var out bytes.Buffer
	ok, err := EnsureTrusted(file, dir, strings.NewReader(answer), &out, interactive)
	if err != nil {
		t.Fatalf("EnsureTrusted: %v", err)
	}
	return ok, out.String()
}

func TestTrustApprovalIsRecordedAndNotAskedAgain(t *testing.T) {
	file, dir := trustFixture(t)
	if ok, out := ask(t, file, dir, "y\n", true); !ok || !strings.Contains(out, "[y/N]") {
		t.Fatalf("ok = %v, out = %q", ok, out)
	}
	if !IsTrusted(file, dir) {
		t.Fatal("directory not recorded as trusted")
	}
	// A second start asks nothing: empty input and a prompt-free output.
	if ok, out := ask(t, file, dir, "", true); !ok || out != "" {
		t.Fatalf("second ok = %v, out = %q", ok, out)
	}
}

func TestTrustPreservesFileContentAndMode(t *testing.T) {
	file, dir := trustFixture(t)
	ask(t, file, dir, "yes\n", true)

	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != RequiredMode {
		t.Errorf("mode = %04o", info.Mode().Perm())
	}
	data, _ := os.ReadFile(file)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["OPENROUTER_API_KEY"]) != `"`+trustSecret+`"` {
		t.Errorf("key changed: %s", got["OPENROUTER_API_KEY"])
	}
	if _, ok := got["future_key"]; !ok {
		t.Error("unknown key dropped")
	}
	if _, err := os.Stat(file + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Error("temporary file left behind")
	}
}

func TestTrustRefusalsRecordNothing(t *testing.T) {
	for _, answer := range []string{"", "\n", "n\n", "no\n", "maybe\n", "yy\n"} {
		file, dir := trustFixture(t)
		before, _ := os.ReadFile(file)
		if ok, _ := ask(t, file, dir, answer, true); ok {
			t.Errorf("answer %q trusted the directory", answer)
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(before, after) {
			t.Errorf("answer %q changed the file", answer)
		}
	}
}

func TestTrustNonInteractiveIsNotTrusted(t *testing.T) {
	file, dir := trustFixture(t)
	before, _ := os.ReadFile(file)
	ok, out := ask(t, file, dir, "y\n", false)
	if ok || out != "" {
		t.Fatalf("ok = %v, out = %q", ok, out)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Error("file changed")
	}
}

func TestTrustDefaultsToUntrustedOnError(t *testing.T) {
	file, dir := trustFixture(t)
	if IsTrusted(filepath.Join(t.TempDir(), "absent.json"), dir) {
		t.Error("missing file trusted")
	}
	// A wrong mode is refused for reading and for writing.
	if err := os.Chmod(file, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Trust(file, dir); err == nil {
		t.Error("Trust accepted a 0644 file")
	}
	var out bytes.Buffer
	if ok, err := EnsureTrusted(file, dir, strings.NewReader("y\n"), &out, true); ok || err == nil {
		t.Errorf("ok = %v, err = %v", ok, err)
	}
	// Malformed JSON and a wrongly typed key grant nothing.
	for _, body := range []string{`{`, `[]`, `{"OPENROUTER_TRUSTED": "x"}`} {
		os.WriteFile(file, []byte(body), RequiredMode)
		os.Chmod(file, RequiredMode)
		if IsTrusted(file, dir) {
			t.Errorf("%q trusted", body)
		}
		if err := Trust(file, dir); err == nil {
			t.Errorf("Trust rewrote %q", body)
		}
		if got, _ := os.ReadFile(file); string(got) != body {
			t.Errorf("file %q was altered", body)
		}
	}
	if ok, err := EnsureTrusted("", dir, strings.NewReader("y\n"), &out, true); ok || err == nil {
		t.Error("empty path trusted")
	}
}

func TestTrustIsNotInheritedByChildren(t *testing.T) {
	file, dir := trustFixture(t)
	ask(t, file, dir, "y\n", true)
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsTrusted(file, child) {
		t.Error("child inherited trust")
	}
}

func TestTrustIgnoresSymlinkSpelling(t *testing.T) {
	file, dir := trustFixture(t)
	ask(t, file, dir, "y\n", true)
	link := filepath.Join(filepath.Dir(file), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if !IsTrusted(file, link) {
		t.Error("link to a trusted directory not recognized")
	}
}
