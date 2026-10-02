package tui

import "testing"

// The benchmarks fold a reply of 5000 lines, which is the size a long answer
// with code in it comes to, at the width of a normal terminal.

func BenchmarkWrapBlock5000(b *testing.B) {
	in := bigReply(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		WrapBlock(in, 80)
	}
}

func BenchmarkWrapBlockStyled5000(b *testing.B) {
	in := bigReply(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		WrapBlockStyled(in, 80)
	}
}

func BenchmarkLegacyWrapBlock5000(b *testing.B) {
	in := bigReply(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		legacyWrapBlock(in, 80)
	}
}

// A repaint with color off is what every repaint cost before color existed.
func BenchmarkRepaint5000ColorOff(b *testing.B) {
	f := Frame{Reply: []string{bigReply(5000)}, Kinds: []entryKind{{kind: kindReply}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderStyled(f, 40, 80)
	}
}

// A repaint with color on adds the spans of the rows in sight.
func BenchmarkRepaint5000ColorOn(b *testing.B) {
	f := Frame{Reply: []string{bigReply(5000)}, Kinds: []entryKind{{kind: kindReply}}, styleReplies: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderStyled(f, 40, 80)
	}
}

// The same repaint scrolled to the middle of the reply.
func BenchmarkRepaint5000ColorOnScrolled(b *testing.B) {
	f := Frame{Reply: []string{bigReply(5000)}, Kinds: []entryKind{{kind: kindReply}}, styleReplies: true, Scroll: 3000}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderStyled(f, 40, 80)
	}
}
