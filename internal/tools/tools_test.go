package tools

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// call runs a tool the way a model reaches one, through the registry, so that
// what a test exercises is the seam a reply is answered through rather than a
// body reached directly.
func call(t *testing.T, s *Set, name, args string) Result {
	t.Helper()
	return s.Run(openrouter.ToolCall{
		Index: 0,
		ID:    "call-1",
		Type:  openrouter.ToolTypeFunction,
		Function: openrouter.ToolCallFunction{
			Name:      name,
			Arguments: args,
		},
	})
}

// mustText returns the text of a call that succeeded, failing the test when it
// did not.
func mustText(t *testing.T, r Result) string {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("the call failed: %v", r.Err)
	}
	return r.Text
}

// mustFail returns the error of a call that was refused, failing the test when
// it was not. A call that produced no result at all is the failure this exists
// to catch, so it is checked rather than assumed.
func mustFail(t *testing.T, r Result) error {
	t.Helper()
	if r.Err == nil {
		t.Fatalf("the call succeeded, returning %q", r.Text)
	}
	return r.Err
}

// stubSet returns a set holding one tool of a chosen body, for the tests about
// the registry rather than about any one tool.
func stubSet(name string, body func(json.RawMessage) (string, error)) *Set {
	s := New()
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name:        name,
			Description: "A tool standing in for one with a body under test.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}, body)
	return s
}

// TestRunCarriesTheCallWhateverTheOutcome checks that the call is echoed into
// the result, since the reply going back to a model names the call it answers.
func TestRunCarriesTheCallWhateverTheOutcome(t *testing.T) {
	s := stubSet("echo", func(raw json.RawMessage) (string, error) {
		return "answered", nil
	})

	for _, name := range []string{"echo", "missing", ""} {
		r := call(t, s, name, "{}")
		if r.Call.Function.Name != name {
			t.Errorf("the result carried the call %q, not %q", r.Call.Function.Name, name)
		}
		if r.Call.ID != "call-1" {
			t.Errorf("the result carried the call id %q, not the one it was given", r.Call.ID)
		}
	}
}

// TestRunOnAnUnknownToolIsAnError checks that a name the set was not offered is
// reported with what was offered, so that a model has something to choose from
// rather than only a mistake.
func TestRunOnAnUnknownToolIsAnError(t *testing.T) {
	s := stubSet("read_file", func(raw json.RawMessage) (string, error) {
		return "", nil
	})

	err := mustFail(t, call(t, s, "write_file", "{}"))
	if !strings.Contains(err.Error(), "write_file") {
		t.Errorf("the refusal did not name the tool asked for: %v", err)
	}
	if !strings.Contains(err.Error(), "read_file") {
		t.Errorf("the refusal did not name what was offered: %v", err)
	}
}

// TestRunOnAnEmptySetNamesTheAbsence checks the message where nothing at all is
// offered, since a refusal listing nothing reads as a set that was not asked.
func TestRunOnAnEmptySetNamesTheAbsence(t *testing.T) {
	err := mustFail(t, call(t, New(), "read_file", "{}"))
	if !strings.Contains(err.Error(), "no tools") {
		t.Errorf("the refusal did not say that no tools are offered: %v", err)
	}
}

// TestRunOnMalformedArgumentsIsAnError checks the three shapes of arguments
// that cannot be decoded into an object, each of which a model produces by
// accident rather than by intent.
func TestRunOnMalformedArgumentsIsAnError(t *testing.T) {
	s := NewFilesystemAtOrSkip(t)

	for _, args := range []string{"", "   ", "{", `["a"]`, `"a string"`, "42"} {
		r := call(t, s, readFileTool, args)
		err := mustFail(t, r)
		if r.Text != "" {
			t.Errorf("arguments %q produced the text %q alongside the error", args, r.Text)
		}
		if !strings.Contains(err.Error(), "arguments") {
			t.Errorf("arguments %q were refused without saying what was wrong: %v",
				args, err)
		}
	}
}

// TestRunReportsAMissingArgumentByName checks that the refusal names the
// argument the schema requires, since a model is the reader of the message and
// the name it has to add is the one it was offered.
func TestRunReportsAMissingArgumentByName(t *testing.T) {
	s := NewFilesystemAtOrSkip(t)

	err := mustFail(t, call(t, s, readFileTool, `{}`))
	if !strings.Contains(err.Error(), `"path"`) {
		t.Errorf("the refusal did not name the argument: %v", err)
	}
}

// TestRunRecoversFromAPanic checks that a body which panics is reported as a
// failure rather than taking the turn down with it.
func TestRunRecoversFromAPanic(t *testing.T) {
	s := stubSet("boom", func(raw json.RawMessage) (string, error) {
		panic("a body with no check in it")
	})

	err := mustFail(t, call(t, s, "boom", "{}"))
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("the refusal did not say the body panicked: %v", err)
	}
	if !strings.Contains(err.Error(), "a body with no check in it") {
		t.Errorf("the refusal did not carry what the body said: %v", err)
	}
}

// TestRunRecoversFromAPanicWithAnErrorValue checks the other shape a panic
// takes, since a body that panics with an error is common where one indexes a
// value it did not check.
func TestRunRecoversFromAPanicWithAnErrorValue(t *testing.T) {
	s := stubSet("boom", func(raw json.RawMessage) (string, error) {
		panic(errors.New("an error rather than a message"))
	})

	err := mustFail(t, call(t, s, "boom", "{}"))
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("the refusal did not say the body panicked: %v", err)
	}
	if !strings.Contains(err.Error(), "an error rather than a message") {
		t.Errorf("the refusal did not carry the error the body panicked with: %v", err)
	}
}

