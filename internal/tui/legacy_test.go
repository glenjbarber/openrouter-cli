package tui

import "strings"

// This file keeps the fold as it was before commit 8 made the styled twin: the
// functions below are copied from that commit's parent and renamed, with their
// comments dropped. They are the reference the twin is held to, so that a change
// to the text of a row is caught here rather than found on a screen.

func legacyRenderMarkdown(line string, width int) []string {
	if width < 1 {
		width = wrapWidth
	}
	if strings.TrimSpace(line) == "" {
		return []string{""}
	}
	if prefix, text, ok := splitHeading(line); ok {
		return legacyFoldConstruct(prefix, text, width)
	}
	if prefix, text, ok := splitListItem(line); ok {
		return legacyFoldConstruct(prefix, text, width)
	}
	return foldWords(legacyRenderInline(line), width)
}

func legacyFoldConstruct(prefix, text string, width int) []string {
	text = legacyRenderInline(text)

	hang := displayWidth(prefix)
	if hang >= width {
		rows := foldWords(text, width)
		return append([]string{truncate(prefix, width)}, rows...)
	}

	rows := foldWords(text, width-hang)
	indent := strings.Repeat(" ", hang)
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		if i == 0 {
			out = append(out, prefix+row)
			continue
		}
		out = append(out, indent+row)
	}
	return out
}

func legacyRenderInline(s string) string {
	r := []rune(s)
	var b strings.Builder

	var closing []int

	for i := 0; i < len(r); {
		if n := len(closing); n > 0 && i == closing[n-1] {
			i += runLen(r, i, r[i])
			closing = closing[:n-1]
			continue
		}

		c := r[i]
		switch {
		case c == '\\' && i+1 < len(r) && isDelimiter(r[i+1]):
			b.WriteString(string(r[i : i+2]))
			i += 2
		case c == '`':
			n := runLen(r, i, '`')
			if end := findRun(r, i+n, '`', n); end >= 0 {
				b.WriteString(string(r[i : end+n]))
				i = end + n
				continue
			}
			b.WriteRune(c)
			i++
		case isDelimiter(c):
			n := runLen(r, i, c)
			if n > maxDelimRun || !opensSpan(r, i, n, c) {
				b.WriteString(string(r[i : i+n]))
				i += n
				continue
			}
			end := findRun(r, i+n, c, n)
			if end < 0 || !closesSpan(r, end, n, c) {
				b.WriteString(string(r[i : i+n]))
				i += n
				continue
			}
			closing = append(closing, end)
			i += n
		default:
			b.WriteRune(c)
			i++
		}
	}
	return b.String()
}

func legacyWrapBlock(s string, width int) []string {
	if width < 1 {
		width = wrapWidth
	}

	var out []string
	var block []string
	inFence := false

	flush := func() {
		for _, l := range block {
			if strings.TrimSpace(l) == "" {
				out = append(out, "")
				continue
			}
			out = append(out, l)
		}
		block = nil
	}

	for _, line := range strings.Split(s, "\n") {
		before, marker, after := splitFence(line)
		if marker != "" {
			before = strings.TrimRight(before, " \t")
			if before != "" {
				if inFence {
					block = append(block, before)
				} else {
					out = append(out, legacyRenderMarkdown(before, width)...)
				}
			}
			flush()
			inFence = !inFence
			out = append(out, marker)
			if after != "" {
				if inFence {
					block = append(block, strings.TrimRight(after, " \t"))
				} else {
					out = append(out, legacyRenderMarkdown(after, width)...)
				}
			}
			continue
		}

		if inFence {
			block = append(block, line)
			continue
		}
		out = append(out, legacyRenderMarkdown(line, width)...)
	}

	flush()
	return out
}
