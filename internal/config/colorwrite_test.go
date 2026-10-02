package config

import (
	"encoding/json"
	"math/rand"
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

// colorCases are files in layouts the old writer would not have produced. Each
// holds the file, the exact file expected after turning color on, and likewise
// after turning it off. Only the value, or one inserted member, may differ.
var colorCases = []struct {
	name, body, on, off string
}{
	{"odd order with tabs",
		"{\n\t\"future_key\": [1,   2, {\"a\":  \"<&>\"}],\n\t\"color\": false,\n\t\"OPENROUTER_API_KEY\":   \"" + colorSecret + "\"\n}\n",
		"{\n\t\"future_key\": [1,   2, {\"a\":  \"<&>\"}],\n\t\"color\": true,\n\t\"OPENROUTER_API_KEY\":   \"" + colorSecret + "\"\n}\n",
		"{\n\t\"future_key\": [1,   2, {\"a\":  \"<&>\"}],\n\t\"color\": false,\n\t\"OPENROUTER_API_KEY\":   \"" + colorSecret + "\"\n}\n"},
	{"two space indentation",
		"{\n  \"b\": 1,\n  \"color\":false,\n  \"a\": 2\n}\n",
		"{\n  \"b\": 1,\n  \"color\":true,\n  \"a\": 2\n}\n",
		"{\n  \"b\": 1,\n  \"color\":false,\n  \"a\": 2\n}\n"},
	{"CRLF line endings",
		"{\r\n\t\"b\": 1,\r\n\t\"color\": false\r\n}\r\n",
		"{\r\n\t\"b\": 1,\r\n\t\"color\": true\r\n}\r\n",
		"{\r\n\t\"b\": 1,\r\n\t\"color\": false\r\n}\r\n"},
	{"compact single line",
		`{"b":1,"color":false,"a":[1,2]}`,
		`{"b":1,"color":true,"a":[1,2]}`,
		`{"b":1,"color":false,"a":[1,2]}`},
	{"no trailing newline",
		"{\n\t\"color\": false\n}",
		"{\n\t\"color\": true\n}",
		"{\n\t\"color\": false\n}"},
	{"api key and spaces around values",
		"  {\n\t\"OPENROUTER_API_KEY\" : \"" + colorSecret + "\" ,\n\t\"color\" :   false  \n}  \n\n",
		"  {\n\t\"OPENROUTER_API_KEY\" : \"" + colorSecret + "\" ,\n\t\"color\" :   true  \n}  \n\n",
		"  {\n\t\"OPENROUTER_API_KEY\" : \"" + colorSecret + "\" ,\n\t\"color\" :   false  \n}  \n\n"},
	{"unknown key with nested object and array",
		"{\n\t\"future\": {\"k\": [1, {\"x\": null}, true, -1.5e3], \"y\": {}},\n\t\"color\": false\n}\n",
		"{\n\t\"future\": {\"k\": [1, {\"x\": null}, true, -1.5e3], \"y\": {}},\n\t\"color\": true\n}\n",
		"{\n\t\"future\": {\"k\": [1, {\"x\": null}, true, -1.5e3], \"y\": {}},\n\t\"color\": false\n}\n"},
	{"color_theme as a nested object",
		"{\n\t\"color_theme\": {  \"foreground\": \"red\",\"background\":\"#00ff00\" },\n\t\"color\": false\n}\n",
		"{\n\t\"color_theme\": {  \"foreground\": \"red\",\"background\":\"#00ff00\" },\n\t\"color\": true\n}\n",
		"{\n\t\"color_theme\": {  \"foreground\": \"red\",\"background\":\"#00ff00\" },\n\t\"color\": false\n}\n"},
	{"nested color key and a string holding a color member",
		"{\n\t\"nested\": {\"color\": false, \"in\": {\"color\": true}},\n\t\"text\": \"\\\"color\\\": false and \\\\\\\"color\\\": false\",\n\t\"raw\": \"\\\"color\\\": false\",\n\t\"color\": false\n}\n",
		"{\n\t\"nested\": {\"color\": false, \"in\": {\"color\": true}},\n\t\"text\": \"\\\"color\\\": false and \\\\\\\"color\\\": false\",\n\t\"raw\": \"\\\"color\\\": false\",\n\t\"color\": true\n}\n",
		"{\n\t\"nested\": {\"color\": false, \"in\": {\"color\": true}},\n\t\"text\": \"\\\"color\\\": false and \\\\\\\"color\\\": false\",\n\t\"raw\": \"\\\"color\\\": false\",\n\t\"color\": false\n}\n"},
	{"unicode escapes and an HTML character in values",
		"{\n\t\"n\": \"caf\\u00e9 \\ud83d\\ude00 <b>&</b>\",\n\t\"color\": false\n}\n",
		"{\n\t\"n\": \"caf\\u00e9 \\ud83d\\ude00 <b>&</b>\",\n\t\"color\": true\n}\n",
		"{\n\t\"n\": \"caf\\u00e9 \\ud83d\\ude00 <b>&</b>\",\n\t\"color\": false\n}\n"},
	{"duplicate color keys, the last is edited",
		"{\n\t\"color\": true,\n\t\"a\": 1,\n\t\"color\": false\n}\n",
		"{\n\t\"color\": true,\n\t\"a\": 1,\n\t\"color\": true\n}\n",
		"{\n\t\"color\": true,\n\t\"a\": 1,\n\t\"color\": false\n}\n"},
	{"duplicate color keys, the last is on",
		"{\"color\":false,\"color\":true}",
		"{\"color\":false,\"color\":true}",
		"{\"color\":false,\"color\":false}"},
	{"key spelled with an escape",
		"{\n\t\"col\\u006fr\": false\n}\n",
		"{\n\t\"col\\u006fr\": true\n}\n",
		"{\n\t\"col\\u006fr\": false\n}\n"},
}

func TestWriteColorEditsOnlyTheValue(t *testing.T) {
	for _, c := range colorCases {
		for _, step := range []struct {
			on   bool
			want string
		}{{true, c.on}, {false, c.off}, {true, c.on}} {
			path := writeColorFixture(t, c.body, 0o600)
			if err := WriteColor(path, step.on); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != step.want {
				t.Errorf("%s on=%v:\nwant %q\n got %q", c.name, step.on, step.want, got)
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0o600 {
				t.Errorf("%s: mode is %v", c.name, info.Mode().Perm())
			}
			if _, err := os.Stat(path + ".new"); err == nil {
				t.Errorf("%s: temporary file left behind", c.name)
			}
		}
	}
}

func TestWriteColorRoundTripRestoresTheOriginalBytes(t *testing.T) {
	for _, c := range colorCases {
		state := c.body == c.on
		path := writeColorFixture(t, c.body, 0o600)
		for _, on := range []bool{!state, state} {
			if err := WriteColor(path, on); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}
		got, _ := os.ReadFile(path)
		if string(got) != c.body {
			t.Errorf("%s: the original was not restored:\nwant %q\n got %q", c.name, c.body, got)
		}
	}
}

// absentCases lack a color key. The result is the file with one member
// inserted, in the file's own style, and the comma on the previous last member.
var absentCases = []struct {
	name, body, want string
}{
	{"multi-line tabs",
		"{\n\t\"OPENROUTER_API_KEY\": \"" + colorSecret + "\",\n\t\"b\": [1, 2]\n}\n",
		"{\n\t\"OPENROUTER_API_KEY\": \"" + colorSecret + "\",\n\t\"b\": [1, 2],\n\t\"color\": true\n}\n"},
	{"multi-line two spaces, no colon space",
		"{\n  \"a\":1\n}",
		"{\n  \"a\":1,\n  \"color\":true\n}"},
	{"multi-line CRLF",
		"{\r\n\t\"a\": 1\r\n}\r\n",
		"{\r\n\t\"a\": 1,\r\n\t\"color\": true\r\n}\r\n"},
	{"multi-line, trailing blank lines",
		"{\n\t\"a\": 1\n}\n\n\n",
		"{\n\t\"a\": 1,\n\t\"color\": true\n}\n\n\n"},
	{"compact without spaces",
		`{"a":1,"b":{"color":false}}`,
		`{"a":1,"b":{"color":false},"color":true}`},
	{"compact with spaces",
		`{"a": 1, "b": "x"}`,
		`{"a": 1, "b": "x", "color": true}`},
	{"compact with padding",
		`{ "a": 1 }`,
		`{ "a": 1, "color": true }`},
	{"color only inside a nested object and a string",
		"{\n\t\"n\": {\"color\": false},\n\t\"s\": \"\\\"color\\\": false\"\n}\n",
		"{\n\t\"n\": {\"color\": false},\n\t\"s\": \"\\\"color\\\": false\",\n\t\"color\": true\n}\n"},
	{"color_theme only",
		"{\n\t\"color_theme\": {\"foreground\": \"red\"}\n}\n",
		"{\n\t\"color_theme\": {\"foreground\": \"red\"},\n\t\"color\": true\n}\n"},
	{"empty object", `{}`, `{"color": true}`},
	{"empty object with newline", "{\n}\n", "{\n\t\"color\": true\n}\n"},
	{"empty object with CRLF", "{\r\n}\r\n", "{\r\n\t\"color\": true\r\n}\r\n"},
}

func TestWriteColorInsertsInTheFilesStyle(t *testing.T) {
	for _, c := range absentCases {
		path := writeColorFixture(t, c.body, 0o600)
		if err := WriteColor(path, true); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != c.want {
			t.Errorf("%s:\nwant %q\n got %q", c.name, c.want, got)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(got, &obj); err != nil || string(obj["color"]) != "true" {
			t.Errorf("%s: the result does not parse with color true: %v", c.name, err)
		}
		// Turning it off afterwards edits the inserted value in place.
		if err := WriteColor(path, false); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		off, _ := os.ReadFile(path)
		if string(off) != strings.Replace(c.want, "true", "false", 1) {
			t.Errorf("%s: off gave %q", c.name, off)
		}
	}
}

func TestWriteColorAlreadyTheRequestedValueChangesNothing(t *testing.T) {
	for _, c := range colorCases {
		path := writeColorFixture(t, c.on, 0o600)
		if err := WriteColor(path, true); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != c.on {
			t.Errorf("%s: the file changed", c.name)
		}
	}
}

func TestWriteColorKeepsTheSecretAndTheLinkTarget(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	body := "{\n\t\"OPENROUTER_API_KEY\": \"" + colorSecret + "\"\n}\n"
	if err := os.WriteFile(real, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symbolic links are not available")
	}
	if err := WriteColor(link, true); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	got, _ := os.ReadFile(real)
	want := "{\n\t\"OPENROUTER_API_KEY\": \"" + colorSecret + "\",\n\t\"color\": true\n}\n"
	if string(got) != want {
		t.Errorf("got %q", got)
	}
}

// fuzzValues are values that carry what a rewrite would disturb.
var fuzzValues = []string{
	`1`, `-0.5e+10`, `true`, `false`, `null`, `"plain"`, `"caf\u00e9 <&>"`,
	`"say \"color\": false"`, `"back\\"`, `"\\\"color\\\": true"`, `[]`, `{}`,
	`[1, 2, {"color": true}]`, `{"color": false, "x": {"color": null}}`,
	`{ "a" : [ "]", "}", "\"" ] }`, `"` + colorSecret + `"`,
}

var fuzzKeys = []string{"a", "OPENROUTER_API_KEY", "color_theme", "colors", "x y", "k\\u0041", "col", "\\\"color\\\""}

// TestWriteColorRandomFiles builds random objects in random layouts and checks
// that only the target bytes change and that the result still parses.
func TestWriteColorRandomFiles(t *testing.T) {
	rng := rand.New(rand.NewSource(20261002))
	pick := func(xs []string) string { return xs[rng.Intn(len(xs))] }
	space := func(newlineOK bool) string {
		opts := []string{"", " ", "  ", "\t"}
		if newlineOK {
			opts = append(opts, "\n", "\n\t", "\n  ", "\r\n", "\r\n\t", " \n ")
		}
		return pick(opts)
	}
	for n := 0; n < 3000; n++ {
		multi := rng.Intn(2) == 0
		var members []string
		count := rng.Intn(6)
		for i := 0; i < count; i++ {
			members = append(members, `"`+pick(fuzzKeys)+`"`+space(false)+":"+space(false)+pick(fuzzValues))
		}
		hasColor := false
		if rng.Intn(3) > 0 {
			// Up to two top-level color keys, anywhere.
			for k := 0; k < 1+rng.Intn(2); k++ {
				m := `"color"` + space(false) + ":" + space(false) + pick([]string{"true", "false"})
				at := rng.Intn(len(members) + 1)
				members = append(members[:at], append([]string{m}, members[at:]...)...)
				hasColor = true
			}
		}
		var b strings.Builder
		b.WriteString(space(false) + "{")
		for i, m := range members {
			b.WriteString(space(multi))
			b.WriteString(m)
			if i < len(members)-1 {
				b.WriteString(space(false) + ",")
			}
		}
		b.WriteString(space(multi) + "}" + space(true))
		body := b.String()
		if !json.Valid([]byte(body)) {
			t.Fatalf("generator made invalid JSON: %q", body)
		}
		var before map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &before)

		for _, on := range []bool{true, false} {
			path := writeColorFixture(t, body, 0o600)
			if err := WriteColor(path, on); err != nil {
				t.Fatalf("%q: %v", body, err)
			}
			gotb, _ := os.ReadFile(path)
			got := string(gotb)
			var after map[string]json.RawMessage
			if err := json.Unmarshal(gotb, &after); err != nil {
				t.Fatalf("%q gave an unparsable %q", body, got)
			}
			want := "false"
			if on {
				want = "true"
			}
			if string(after["color"]) != want {
				t.Fatalf("%q on=%v gave %q", body, on, got)
			}
			for k, v := range before {
				if k != "color" && string(after[k]) != string(v) {
					t.Fatalf("%q: key %q changed in %q", body, k, got)
				}
			}
			if hasColor {
				if got == body {
					if string(before["color"]) != want {
						t.Fatalf("%q on=%v: nothing changed", body, on)
					}
					continue
				}
				// Exactly one true or false token differs, and nothing else.
				p := 0
				for p < len(body) && p < len(got) && body[p] == got[p] {
					p++
				}
				ok := false
				for w := max(0, p-5); w <= p && !ok; w++ {
					for _, tok := range []string{"true", "false"} {
						if strings.HasPrefix(body[w:], tok) && got[:w] == body[:w] &&
							got[w:] == want+body[w+len(tok):] {
							ok = true
						}
					}
				}
				if !ok {
					t.Fatalf("%q on=%v: more than the value changed in %q", body, on, got)
				}
				continue
			}
			if count == 0 {
				// The whitespace inside an empty object is the one thing that
				// is laid out again, since it holds no member to copy from.
				if strings.Join(strings.Fields(got), "") != `{"color":`+want+`}` {
					t.Fatalf("%q: gave %q", body, got)
				}
				continue
			}
			// One member was inserted and nothing else changed.
			add := len(got) - len(body)
			ok := false
			for q := 0; q+add <= len(got) && !ok; q++ {
				if add > 0 && got[:q]+got[q+add:] == body && strings.Contains(got[q:q+add], `"color"`) {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("%q: not a single insertion in %q", body, got)
			}
		}
	}
}
