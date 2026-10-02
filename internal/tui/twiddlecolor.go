package tui

import (
	"math"
	"strconv"
	"time"
)

// The twiddle row is the one part of the frame that is drawn in color of its
// own, and it scrolls through the primaries while the work it reports is in
// progress.
//
// Every other row is prose. A reply is written by a model and copied out by a
// reader, and a sequence written into prose is copied out with it and acts on
// whatever it is pasted into. The twiddle row is an indicator rather than
// prose, and an indicator that never changes is the one thing on the screen a
// reader learns to look past.
//
// The ramp runs through the six primaries rather than through a set of chosen
// colors, since the primaries are the ones every terminal can be relied upon
// to show and the ones a reader names without having to be told them.

// rgb is a color as three components, each from 0 to 255.
type rgb struct{ r, g, b int }

// primaryColors is the ramp the twiddle scrolls through, in order.
//
// The order runs warm to cool and back again, so the ramp arrives home rather
// than jumping. Red leads rather than follows, since it is the primary a
// reader already reads as attention, and a turn opening in it says that
// something is happening before the word beside it has been read.
var primaryColors = []rgb{
	{255, 0, 0},   // red
	{255, 255, 0}, // yellow
	{0, 255, 0},   // green
	{0, 255, 255}, // cyan
	{0, 0, 255},   // blue
	{255, 0, 255}, // magenta
}

// twiddleCircuit is how long the ramp takes to come all the way round.
//
// The figure is chosen so that the color drifts rather than flashes. A circuit
// timed to the figure itself would come round every nine hundred milliseconds,
// which is faster than the eye resolves and reads as a flicker rather than as a
// scroll. Four seconds is long enough that the movement is seen as movement and
// short enough that a turn of a minute goes round it several times.
const twiddleCircuit = 4 * time.Second

// twiddleSteps is how many steps one circuit is divided into.
//
// The step is one repaint of the figure, so the ramp is expressed in the units
// the twiddle is actually drawn in and moves at the same rate whatever the
// interval is changed to. It is held at one or more, since a circuit shorter
// than a single repaint would leave nothing to divide by.
var twiddleSteps = maxInt(1, int(twiddleCircuit/spinnerInterval))

// twiddleFloor is the lowest value a component of the ramp may take.
//
// A primary drawn with one of its components at zero is dim rather than
// colored on a dark background, and a dark background is the common case. The
// floor is the second level of the cube, which every primary sits above while
// staying recognisably the primary it is: a blue at the floor is still blue
// rather than the black that red at zero would be.
const twiddleFloor = 95

// cubeLevels are the six values each component of the 256 color cube takes.
var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// twiddleTint returns the sequence that draws one step of the ramp.
//
// The step is taken modulo the length of the circuit, so a turn long enough to
// go round it many times repeats the ramp exactly rather than running off the
// end of it. The modulo is made positive on the way, since the result indexes
// a table and a step outside it in either direction would read past the ends.
//
// The color is interpolated between the two primaries either side of the step
// rather than drawn as the nearest one. A ramp that went red, yellow, green,
// cyan, blue, magenta and back again would be six colors turning, which is not
// a scroll, and a hard change is the thing a scroll is wanted in place of.
func twiddleTint(step int) string {
	steps := maxInt(1, twiddleSteps)
	step = ((step % steps) + steps) % steps

	// The position along the ramp is taken in primaries rather than in
	// components, since one primary is one segment of it and the primaries
	// are not evenly spaced in any one channel.
	at := float64(step) * float64(len(primaryColors)) / float64(steps)
	i := int(at) % len(primaryColors)
	frac := at - float64(int(at))
	here, next := primaryColors[i], primaryColors[(i+1)%len(primaryColors)]

	return cubeSequence(rgb{
		r: lerp(here.r, next.r, frac),
		g: lerp(here.g, next.g, frac),
		b: lerp(here.b, next.b, frac),
	})
}

// lerp returns the point frac of the way from a to b.
//
// The result is rounded rather than dropped, so that a step at the far end of
// a segment lands on the primary it is heading for rather than short of it.
func lerp(a, b int, frac float64) int {
	return int(math.Round(float64(a) + (float64(b)-float64(a))*frac))
}

// cubeSequence returns the sequence that sets the foreground to a color taken
// from the 256 color cube.
//
// The cube rather than a direct 24 bit color, since nothing requires a
// terminal to understand one and a terminal that does not is at the mercy of
// whatever it does instead. The six by six by six cube is understood everywhere
// the client runs, so a color taken from it is a color that is shown rather
// than one that is refused or approximated.
func cubeSequence(c rgb) string {
	return "\x1b[38;5;" +
		strconv.Itoa(16+36*cubeLevel(c.r)+6*cubeLevel(c.g)+cubeLevel(c.b)) +
		"m"
}

// cubeLevel returns the index of the cube level a component is drawn at.
//
// The component is floored first, so that a primary reaching zero on its way
// down the ramp is drawn at the floor rather than at the bottom of the cube.
// The nearest level is taken rather than the one below it, so that a color at
// the top of the range stays at the top of it.
func cubeLevel(v int) int {
	if v < twiddleFloor {
		v = twiddleFloor
	}
	best, gap := 0, absInt(cubeLevels[0]-v)
	for i := 1; i < len(cubeLevels); i++ {
		if d := absInt(cubeLevels[i] - v); d < gap {
			best, gap = i, d
		}
	}
	return best
}

// absInt returns the magnitude of v.
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
