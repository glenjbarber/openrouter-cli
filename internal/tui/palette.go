package tui

import (
	"fmt"
	"strconv"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// This file turns a theme into the sequences the screen writes. It is pure:
// every function takes values and returns strings, and nothing here draws.
//
// Only these forms are ever produced: 30 to 37 and 90 to 97 for the sixteen
// named foregrounds, and 38;5;n and 48;5;n for everything else. A 24 bit
// sequence (38;2 or 48;2) is never produced, since Terminal.app supports 256
// colors and does not support 24 bit color. A #rrggbb value is mapped to the
// nearest of the 256 instead.
//
// The sixteen names are written as the ANSI colors, and not as fixed values,
// so that the terminal theme decides what they look like. With no theme set, no
// base color is written at all and the terminal theme shows through.

// sgrReset clears every attribute.
const sgrReset = "\x1b[0m"

// sgrFaint is the faint attribute, offered only as an alternative to the bright
// black dim. Whether Terminal.app draws faint text is unverified, which is why
// the default dim is 90 and faint is opt in through withFaintDim.
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

// roleSequences holds one sequence for each role. They are the sixteen ANSI
// foregrounds, so the terminal theme decides how they look.
var roleSequences = [roleCount]string{
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

// palette is a resolved theme: the base colors and the sequence of each role.
type palette struct {
	// fg and bg are the base sequences, empty when the terminal theme is
	// followed. They are written once and re-applied after every reset.
	fg, bg string
	roles  [roleCount]string
}

// newPalette resolves a theme.
//
// A side that is empty, or that holds a value that cannot be resolved, is left
// to the terminal theme. The loader has already dropped values that are not
// valid, so the second case is only a guard for a caller that did not come
// through it.
func newPalette(theme config.Theme) palette {
	p := palette{roles: roleSequences}
	if seq, err := colorSequence(theme.Foreground, false); err == nil {
		p.fg = seq
	}
	if seq, err := colorSequence(theme.Background, true); err == nil {
		p.bg = seq
	}
	return p
}

// withFaintDim returns the palette with the dim role drawn as the faint
// attribute in place of bright black. Faint support in Terminal.app is
// unverified, so this is an alternative to be tried rather than the default.
func (p palette) withFaintDim() palette {
	p.roles[roleDim] = sgrFaint
	return p
}

// base returns the sequences that set the base colors, empty when the
// terminal theme is followed on both sides.
func (p palette) base() string { return p.fg + p.bg }

// reset returns the sequence that ends a colored run. It clears every
// attribute and then sets the base colors again, since a bare reset would also
// clear the base and leave the rest of the row in the terminal theme.
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
// A name below eight is written as 30 to 37 and a bright name as 90 to 97 for a
// foreground. A background is written as 48;5;n for every form, since the
// background forms 40 to 47 and 100 to 107 are not used. An index is written as
// 38;5;n or 48;5;n. A #rrggbb value is mapped to the nearest 256 color index.
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
