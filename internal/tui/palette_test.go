package tui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// allowedSequence matches the only forms the palette may produce: a named
// foreground, or a 256 color foreground or background. Faint is checked on its
// own, since it is an opt in alternative.
var allowedSequence = regexp.MustCompile(`^\x1b\[(3[0-7]|9[0-7]|[34]8;5;(\d{1,3}))m$`)

func TestColorSequenceForeground(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"black", "\x1b[30m"},
		{"red", "\x1b[31m"},
		{"green", "\x1b[32m"},
		{"yellow", "\x1b[33m"},
		{"blue", "\x1b[34m"},
		{"magenta", "\x1b[35m"},
		{"cyan", "\x1b[36m"},
		{"white", "\x1b[37m"},
		{"bright_black", "\x1b[90m"},
		{"bright_red", "\x1b[91m"},
		{"bright_green", "\x1b[92m"},
		{"bright_yellow", "\x1b[93m"},
		{"bright_blue", "\x1b[94m"},
		{"bright_magenta", "\x1b[95m"},
		{"bright_cyan", "\x1b[96m"},
		{"bright_white", "\x1b[97m"},
		{"Bright_Red", "\x1b[91m"},
		{"0", "\x1b[38;5;0m"},
		{"9", "\x1b[38;5;9m"},
		{"16", "\x1b[38;5;16m"},
		{"250", "\x1b[38;5;250m"},
		{"255", "\x1b[38;5;255m"},
		{"007", "\x1b[38;5;7m"},
		{"#000000", "\x1b[38;5;16m"},
		{"#ffffff", "\x1b[38;5;231m"},
		{"#FF0000", "\x1b[38;5;196m"},
	}
	for _, c := range cases {
		got, err := colorSequence(c.in, false)
		if err != nil || got != c.want {
			t.Errorf("colorSequence(%q, fg) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

// A background is always the 256 color form, so a named background is its
// index from 0 to 15.
func TestColorSequenceBackground(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"black", "\x1b[48;5;0m"},
		{"red", "\x1b[48;5;1m"},
		{"white", "\x1b[48;5;7m"},
		{"bright_black", "\x1b[48;5;8m"},
		{"bright_white", "\x1b[48;5;15m"},
		{"17", "\x1b[48;5;17m"},
		{"#121212", "\x1b[48;5;233m"},
		{"#ffd700", "\x1b[48;5;220m"},
	}
	for _, c := range cases {
		got, err := colorSequence(c.in, true)
		if err != nil || got != c.want {
			t.Errorf("colorSequence(%q, bg) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestColorSequenceRejects(t *testing.T) {
	for _, in := range []string{"", "  ", "orange", "256", "-1", "#fff", "#12345g", "ff0000", "bright red"} {
		for _, bg := range []bool{false, true} {
			if got, err := colorSequence(in, bg); err == nil || got != "" {
				t.Errorf("colorSequence(%q, %v) = %q, %v; want an error and nothing", in, bg, got, err)
			}
		}
	}
}

func TestNearest256(t *testing.T) {
	cases := []struct {
		name string
		in   rgb
		want int
	}{
		{"black is the cube origin, not the floor", rgb{0, 0, 0}, 16},
		{"white", rgb{255, 255, 255}, 231},
		{"red", rgb{255, 0, 0}, 196},
		{"green", rgb{0, 255, 0}, 46},
		{"blue", rgb{0, 0, 255}, 21},
		{"yellow", rgb{255, 255, 0}, 226},
		{"gold", rgb{255, 215, 0}, 220},
		{"cube blue-grey", rgb{95, 135, 175}, 67},
		{"mid grey is on the ramp", rgb{128, 128, 128}, 244},
		{"near black grey", rgb{18, 18, 18}, 233},
		{"light grey on the ramp", rgb{238, 238, 238}, 255},
		{"grey 118 on the ramp", rgb{118, 118, 118}, 243},
		{"dark color keeps zero components", rgb{0, 0, 95}, 17},
		{"component below the old floor stays low", rgb{0, 10, 0}, 16},
		{"out of range is held", rgb{300, -5, 0}, 196},
	}
	for _, c := range cases {
		if got := nearest256(c.in); got != c.want {
			t.Errorf("%s: nearest256(%v) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

func TestNearestCubeLevel(t *testing.T) {
	cases := map[int]int{
		0: 0, 47: 0, 48: 1, 95: 1, 115: 1, 116: 2, 135: 2, 155: 2, 156: 3,
		175: 3, 195: 3, 196: 4, 215: 4, 235: 4, 236: 5, 255: 5,
	}
	for v, want := range cases {
		if got := nearestCubeLevel(v); got != want {
			t.Errorf("nearestCubeLevel(%d) = %d, want %d", v, got, want)
		}
	}
}

// The result is checked against a search of every candidate, over a spread of
// colors, so the shortcut through the cube levels cannot disagree with the
// definition of nearest.
func TestNearest256AgreesWithABruteForceSearch(t *testing.T) {
	brute := func(c rgb) int {
		best := colorDistance(c, rgb{paletteCubeLevels[0], paletteCubeLevels[0], paletteCubeLevels[0]})
		for r := 0; r < 6; r++ {
			for g := 0; g < 6; g++ {
				for b := 0; b < 6; b++ {
					d := colorDistance(c, rgb{paletteCubeLevels[r], paletteCubeLevels[g], paletteCubeLevels[b]})
					if d < best {
						best = d
					}
				}
			}
		}
		for i := 0; i < paletteGrayCount; i++ {
			v := 8 + 10*i
			if d := colorDistance(c, rgb{v, v, v}); d < best {
				best = d
			}
		}
		return best
	}
	// The index is turned back into a color so that its distance can be
	// compared with the best one found.
	colorOf := func(n int) rgb {
		if n >= paletteGrayFirst {
			v := 8 + 10*(n-paletteGrayFirst)
			return rgb{v, v, v}
		}
		n -= 16
		return rgb{paletteCubeLevels[n/36], paletteCubeLevels[(n/6)%6], paletteCubeLevels[n%6]}
	}
	for r := 0; r <= 255; r += 5 {
		for g := 0; g <= 255; g += 5 {
			for b := 0; b <= 255; b += 5 {
				c := rgb{r, g, b}
				n := nearest256(c)
				if n < 16 || n > 255 {
					t.Fatalf("nearest256(%v) = %d, outside the cube and the ramp", c, n)
				}
				if got, want := colorDistance(c, colorOf(n)), brute(c); got != want {
					t.Fatalf("nearest256(%v) = %d at distance %d, best is %d", c, n, got, want)
				}
			}
		}
	}
}

// No theme means no base color at all, so the terminal theme shows through.
func TestNoThemeWritesNoBase(t *testing.T) {
	p := newPalette(config.Theme{})
	if p.fg != "" || p.bg != "" || p.base() != "" {
		t.Errorf("fg %q bg %q base %q, want none", p.fg, p.bg, p.base())
	}
	if p.reset() != "\x1b[0m" {
		t.Errorf("reset = %q, want a bare reset", p.reset())
	}
}

func TestThemeSetsTheBase(t *testing.T) {
	cases := []struct {
		name  string
		theme config.Theme
		base  string
		reset string
	}{
		{"foreground only", config.Theme{Foreground: "white"},
			"\x1b[37m", "\x1b[0m\x1b[37m"},
		{"background only", config.Theme{Background: "black"},
			"\x1b[48;5;0m", "\x1b[0m\x1b[48;5;0m"},
		{"both", config.Theme{Foreground: "#ffffff", Background: "17"},
			"\x1b[38;5;231m\x1b[48;5;17m", "\x1b[0m\x1b[38;5;231m\x1b[48;5;17m"},
		{"hex nearest", config.Theme{Foreground: "#808080", Background: "#121212"},
			"\x1b[38;5;244m\x1b[48;5;233m", "\x1b[0m\x1b[38;5;244m\x1b[48;5;233m"},
	}
	for _, c := range cases {
		p := newPalette(c.theme)
		if p.base() != c.base {
			t.Errorf("%s: base = %q, want %q", c.name, p.base(), c.base)
		}
		if p.reset() != c.reset {
			t.Errorf("%s: reset = %q, want %q", c.name, p.reset(), c.reset)
		}
	}
}

// A value that cannot be resolved leaves that side to the terminal theme and
// keeps the other.
func TestAnUnresolvableThemeValueFallsBack(t *testing.T) {
	p := newPalette(config.Theme{Foreground: "orange", Background: "blue"})
	if p.fg != "" {
		t.Errorf("fg = %q, want none for an invalid value", p.fg)
	}
	if p.bg != "\x1b[48;5;4m" {
		t.Errorf("bg = %q, want the valid side kept", p.bg)
	}
}

// Color is never 24 bit, and every sequence is one of the allowed forms, for
// every role and for every theme value form.
func TestOnlyAllowedSequencesAreProduced(t *testing.T) {
	check := func(label, seq string) {
		t.Helper()
		if strings.Contains(seq, "38;2") || strings.Contains(seq, "48;2") {
			t.Errorf("%s: %q is a 24 bit sequence", label, seq)
		}
		m := allowedSequence.FindStringSubmatch(seq)
		if m == nil {
			t.Errorf("%s: %q is not an allowed form", label, seq)
			return
		}
		if m[2] != "" {
			if n, _ := strconv.Atoi(m[2]); n > 255 {
				t.Errorf("%s: %q has an index above 255", label, seq)
			}
		}
	}
	p := newPalette(config.Theme{})
	for r := role(0); r < roleCount; r++ {
		check("role", p.role(r))
	}
	for _, v := range []string{"red", "bright_cyan", "0", "255", "#123456", "#abcdef", "#000000", "#ffffff"} {
		for _, bg := range []bool{false, true} {
			seq, err := colorSequence(v, bg)
			if err != nil {
				t.Errorf("colorSequence(%q): %v", v, err)
				continue
			}
			check(v, seq)
		}
	}
}

func TestEveryRoleHasASequence(t *testing.T) {
	p := newPalette(config.Theme{})
	for r := role(0); r < roleCount; r++ {
		if p.role(r) == "" {
			t.Errorf("role %d has no sequence", r)
		}
	}
	if p.role(-1) != "" || p.role(roleCount) != "" {
		t.Error("a value that is not a role returned a sequence")
	}
}

// The roles the later phases depend on are all defined, by name.
func TestTheNamedRolesAreDefined(t *testing.T) {
	p := newPalette(config.Theme{})
	named := map[string]role{
		"chrome": roleChrome, "title": roleTitle, "notice": roleNotice,
		"failure": roleFailure, "success": roleSuccess, "approval": roleApproval,
		"dim": roleDim, "code": roleCode, "heading": roleHeading,
		"emphasis": roleEmphasis, "quote": roleQuote, "list": roleList,
		"link": roleLink, "filesystem": roleFilesystem, "git": roleGit,
		"shell": roleShell,
	}
	if len(named) != int(roleCount) {
		t.Errorf("%d roles are named here and %d are defined", len(named), roleCount)
	}
	for name, r := range named {
		if p.role(r) == "" {
			t.Errorf("%s has no sequence", name)
		}
	}
}

// Dim is bright black by default, and failure and approval are never mistaken
// for it or for each other.
func TestDimAndTheStrongRolesAreDistinct(t *testing.T) {
	p := newPalette(config.Theme{})
	if p.role(roleDim) != "\x1b[90m" {
		t.Errorf("dim = %q, want bright black", p.role(roleDim))
	}
	strong := []role{roleFailure, roleApproval, roleSuccess}
	for i, a := range strong {
		if p.role(a) == p.role(roleDim) {
			t.Errorf("role %d is the same as dim", a)
		}
		for _, b := range strong[i+1:] {
			if p.role(a) == p.role(b) {
				t.Errorf("roles %d and %d share %q", a, b, p.role(a))
			}
		}
	}
}

// The three tool identities differ from each other and from the roles a tool
// line may also carry.
func TestToolIdentitiesAreDistinct(t *testing.T) {
	p := newPalette(config.Theme{})
	tools := []role{roleFilesystem, roleGit, roleShell}
	for i, a := range tools {
		for _, b := range tools[i+1:] {
			if p.role(a) == p.role(b) {
				t.Errorf("tool roles %d and %d share %q", a, b, p.role(a))
			}
		}
		for _, other := range []role{roleDim, roleFailure, roleApproval} {
			if p.role(a) == p.role(other) {
				t.Errorf("tool role %d shares a color with role %d", a, other)
			}
		}
	}
}

// The roles do not depend on the theme: the theme sets the base only.
func TestThemeDoesNotChangeTheRoles(t *testing.T) {
	a := newPalette(config.Theme{})
	b := newPalette(config.Theme{Foreground: "#102030", Background: "white"})
	if a.roles != b.roles {
		t.Error("a theme changed the role sequences")
	}
}

func TestFaintDimIsAnAlternative(t *testing.T) {
	p := newPalette(config.Theme{})
	f := p.withFaintDim()
	if f.role(roleDim) != "\x1b[2m" {
		t.Errorf("faint dim = %q, want the faint attribute", f.role(roleDim))
	}
	if p.role(roleDim) != "\x1b[90m" {
		t.Error("withFaintDim changed the palette it was called on")
	}
	for r := role(0); r < roleCount; r++ {
		if r != roleDim && f.role(r) != p.role(r) {
			t.Errorf("role %d changed with faint dim", r)
		}
	}
}

func TestPaint(t *testing.T) {
	p := newPalette(config.Theme{})
	if got, want := p.paint(roleFailure, "no"), "\x1b[31mno\x1b[0m"; got != want {
		t.Errorf("paint = %q, want %q", got, want)
	}
	if got := p.paint(roleFailure, ""); got != "" {
		t.Errorf("paint of nothing = %q, want nothing", got)
	}
	// With a theme the reset puts the base back, so the rest of the row is not
	// left in the terminal theme.
	q := newPalette(config.Theme{Foreground: "white", Background: "17"})
	if got, want := q.paint(roleNotice, "x"), "\x1b[33mx\x1b[0m\x1b[37m\x1b[48;5;17m"; got != want {
		t.Errorf("paint with a theme = %q, want %q", got, want)
	}
}

// Stripping the sequences leaves the text exactly, which is the property the
// copy-out rule rests on.
func TestPaintedTextIsTheTextPlusSequences(t *testing.T) {
	p := newPalette(config.Theme{Foreground: "#445566", Background: "bright_black"})
	text := "a reply with spaces, and 38;2 in it"
	got := p.paint(roleEmphasis, text)
	strip := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if strip.ReplaceAllString(got, "") != text {
		t.Errorf("stripping %q did not leave %q", got, text)
	}
}