// TestAddIgnoresAToolWithNoNameOrNoBody checks that neither is registered,
// since a call the set cannot run is reported when it arrives rather than being
// answered by something that is not there.
func TestAddIgnoresAToolWithNoNameOrNoBody(t *testing.T) {
	s := New()
	s.Add(openrouter.Tool{Type: openrouter.ToolTypeFunction}, nil)
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name:       "",
			Parameters: json.RawMessage(`{"type":"object"}`),
		},
	}, func(raw json.RawMessage) (string, error) { return "", nil })

	if names := s.Names(); len(names) != 0 {
		t.Errorf("the set holds %v, which cannot be run", names)
	}
}

// TestSpecsAndNamesFollowTheOrderAdded checks the order a model is offered
// tools in, since a catalogue in a different order on each run reads as a
// different catalogue.
func TestSpecsAndNamesFollowTheOrderAdded(t *testing.T) {
	s := New()
	for _, name := range []string{"third", "first", "second"} {
		s.Add(openrouter.Tool{
			Type: openrouter.ToolTypeFunction,
			Function: openrouter.ToolFunction{
				Name:       name,
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		}, func(raw json.RawMessage) (string, error) { return "", nil })
	}
	// A second registration of a name replaces rather than repeats, since it
	// is one tool written twice.
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name:       "first",
			Parameters: json.RawMessage(`{"type":"object"}`),
		},
	}, func(raw json.RawMessage) (string, error) { return "again", nil })

	names := s.Names()
	want := []string{"third", "first", "second"}
	if len(names) != len(want) {
		t.Fatalf("the set holds %v, want %v", names, want)
	}
	for i, name := range want {
		if names[i] != name {
			t.Errorf("name %d is %q, want %q", i, names[i], name)
		}
	}
	specs := s.Specs()
	if len(specs) != len(want) {
		t.Fatalf("the specs hold %d tools, want %d", len(specs), len(want))
	}
	for i, name := range want {
		if specs[i].Function.Name != name {
			t.Errorf("spec %d is %q, want %q", i, specs[i].Function.Name, name)
		}
	}
}

// TestSpecsAreACopy checks that a caller cannot change what the next request
// offers by editing what it was handed.
func TestSpecsAreACopy(t *testing.T) {
	s := NewFilesystemAtOrSkip(t)

	specs := s.Specs()
	specs[0].Function.Name = "edited"
	if again := s.Specs(); again[0].Function.Name == "edited" {
		t.Error("editing the returned specs changed what the set offers")
	}
}

// TestRunAcrossGoroutines checks the registry under the race detector, since a
// turn runs on a goroutine of its own and a command can add a tool while one is
// running.
func TestRunAcrossGoroutines(t *testing.T) {
	s := NewFilesystemAtOrSkip(t)
	outside := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := call(t, s, listDirTool, `{"path":"."}`); r.Err != nil {
				t.Errorf("listing the root: %v", r.Err)
			}
			if r := call(t, s, readFileTool, `{"path":"nothing-here"}`); r.Err == nil {
				t.Errorf("a read of a file that does not exist returned %q", r.Text)
			}
			if r := call(t, s, writeFileTool,
				`{"path":"`+outside+`/written","content":"x"}`); r.Err == nil {
				t.Errorf("a write outside the root was allowed, returning %q", r.Text)
			}
			s.Add(openrouter.Tool{
				Type: openrouter.ToolTypeFunction,
				Function: openrouter.ToolFunction{
					Name:       "added",
					Parameters: json.RawMessage(`{"type":"object"}`),
				},
			}, func(raw json.RawMessage) (string, error) { return "added", nil })
		}()
	}
	wg.Wait()
}

// TestEveryOfferedSchemaIsAJSONObject checks every schema a model is sent, so
// that a bracket left out in a literal is caught here rather than by a model
// receiving a schema it cannot read.
func TestEveryOfferedSchemaIsAJSONObject(t *testing.T) {
	sets := map[string]*Set{
		"filesystem": NewFilesystemAtOrSkip(t),
	}
	if git := NewGitOrSkip(t); git != nil {
		sets["git"] = git
	}

	for name, s := range sets {
		specs := s.Specs()
		if len(specs) == 0 {
			t.Errorf("the %s set offers no tools, so nothing was checked", name)
			continue
		}
		for _, spec := range specs {
			if spec.Type != openrouter.ToolTypeFunction {
				t.Errorf("%s: the type is %q, want %q", spec.Function.Name,
					spec.Type, openrouter.ToolTypeFunction)
			}
			if spec.Function.Description == "" {
				t.Errorf("%s: no description was offered", spec.Function.Name)
			}
			var schema map[string]any
			if err := json.Unmarshal(spec.Function.Parameters, &schema); err != nil {
				t.Errorf("%s: the schema is not a JSON object: %v\n%s",
					spec.Function.Name, err, spec.Function.Parameters)
				continue
			}
			if got := schema["type"]; got != "object" {
				t.Errorf("%s: the schema type is %v, want \"object\"", spec.Function.Name, got)
			}
		}
	}
}
