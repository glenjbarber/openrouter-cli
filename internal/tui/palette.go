package tui

import (
	"fmt"
	"math"
	"strconv"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// This file turns a theme into the sequences the screen writes. It is pure:
// every function takes values and returns strings, and nothing here draws.
//
// Only these forms are ever produced: a 256 color foreground or background,
// 38;5;n or 48;5;n. A 24 bit sequence (38;2 or 48;2) is never produced, since
// Terminal.app supports 256 colors and does not support 24 bit color. A #rrggbb
// value in a theme is mapped to the nearest of the 256.
//
// The default roles are fixed values rather than the ANSI color numbers, since a
// color number leaves what the reader sees to the terminal theme and the roles
// were asked to be brighter than that theme usually draws them. The two tables
// below are chosen by the background the theme names, so that a reader on a
// light terminal gets the table that reads against a light ground.
//
// A role and a theme base are both written as 38;5;n and 48;5;n, so a reader of
// the output cannot tell one from the other by its form. The base is the
// background one, and a foreground is a role, which is what a test of the base
// checks.

// sgrReset clears every attribute.
const sgrReset = "\x1b[0m"

// sgrFaint is the faint attribute, offered only as an alternative to the dim
// grey. Whether Terminal.app draws faint text is unverified, which is why the
// default dim is a grey and faint is opt in through withFaintDim.
const sgrFaint = "\x1b[2m"

// role names what a piece of text is, and so which color it is drawn in. The
// screen asks for a role and never for a color, so the colors are decided in
// one place.
type role int

const (
	roleChrome role = iota
	roleTitle
	roleNotice
	roleFailure
	roleSuccess
	roleApproval
	roleDim
	roleCode
	roleHeading
	roleEmphasis
	roleQuote
	roleList
	roleLink
	// The three tool identities. A tool line is drawn in the color of the
	// kind of tool it is, so a reader can tell a file read from a command
	// without reading the name. The colors are distinct from each other and
	// from the failure and approval colors, which must never be mistaken for
	// an identity.
	roleFilesystem
	roleGit
	roleShell

	roleCount
)

// roleSequences holds one sequence for each role on a dark terminal, which is
// the default and the common case.
//
// Every color is a fixed value rather than an ANSI color number, since a number
// leaves what the reader sees to the terminal theme.
//
// Every role is brighter than the color number it replaced, which is what the
// request for brighter defaults rests on. Three of them are pale rather than
// saturated: the notice, the emphasis and the shell each replaced a yellow that
// was already a bright one, and the cube has nothing brighter in that family
// that is not a shade of cream. A role that was already at the top of the scale
// cannot be brightened without losing its hue, and brightness was asked for.
// The hue is held as far as the cube allows, so the notice is amber, the
// emphasis is yellow and the shell is cream.
var roleSequences = [roleCount]string{
	roleChrome:     cubeSequence(rgb{95, 175, 255}),
	roleTitle:      cubeSequence(rgb{255, 255, 255}),
	roleNotice:     cubeSequence(rgb{255, 215, 95}),
	roleFailure:    cubeSequence(rgb{255, 95, 95}),
	roleSuccess:    cubeSequence(rgb{135, 255, 135}),
	roleApproval:   cubeSequence(rgb{255, 95, 135}),
	roleDim:        greySequence(244),
	roleCode:       cubeSequence(rgb{175, 255, 255}),
	roleHeading:    cubeSequence(rgb{255, 95, 255}),
	roleEmphasis:   cubeSequence(rgb{255, 255, 95}),
	roleQuote:      cubeSequence(rgb{215, 135, 255}),
	roleList:       cubeSequence(rgb{135, 175, 255}),
	roleLink:       cubeSequence(rgb{95, 255, 135}),
	roleFilesystem: cubeSequence(rgb{95, 255, 215}),
	roleGit:        cubeSequence(rgb{255, 135, 255}),
	roleShell:      cubeSequence(rgb{255, 255, 175}),
}

// roleSequencesLight holds one sequence for each role on a light terminal,
// which is the table the roles carried before they were brightened.
//
// A color bright enough to read on a dark ground washes out on a light one, so a
// reader on a light terminal needs the darker values back. The table is selected
// by the background the theme names, and a reader whose terminal is light sets
// color_theme.background to say so.
var roleSequencesLight = [roleCount]string{
	roleChrome:     "\x1b[34m",
	roleTitle:      "\x1b[96m",
	roleNotice:     "\x1b[33m",
	roleFailure:    "\x1b[31m",
	roleSuccess:    "\x1b[32m",
	roleApproval:   "\x1b[91m",
	roleDim:        "\x1b[90m",
	roleCode:       "\x1b[36m",
	roleHeading:    "\x1b[95m",
	roleEmphasis:   "\x1b[93m",
	roleQuote:      "\x1b[35m",
	roleList:       "\x1b[94m",
	roleLink:       "\x1b[92m",
	roleFilesystem: "\x1b[94m",
	roleGit:        "\x1b[95m",
	roleShell:      "\x1b[93m",
}

// greySequence returns the sequence that sets a foreground to a grey from the
// ramp that follows the cube.
//
// The ramp rather than the cube, since a grey is a grey at every level of the
// cube and the cube would have to spend three channels on it. The value is an
// index into the 256 color palette, since that is what a role is written as
// elsewhere here, and an index outside the ramp is held to the ends of it.
func greySequence(index int) string {
	level := minInt(maxInt(index-paletteGrayFirst, 0), paletteGrayCount-1)
	return "\x1b[38;5;" + strconv.Itoa(paletteGrayFirst+level) + "m"
}

// palette is a resolved theme: the base colors and the sequence of each role.
type palette struct {
	// fg and bg are the base sequences, empty when the terminal theme is
	// followed. They are written once and re-applied after every reset.
	fg, bg string
	roles  [roleCount]string
}

// newPalette resolves a theme.
//
// The roles are chosen by the background the theme names, since a color that
// reads on a dark ground does not read on a light one. A background that is
// absent, or that cannot be resolved, is taken to be dark, which is the common
// case and the one the defaults were chosen for.
//
// A side that is empty, or that holds a value that cannot be resolved, is left
// to the terminal theme. The loader has already dropped values that are not
// valid, so that case is only a guard for a caller that did not come through it.
func newPalette(theme config.Theme) palette { return newPaletteOn(theme, groundAuto) }

// newPaletteOn resolves a theme against a ground the reader chose.
//
// It is the form the interface uses, since /theme sets the ground for the
// session and the frame has to honour it. A caller with no choice to honour
// calls newPalette, which is the same with the ground left to follow the theme.
func newPaletteOn(theme config.Theme, chosen ground) palette {
	p := palette{roles: roleSequences}
	if groundIsLight(theme, chosen) {
		p.roles = roleSequencesLight
	}
	if seq, err := colorSequence(theme.Foreground, false); err == nil {
		p.fg = seq
	}
	if seq, err := colorSequence(theme.Background, true); err == nil {
		p.bg = seq
	}
	return p
}

// lightBackgroundLuminance is the luminance at or above which a background is
// taken to be a light one and the roles are darkened to suit it.
//
// The figure is the half of the relative luminance range, which is where a
// ground stops being a dark one to the eye. It is a boundary rather than a
// scale: the two tables are not two ends of a range of brightness but two
// choices, since a half-bright role reads as neither.
const lightBackgroundLuminance = 0.5

// isLightBackground reports whether a theme names a light background.
//
// An absent background is not a light one. The roles default to the table that
// reads on a dark ground, so a reader on a light terminal says so by naming a
// background rather than by leaving the theme alone. Reading it the other way
// would draw the common case for the uncommon one.
func isLightBackground(theme config.Theme) bool {
	c, ok := themeRGB(theme.Background)
	if !ok {
		return false
	}
	return relativeLuminance(c) >= lightBackgroundLuminance
}

// ansiPalette is the value of each of the sixteen ANSI colors, in the order of
// config.ColorNames.
//
// They are spelled out rather than derived, since the terminal decides what an
// ANSI color number looks like and this table is the client guessing at it. It
// is read only to judge a background the theme named as a color, and to say how
// bright a role was before it was brightened, so an approximation is enough:
// what matters is whether a ground is light and whether one role is brighter
// than another, and every terminal that is asked agrees on both.
var ansiPalette = [16]rgb{
	{0, 0, 0},       // black
	{205, 0, 0},     // red
	{0, 205, 0},     // green
	{205, 205, 0},   // yellow
	{0, 0, 238},     // blue
	{205, 0, 205},   // magenta
	{0, 205, 205},   // cyan
	{229, 229, 229}, // white
	{127, 127, 127}, // bright black
	{255, 0, 0},     // bright red
	{0, 255, 0},     // bright green
	{255, 255, 0},   // bright yellow
	{92, 92, 255},   // bright blue
	{255, 0, 255},   // bright magenta
	{0, 255, 255},   // bright cyan
	{255, 255, 255}, // bright white
}

// themeRGB returns the color a theme value names, as three components.
//
// A value is read the way config.NormalizeColor reads it, so a name, an index
// and a hex value cannot be spelled two ways between the two readers. A value
// that is absent or that cannot be resolved reports false rather than a color,
// since a background the client cannot read is not a background it may judge
// the roles against.
func themeRGB(value string) (rgb, bool) {
	canon, ok := config.NormalizeColor(value)
	if !ok {
		return rgb{}, false
	}
	switch {
	case canon[0] == '#':
		r, _ := strconv.ParseUint(canon[1:3], 16, 8)
		g, _ := strconv.ParseUint(canon[3:5], 16, 8)
		b, _ := strconv.ParseUint(canon[5:7], 16, 8)
		return rgb{int(r), int(g), int(b)}, true
	case canon[0] >= '0' && canon[0] <= '9':
		n, err := strconv.Atoi(canon)
		if err != nil || n < 0 || n > 255 {
			return rgb{}, false
		}
		return paletteAt(n), true
	default:
		for i, name := range config.ColorNames {
			if name == canon {
				return ansiPalette[i], true
			}
		}
	}
	return rgb{}, false
}

// paletteAt returns the color at an index of the 256 color palette.
//
// The sixteen indexes below the cube are the terminal theme rather than a fixed
// value, so they are read from the table above. The rest is the cube and the grey
// ramp, which are the same everywhere the client runs.
func paletteAt(n int) rgb {
	switch {
	case n < 0:
		return rgb{}
	case n < 16:
		return ansiPalette[n]
	case n < paletteGrayFirst:
		n -= 16
		return rgb{paletteCubeLevels[n/36], paletteCubeLevels[(n/6)%6], paletteCubeLevels[n%6]}
	default:
		v := 8 + 10*(n-paletteGrayFirst)
		if v > 255 {
			v = 255
		}
		return rgb{v, v, v}
	}
}

// relativeLuminance returns how bright a color is, from zero to one.
//
// The formula is the one for relative luminance rather than an average of the
// channels, since the eye is not equally sensitive to each of them and an
// average calls a saturated blue brighter than a mid grey when it is not. Each
// channel is linearized first, since the components are gamma encoded and the
// curve is part of what the eye is reading.
func relativeLuminance(c rgb) float64 {
	channel := func(v int) float64 {
		f := float64(clamp255(v)) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.r) + 0.7152*channel(c.g) + 0.0722*channel(c.b)
}

// withFaintDim returns the palette with the dim role drawn as the faint
// attribute in place of the dim grey. Faint support in Terminal.app is
// unverified, so this is an alternative to be tried rather than the default.
func (p palette) withFaintDim() palette {
	p.roles[roleDim] = sgrFaint
	return p
}

// base returns the sequences that set the base colors, empty when the terminal
// theme is followed on both sides.
func (p palette) base() string { return p.fg + p.bg }

// reset returns the sequence that ends a colored run. It clears every attribute
// and then sets the base colors again, since a bare reset would also clear the
// base and leave the rest of the row in the terminal theme.
func (p palette) reset() string { return sgrReset + p.base() }

// role returns the sequence that begins a role, empty for a value that is not a
// role.
func (p palette) role(r role) string {
	if r < 0 || r >= roleCount {
		return ""
	}
	return p.roles[r]
}

// paint wraps text in a role and a reset. Empty text stays empty, so nothing is
// written that would color no character.
func (p palette) paint(r role, text string) string {
	if text == "" {
		return ""
	}
	return p.role(r) + text + p.reset()
}

// colorSequence returns the sequence that sets a color from a theme value.
//
// An index is written as 38;5;n or 48;5;n. A name below eight is written as 30
// to 37 and a bright name as 90 to 97 for a foreground, since a name is the
// reader asking the terminal theme to decide. A background is written as
// 48;5;n for every form, since the background forms 40 to 47 and 100 to 107 are
// not used. A #rrggbb value is mapped to the nearest 256 color index.
func colorSequence(value string, background bool) (string, error) {
	canon, ok := config.NormalizeColor(value)
	if !ok {
		return "", fmt.Errorf("%q is not a color name, an index from 0 to 255 or #rrggbb", value)
	}

	n := -1
	switch {
	case canon[0] == '#':
		r, _ := strconv.ParseUint(canon[1:3], 16, 8)
		g, _ := strconv.ParseUint(canon[3:5], 16, 8)
		b, _ := strconv.ParseUint(canon[5:7], 16, 8)
		n = nearest256(rgb{int(r), int(g), int(b)})
	case canon[0] >= '0' && canon[0] <= '9':
		n, _ = strconv.Atoi(canon)
	default:
		for i, name := range config.ColorNames {
			if name != canon {
				continue
			}
			if !background {
				if i < 8 {
					return "\x1b[" + strconv.Itoa(30+i) + "m", nil
				}
				return "\x1b[" + strconv.Itoa(90+i-8) + "m", nil
			}
			n = i
		}
	}
	if n < 0 {
		return "", fmt.Errorf("%q could not be resolved", value)
	}
	lead := "38"
	if background {
		lead = "48"
	}
	return "\x1b[" + lead + ";5;" + strconv.Itoa(n) + "m", nil
}

// paletteCubeLevels are the six values each component of the 256 color cube
// takes, and paletteGrayFirst and paletteGrayCount describe the grey ramp that
// follows it. The cube is indexes 16 to 231 and the ramp is 232 to 255, at 8 +
// 10*i. The sixteen indexes below the cube are left out of the search: what
// they look like is up to the terminal theme, so a fixed value cannot be
// compared against them.
var paletteCubeLevels = [6]int{0, 95, 135, 175, 215, 255}

const (
	paletteGrayFirst = 232
	paletteGrayCount = 24
)

// nearest256 returns the index in the 256 color palette closest to a color,
// searching the cube and the grey ramp by squared distance.
//
// It does not reuse cubeLevel from the twiddle, which floors every component at
// 95 so that a ramp never reaches black. Here a component of zero must stay at
// zero, or #000000 would map to a dark blue and every dark theme value would be
// pushed away from what was written.
//
// On a tie the cube wins over the grey ramp, and the lower level wins within the
// cube, so the result does not depend on the order candidates are tried in.
func nearest256(c rgb) int {
	c = rgb{clamp255(c.r), clamp255(c.g), clamp255(c.b)}

	ri, gi, bi := nearestCubeLevel(c.r), nearestCubeLevel(c.g), nearestCubeLevel(c.b)
	best := 16 + 36*ri + 6*gi + bi
	bestDist := colorDistance(c, rgb{paletteCubeLevels[ri], paletteCubeLevels[gi], paletteCubeLevels[bi]})

	for i := 0; i < paletteGrayCount; i++ {
		v := 8 + 10*i
		if d := colorDistance(c, rgb{v, v, v}); d < bestDist {
			best, bestDist = paletteGrayFirst+i, d
		}
	}
	return best
}

// nearestCubeLevel returns the index of the cube level closest to a component.
// A component exactly between two levels takes the lower.
func nearestCubeLevel(v int) int {
	best, gap := 0, absInt(paletteCubeLevels[0]-v)
	for i := 1; i < len(paletteCubeLevels); i++ {
		if d := absInt(paletteCubeLevels[i] - v); d < gap {
			best, gap = i, d
		}
	}
	return best
}

// colorDistance is the squared distance between two colors in RGB.
func colorDistance(a, b rgb) int {
	dr, dg, db := a.r-b.r, a.g-b.g, a.b-b.b
	return dr*dr + dg*dg + db*db
}

// clamp255 holds a component inside 0 to 255.
func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
