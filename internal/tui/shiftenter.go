package tui

import "strconv"

// shiftModifier is the value a terminal writes into the modifier parameter of a
// CSI u report for a key held with shift.
//
// The modifier parameter is the sum of the modifier bits, so a key held with
// shift and nothing else carries 2. A terminal that sends a CSI u report with
// any other modifier is not this key, and a report with no modifier at all is
// the key without shift.
const shiftModifier = 2

// csiU is the final byte of a CSI u report.
//
// The form is ESC [ key ; modifier u, and 'u' ends it. No other sequence this
// reader takes ends in 'u', so it is a final of its own rather than a member of
// the ordinary final range.
const csiU = 'u'

// keyEnterCSI is the key number a terminal writes for the enter key in a CSI u
// report.
//
// The number is the C0 control the key carries, which is a carriage return.
// Ctrl-J carries 10 and is a different key, so only this one number is taken as
// a shifted enter.
const keyEnterCSI = 13

// takeShiftedEnter consumes a shift+enter report at the front of buf.
//
// Most terminals send nothing at all for shift+enter, and a client that reads
// neither a report nor a byte has nothing to do for the key, which is the
// honest outcome where the terminal sends nothing. Where a terminal does report
// the key, it writes the CSI u form, and this takes it whole.
//
// The walk is the same shape as takeSequence's, so a form that is not this one
// is left alone rather than being half taken. The up arrow is the shortest
// sequence that begins like this and is not one, so taking it here would
// swallow the key rather than the report.
func takeShiftedEnter(buf []byte) (rest []byte, ok bool) {
	if len(buf) < 3 || buf[0] != keyEscape || buf[1] != '[' {
		return buf, false
	}
	// A mouse report is left alone, so a report arriving in pieces is not
	// taken as a key of its own.
	if buf[2] == '<' || buf[2] == 'M' {
		return buf, false
	}
	for i := 2; i < len(buf); i++ {
		c := buf[i]
		if c == csiU {
			// The parameters are the key number and the modifier. Anything
			// other than this pair is a key this reader does not act on, and
			// is left for takeSequence rather than consumed here.
			key, mod, ok := shiftParams(buf[2:i])
			if !ok || key != keyEnterCSI || mod != shiftModifier {
				return buf, false
			}
			return buf[i+1:], true
		}
		if c >= csiFinalLo && c <= csiFinalHi {
			// An ordinary sequence, which takeSequence handles.
			return buf, false
		}
		if c < 0x20 || c > csiParamHi {
			return buf, false
		}
	}
	// The report has not arrived whole, so it is left for the next read.
	return buf, false
}

// shiftParams reads the key number and the modifier out of a CSI u parameter
// list.
//
// The list is two numbers separated by a semicolon. A single parameter is a key
// with no modifier, which is not a shifted key, so it is refused rather than
// read as one with a modifier of nothing.
func shiftParams(params []byte) (key, mod int, ok bool) {
	first, second, two := cutByte(params, ';')
	if !two {
		return 0, 0, false
	}
	key, err := strconv.Atoi(string(first))
	if err != nil {
		return 0, 0, false
	}
	mod, err = strconv.Atoi(string(second))
	if err != nil {
		return 0, 0, false
	}
	return key, mod, true
}

// cutByte splits b at the first occurrence of sep, reporting whether it was
// found.
//
// It is the byte form of strings.Cut, which the CSI u parameters need because
// they are a slice of the read buffer rather than a string of their own.
func cutByte(b []byte, sep byte) (before, after []byte, found bool) {
	for i, c := range b {
		if c == sep {
			return b[:i], b[i+1:], true
		}
	}
	return b, nil, false
}
