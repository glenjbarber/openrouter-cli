package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const colorSecret = "sk-or-SECRET-KEY-VALUE"

// colorFile is a file with a key, an unknown key and a theme object, in a
// layout the writer would not produce, so that a rewritten value shows.
const colorFile = "{\n  \"OPENROUTER_API_KEY\": \"" + colorSecret + "\",\n" +
	"  \"future_key\": [1,   2, {\"a\":  \"<&>\"}],\n" +
	"  \"color_theme\": {  \"foreground\": \"red\",\"background\":\"#00ff00\" },\n" +
	"  \"color\": false\n}\n"

func writeColorFixture(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openrouter-cli.json")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWriteColorFlipsOnlyColor(t *testing.T) {
	path := writeColorFixture(t, colorFile, 0o600)
	untouched := []string{
		`"` + colorSecret + `"`,
		`[1,   2, {"a":  "<&>"}]`,
		`{  "foreground": "red","background":"#00ff00" }`,
	}
	for _, on := range []bool{true, false} {
		if err := WriteColor(path, on); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		for _, raw := range untouched {
			if !bytes.Contains(got, []byte(raw)) {
				t.Errorf("on=%v: %q was not carried over byte for byte in\n%s", on, raw, got)
			}
		}
		want := `"color": false`
		if on {
			want = `"color": true`
		}
		if !strings.Contains(string(got), want) {
			t.Errorf("on=%v: %s missing in\n%s", on, want, got)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode is %v", info.Mode().Perm())
		}
		if _, err := os.Stat(path + ".new"); err == nil {
			t.Error("temporary file left behind")
		}
	}
}

func TestWriteColorAddsTheKeyWhenAbsent(t *testing.T) {
	path := writeColorFixture(t, `{"OPENROUTER_API_KEY": "`+colorSecret+`"}`, 0o600)
	if err := WriteColor(path, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), `"color": true`) || !strings.Contains(string(got), colorSecret) {
		t.Errorf("unexpected file:\n%s", got)
	}
}

func TestWriteColorNeverCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	if err := WriteColor(path, true); err == nil {
		t.Fatal("a missing file was accepted")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the file was created")
	}
	if err := WriteColor("", true); err != ErrNoConfigFile {
		t.Errorf("empty path: %v", err)
	}
}

func TestWriteColorRefusesAndLeavesTheFile(t *testing.T) {
	cases := map[string]struct {
		body string
		mode os.FileMode
	}{
		"mode 0644":    {colorFile, 0o644},
		"invalid JSON": {`{"OPENROUTER_API_KEY": "` + colorSecret + `", `, 0o600},
		"not object":   {`["` + colorSecret + `"]`, 0o600},
		"null":         {`null`, 0o600},
		"string color": {`{"OPENROUTER_API_KEY": "` + colorSecret + `", "color": "yes"}`, 0o600},
		"null color":   {`{"color": null}`, 0o600},
	}
	for name, c := range cases {
		path := writeColorFixture(t, c.body, c.mode)
		err := WriteColor(path, true)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), colorSecret) {
			t.Errorf("%s: the error carries file content: %v", name, err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != c.body {
			t.Errorf("%s: the file changed", name)
		}
	}
}

func TestWriteColorLoadsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".openrouter-cli.json")
	body := `{"OPENROUTER_API_KEY": "` + colorSecret + `"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{true, false} {
		if err := WriteColor(path, on); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Color != on {
			t.Errorf("Color is %v after writing %v", cfg.Color, on)
		}
	}
}
