package main

import "unicode/utf8"

// Terminals draw East Asian Wide and Fullwidth characters across two columns.
// The renderer has to agree with the terminal about that, otherwise every line
// containing Hangul shifts and the diff buffer stops matching what is actually
// on screen.
//
// Ambiguous-width characters (box drawing, bullets, block elements) are treated
// as narrow here, which is what most terminals do — but CJK locales often
// configure them as wide instead. The game therefore avoids them entirely and
// draws its playfield in plain ASCII; only text is allowed to be wide.

var wideRanges = [][2]rune{
	{0x1100, 0x115F}, // Hangul Jamo initial consonants
	{0x2E80, 0x303E}, // CJK radicals, Kangxi, CJK symbols
	{0x3041, 0x33FF}, // kana, Hangul compat jamo, CJK compat
	{0x3400, 0x4DBF}, // CJK ext A
	{0x4E00, 0x9FFF}, // CJK unified
	{0xA000, 0xA4CF}, // Yi
	{0xA960, 0xA97F}, // Hangul Jamo extended A
	{0xAC00, 0xD7A3}, // Hangul syllables
	{0xF900, 0xFAFF}, // CJK compat ideographs
	{0xFE10, 0xFE19}, // vertical forms
	{0xFE30, 0xFE6F}, // CJK compat forms
	{0xFF00, 0xFF60}, // fullwidth forms
	{0xFFE0, 0xFFE6},
	{0x1F300, 0x1F64F}, // emoji
	{0x1F900, 0x1F9FF},
	{0x20000, 0x2FFFD},
	{0x30000, 0x3FFFD},
}

// runeWidth reports how many terminal columns r occupies.
func runeWidth(r rune) int {
	if r < 0x1100 {
		return 1
	}
	lo, hi := 0, len(wideRanges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < wideRanges[mid][0]:
			hi = mid - 1
		case r > wideRanges[mid][1]:
			lo = mid + 1
		default:
			return 2
		}
	}
	return 1
}

// strWidth is the column count of s, used for centring and right-aligning text.
func strWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// bytesWidth is strWidth for scratch labels assembled directly into UTF-8
// bytes. Decoding the slice directly avoids the heap-backed conversion that a
// helper call around range(string(b)) otherwise triggers.
func bytesWidth(b []byte) int {
	w := 0
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		w += runeWidth(r)
		b = b[n:]
	}
	return w
}

// truncate clips s to at most maxw columns without splitting a wide rune.
func truncate(s string, maxw int) string {
	w := 0
	for i, r := range s {
		rw := runeWidth(r)
		if w+rw > maxw {
			return s[:i]
		}
		w += rw
	}
	return s
}
