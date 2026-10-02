package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ColorNames are the sixteen color names a theme value may take, in the order
// of their ANSI indexes: the eight normal colors first and the eight bright
// forms after them. The list is written once here and the palette reads it, so
// the names a file may carry and the names the screen understands cannot drift
// apart.
var ColorNames = [16]string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright_black", "bright_red", "bright_green", "bright_yellow",
	"bright_blue", "bright_magenta", "bright_cyan", "bright_white",
}

// Theme is the base colors the file asks for. An empty field means the
// terminal theme is used for that side, so no base color is written for it.
//
// A value is held in the canonical form NormalizeColor returns: a lower case
// name, a decimal index with no leading zeros, or a lower case #rrggbb. A value
// that was not valid has already been dropped, so a field that is set is one
// the palette can resolve.
type Theme struct {
	Foreground string
	Background string
}

// NormalizeColor reports whether a theme value is acceptable, and returns it in
// canonical form.
//
// A value is one of the sixteen names, an index from 0 to 255, or #rrggbb.
// Surrounding space is ignored and case is not significant. An empty value is
// not valid here, since an absent value is handled by the caller.
func NormalizeColor(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", false
	}
	for _, name := range ColorNames {
		if v == name {
			return v, true
		}
	}
	if v[0] == '#' {
		if len(v) != 7 {
			return "", false
		}
		for _, c := range v[1:] {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return "", false
			}
		}
		return v, true
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	n, err := strconv.Atoi(v)
	if err != nil || n > 255 {
		return "", false
	}
	return strconv.Itoa(n), true
}

// parseTheme reads the color_theme value.
//
// A theme is a preference, so nothing in it is fatal. A value that is not
// valid is dropped and named in the note that is returned, and the side it
// belongs to falls back to the terminal theme. The other side is kept, since
// one bad value is no reason to lose a good one. A theme that is not an object
// is dropped whole. The note is a single line, or empty when nothing was wrong.
func parseTheme(raw json.RawMessage) (Theme, string) {
	var theme Theme
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return theme, ""
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return theme, "color_theme is not an object, using the terminal theme"
	}

	var bad []string
	read := func(key string, dst *string) {
		value, present := fields[key]
		if !present {
			return
		}
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			if strings.TrimSpace(string(value)) == "null" {
				return
			}
			bad = append(bad, fmt.Sprintf("%s %s", key, strings.TrimSpace(string(value))))
			return
		}
		if strings.TrimSpace(s) == "" {
			return
		}
		if canon, ok := NormalizeColor(s); ok {
			*dst = canon
			return
		}
		bad = append(bad, fmt.Sprintf("%s %q", key, s))
	}
	read("foreground", &theme.Foreground)
	read("background", &theme.Background)

	if len(bad) == 0 {
		return theme, ""
	}
	return theme, "color_theme: ignored " + strings.Join(bad, " and ") +
		" (use a color name, an index from 0 to 255 or #rrggbb), using the terminal theme"
}
