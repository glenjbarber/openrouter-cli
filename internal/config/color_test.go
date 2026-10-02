package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeColor(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"red", "red", true},
		{"  Bright_Blue ", "bright_blue", true},
		{"black", "black", true},
		{"bright_white", "bright_white", true},
		{"0", "0", true},
		{"7", "7", true},
		{"007", "7", true},
		{"255", "255", true},
		{"256", "", false},
		{"-1", "", false},
		{"1.5", "", false},
		{"#ff8800", "#ff8800", true},
		{"#FF8800", "#ff8800", true},
		{"#fff", "", false},
		{"#ff88000", "", false},
		{"#gg0000", "", false},
		{"ff8800", "", false},
		{"orange", "", false},
		{"bright-red", "", false},
		{"brightred", "", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeColor(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeColor(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEveryColorNameIsAccepted(t *testing.T) {
	for _, name := range ColorNames {
		if got, ok := NormalizeColor(name); !ok || got != name {
			t.Errorf("NormalizeColor(%q) = %q, %v", name, got, ok)
		}
	}
}

func TestParseColorAbsent(t *testing.T) {
	cfg, err := parse(writeConfig(t, `{"OPENROUTER_API_KEY":"k"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Color {
		t.Error("Color = true, want it off when the file does not ask")
	}
	if cfg.ColorTheme != (Theme{}) {
		t.Errorf("ColorTheme = %+v, want none", cfg.ColorTheme)
	}
	if cfg.ColorNote != "" {
		t.Errorf("ColorNote = %q, want none", cfg.ColorNote)
	}
}

func TestParseColorValid(t *testing.T) {
	body := `{"OPENROUTER_API_KEY":"k","color":true,` +
		`"color_theme":{"foreground":"Bright_Cyan","background":"#0A0B0c"}}`
	cfg, err := parse(writeConfig(t, body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Color {
		t.Error("Color = false, want it on")
	}
	want := Theme{Foreground: "bright_cyan", Background: "#0a0b0c"}
	if cfg.ColorTheme != want {
		t.Errorf("ColorTheme = %+v, want %+v", cfg.ColorTheme, want)
	}
	if cfg.ColorNote != "" {
		t.Errorf("ColorNote = %q, want none", cfg.ColorNote)
	}
}

func TestParseColorIndexTheme(t *testing.T) {
	body := `{"OPENROUTER_API_KEY":"k","color_theme":{"foreground":"250","background":"0"}}`
	cfg, err := parse(writeConfig(t, body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := (Theme{Foreground: "250", Background: "0"}); cfg.ColorTheme != want {
		t.Errorf("ColorTheme = %+v, want %+v", cfg.ColorTheme, want)
	}
}

// One side is enough, and the other follows the terminal theme.
func TestParseColorOneSideOnly(t *testing.T) {
	body := `{"OPENROUTER_API_KEY":"k","color_theme":{"foreground":"white"}}`
	cfg, err := parse(writeConfig(t, body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := (Theme{Foreground: "white"}); cfg.ColorTheme != want {
		t.Errorf("ColorTheme = %+v, want %+v", cfg.ColorTheme, want)
	}
	if cfg.ColorNote != "" {
		t.Errorf("ColorNote = %q, want none", cfg.ColorNote)
	}
}

// An invalid value is never fatal. It falls back to the terminal theme for
// that side, keeps the good side, and says so in one line.
func TestParseColorInvalidIsNotFatal(t *testing.T) {
	cases := []struct {
		name     string
		theme    string
		want     Theme
		mentions []string
	}{
		{"bad name", `{"foreground":"orange","background":"blue"}`,
			Theme{Background: "blue"}, []string{"foreground", "orange"}},
		{"index out of range", `{"foreground":"300"}`,
			Theme{}, []string{"foreground", "300"}},
		{"short hex", `{"background":"#fff"}`,
			Theme{}, []string{"background", "#fff"}},
		{"both bad", `{"foreground":"x","background":"y"}`,
			Theme{}, []string{"foreground", "background"}},
		{"number not string", `{"foreground":5}`,
			Theme{}, []string{"foreground", "5"}},
		{"not an object", `"red"`,
			Theme{}, []string{"color_theme"}},
		{"array", `["red"]`,
			Theme{}, []string{"color_theme"}},
	}
	for _, c := range cases {
		body := `{"OPENROUTER_API_KEY":"k","color":true,"color_theme":` + c.theme + `}`
		cfg, err := parse(writeConfig(t, body))
		if err != nil {
			t.Errorf("%s: parse failed: %v", c.name, err)
			continue
		}
		if !cfg.Color {
			t.Errorf("%s: Color = false, want the switch kept", c.name)
		}
		if cfg.ColorTheme != c.want {
			t.Errorf("%s: ColorTheme = %+v, want %+v", c.name, cfg.ColorTheme, c.want)
		}
		if cfg.ColorNote == "" {
			t.Errorf("%s: no note was produced", c.name)
		}
		if strings.Contains(cfg.ColorNote, "\n") {
			t.Errorf("%s: note is more than one line: %q", c.name, cfg.ColorNote)
		}
		for _, m := range c.mentions {
			if !strings.Contains(cfg.ColorNote, m) {
				t.Errorf("%s: note %q does not mention %q", c.name, cfg.ColorNote, m)
			}
		}
	}
}

// Empty and null values are an absent value rather than an invalid one.
func TestParseColorEmptyAndNullAreAbsent(t *testing.T) {
	for _, theme := range []string{`null`, `{}`, `{"foreground":""}`, `{"foreground":null,"background":"  "}`} {
		body := `{"OPENROUTER_API_KEY":"k","color_theme":` + theme + `}`
		cfg, err := parse(writeConfig(t, body))
		if err != nil {
			t.Errorf("%s: parse failed: %v", theme, err)
			continue
		}
		if cfg.ColorTheme != (Theme{}) || cfg.ColorNote != "" {
			t.Errorf("%s: theme %+v note %q, want neither", theme, cfg.ColorTheme, cfg.ColorNote)
		}
	}
}

// The environment style name is not a key. Only the lower case names are read,
// so a file written with the prefixed spelling does not turn color on.
func TestParseColorHasNoEnvironmentStyleKey(t *testing.T) {
	cfg, err := parse(writeConfig(t, `{"OPENROUTER_API_KEY":"k","OPENROUTER_COLOR":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Color {
		t.Error("OPENROUTER_COLOR turned color on")
	}
}

// A file that cannot be read for its key is still read, so the color
// preferences travel with the typed error.
func TestColorSurvivesAMissingKey(t *testing.T) {
	body := `{"color":true,"color_theme":{"foreground":"green","background":"zzz"}}`
	_, err := parse(writeConfig(t, body))
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if !noKey.Color {
		t.Error("Color = false, want it carried with the error")
	}
	if want := (Theme{Foreground: "green"}); noKey.ColorTheme != want {
		t.Errorf("ColorTheme = %+v, want %+v", noKey.ColorTheme, want)
	}
	if !strings.Contains(noKey.ColorNote, "background") {
		t.Errorf("ColorNote = %q, want the dropped value named", noKey.ColorNote)
	}
}

// A skipped setup is returned as a configuration and carries the color
// preferences too.
func TestColorSurvivesASkippedSetup(t *testing.T) {
	cfg, err := parse(writeConfig(t, `{"setup_complete":true,"color":true,"color_theme":{"background":"17"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Skipped || !cfg.Color || cfg.ColorTheme.Background != "17" {
		t.Errorf("Skipped=%v Color=%v theme=%+v", cfg.Skipped, cfg.Color, cfg.ColorTheme)
	}
}

func TestSetColorOnAConfiguration(t *testing.T) {
	cfg := Empty("m", false).SetColor(true, Theme{Foreground: "red"}, "note")
	if !cfg.Color || cfg.ColorTheme.Foreground != "red" || cfg.ColorNote != "note" {
		t.Errorf("SetColor did not carry the values: %+v", cfg)
	}
}

// The default file is unchanged by color: nothing about it is written until
// the reader asks.
func TestDefaultFileCarriesNoColor(t *testing.T) {
	if strings.Contains(DefaultFile, "color") {
		t.Errorf("DefaultFile mentions color: %q", DefaultFile)
	}
}
