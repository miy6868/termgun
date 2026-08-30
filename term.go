package main

import (
	"bufio"
	"unicode/utf8"
)

// ---- ANSI control sequences -------------------------------------------------

const (
	seqEnterAlt   = "\x1b[?1049h"
	seqLeaveAlt   = "\x1b[?1049l"
	seqHideCursor = "\x1b[?25l"
	seqShowCursor = "\x1b[?25h"
	seqReset      = "\x1b[0m"
	seqClear      = "\x1b[2J"
	// DEC private mode 2026 asks supporting terminals to present the bytes
	// between these markers as one frame. Unsupported terminals ignore the
	// private mode, while supporting ones avoid showing a half-painted row.
	seqSyncBegin = "\x1b[?2026h"
	seqSyncEnd   = "\x1b[?2026l"

	// 1003: report every mouse motion, 1006: SGR extended coordinates.
	seqMouseOn  = "\x1b[?1003h\x1b[?1006h"
	seqMouseOff = "\x1b[?1006l\x1b[?1003l"

	// Kitty keyboard protocol. A plain terminal never tells us when a key is
	// released, which makes true diagonal movement impossible: the OS only
	// auto-repeats the most recently pressed key, so the other direction goes
	// silent. Terminals implementing this protocol report press, repeat and
	// release separately, giving real held-key state and no repeat-delay stall.
	//
	// Flags: 1 disambiguate escape codes, 2 report event types (press/repeat/
	// release), 8 report all keys as escape codes — 8 is what extends release
	// reporting to plain letters like WASD.
	// Focus reporting: the terminal sends CSI I when it gains focus and CSI O
	// when it loses it. Needed when reading the keyboard from /dev/input, so a
	// game does not keep walking while the player types in another window.
	seqFocusOn  = "\x1b[?1004h"
	seqFocusOff = "\x1b[?1004l"

	seqKittyQuery = "\x1b[?u\x1b[c" // capability query, then Primary DA as a fence
	seqKittyPush  = "\x1b[>11u"
	seqKittyPop   = "\x1b[<u"
)

// colorDefault asks the terminal for its own foreground/background.
const colorDefault int16 = -1

// ---- screen buffer ----------------------------------------------------------

// Cell is one character position on screen.
type Cell struct {
	R  rune
	FG int16
	BG int16
}

// wideCont marks the right half of a double-width rune. Nothing is emitted for
// it; the terminal's cursor already moved two columns for the left half.
const wideCont rune = -1

var blank = Cell{R: ' ', FG: colorDefault, BG: colorDefault}

// Screen is a double buffered 256-colour character grid. Flush only writes the
// cells that actually changed since the previous frame, which keeps a 60 fps
// full-screen game comfortably inside a terminal's write bandwidth.
type Screen struct {
	W, H int
	cur  []Cell
	prev []Cell
	out  *bufio.Writer

	// motionAnchor is the current cell of the primary moving glyph. Its old
	// position is emitted before the new frame so terminals without DEC 2026 do
	// not briefly show both positions during vertical movement.
	motionAnchor, prevMotionAnchor int

	// buf accumulates one whole frame's escape stream so Flush hands the writer
	// a single slice instead of a few thousand small calls, and blank holds a
	// row of cleared cells that Clear copies from.
	buf   []byte
	blank []Cell
}

func NewScreen(w, h int, out *bufio.Writer) *Screen {
	s := &Screen{out: out}
	s.Resize(w, h)
	return s
}

func (s *Screen) Resize(w, h int) {
	if w == s.W && h == s.H {
		return
	}
	s.W, s.H = w, h
	s.motionAnchor, s.prevMotionAnchor = -1, -1
	s.cur = make([]Cell, w*h)
	s.prev = make([]Cell, w*h)
	s.blank = make([]Cell, w)
	// Worst case per changed cell is a cursor move plus both colours; one
	// generous allocation here keeps Flush from ever growing the slice.
	s.buf = make([]byte, 0, w*h*24+64)
	for i := range s.blank {
		s.blank[i] = blank
	}
	for i := range s.cur {
		s.cur[i] = blank
		// Force a full repaint by making prev impossible to match.
		s.prev[i] = Cell{R: 0}
	}
	s.out.WriteString(seqClear)
}

// Clear resets the frame by copying a prepared blank row over the grid: copy of
// a Cell slice compiles to a bulk move, where an element-wise loop does not.
func (s *Screen) Clear() {
	s.motionAnchor = -1
	for y := 0; y < s.H; y++ {
		copy(s.cur[y*s.W:(y+1)*s.W], s.blank)
	}
}

// MarkMotionAnchor identifies the primary moving glyph for ordered presentation.
func (s *Screen) MarkMotionAnchor(x, y int) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	s.motionAnchor = y*s.W + x
}

func (s *Screen) Set(x, y int, r rune, fg, bg int16) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	i := y*s.W + x
	// The playfield is pure ASCII and is by far the most written path, so a
	// narrow rune landing on an already-narrow cell skips the pair handling
	// entirely. narrowRune is the same test runeWidth makes first.
	if narrowRune(r) && narrowRune(s.cur[i].R) {
		s.cur[i] = Cell{R: r, FG: fg, BG: bg}
		return
	}
	w := runeWidth(r)
	if w == 2 && x+1 >= s.W {
		// No room for the second half; a split glyph would desync the row.
		r, w = ' ', 1
	}
	s.breakPair(x, y)
	if w == 2 {
		s.breakPair(x+1, y)
	}
	s.cur[i] = Cell{R: r, FG: fg, BG: bg}
	if w == 2 {
		s.cur[i+1] = Cell{R: wideCont, FG: fg, BG: bg}
	}
}

// narrowRune reports whether r is certainly one column wide and not the right
// half of a pair, so callers can skip the wide-glyph bookkeeping.
func narrowRune(r rune) bool { return r >= 0 && r < 0x1100 }

// breakPair blanks both halves of a double-width rune when either half is about
// to be overwritten, so no orphaned half-glyph is left behind.
func (s *Screen) breakPair(x, y int) {
	i := y*s.W + x
	switch {
	case s.cur[i].R == wideCont:
		if x > 0 {
			s.cur[i-1] = blank
		}
		s.cur[i] = blank
	case runeWidth(s.cur[i].R) == 2:
		if x+1 < s.W {
			s.cur[i+1] = blank
		}
		s.cur[i] = blank
	}
}

// SetBG recolours a cell's background without touching its glyph.
func (s *Screen) SetBG(x, y int, bg int16) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	i := y*s.W + x
	s.cur[i].BG = bg
	// Keep both halves of a wide rune on the same background.
	if s.cur[i].R == wideCont && x > 0 {
		s.cur[i-1].BG = bg
	} else if runeWidth(s.cur[i].R) == 2 && x+1 < s.W {
		s.cur[i+1].BG = bg
	}
}

func (s *Screen) Str(x, y int, str string, fg, bg int16) {
	for _, r := range str {
		s.Set(x, y, r, fg, bg)
		x += runeWidth(r)
	}
}

// StrBytes draws UTF-8 bytes without copying them into a string, so callers
// that build a label into a scratch buffer each frame allocate nothing.
func (s *Screen) StrBytes(x, y int, b []byte, fg, bg int16) {
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		s.Set(x, y, r, fg, bg)
		x += runeWidth(r)
		b = b[n:]
	}
}

// Fill paints a solid rectangle. Zoomed-in walls make this the busiest writer
// on screen, so the rectangle is clipped once up front rather than rejecting
// each cell inside Set, and whole rows of a narrow rune are written straight
// into the row slice.
func (s *Screen) Fill(x, y, w, h int, r rune, fg, bg int16) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, s.W), min(y+h, s.H)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	if !narrowRune(r) {
		for j := y0; j < y1; j++ {
			for i := x0; i < x1; i++ {
				s.Set(i, j, r, fg, bg)
			}
		}
		return
	}
	c := Cell{R: r, FG: fg, BG: bg}
	for j := y0; j < y1; j++ {
		row := s.cur[j*s.W : j*s.W+s.W]
		for i := x0; i < x1; i++ {
			// Overwriting half of a wide glyph still has to clear its partner.
			if !narrowRune(row[i].R) {
				s.breakPair(i, j)
			}
			row[i] = c
		}
	}
}

// appendUint writes a small non-negative integer without going through strconv,
// which showed up in the profile purely as call and formatting overhead. Colour
// indices and ordinary terminal coordinates all fit in three digits; the loop
// only exists so an absurdly tall terminal cannot emit a corrupt escape.
func appendUint(b []byte, n int) []byte {
	if n >= 1000 {
		var d [8]byte
		i := len(d)
		for n > 0 {
			i--
			d[i] = byte('0' + n%10)
			n /= 10
		}
		return append(b, d[i:]...)
	}
	if n >= 100 {
		b = append(b, byte('0'+n/100))
		n %= 100
		return append(b, byte('0'+n/10), byte('0'+n%10))
	}
	if n >= 10 {
		return append(b, byte('0'+n/10), byte('0'+n%10))
	}
	return append(b, byte('0'+n))
}

type frameEncoder struct {
	buf     []byte
	fg, bg  int16
	cx, cy  int
	changed bool
}

func (s *Screen) appendChangedCell(e *frameEncoder, x, y int) {
	i := y*s.W + x
	c := s.cur[i]
	if c == s.prev[i] {
		return
	}
	if !e.changed {
		e.buf = append(e.buf, seqSyncBegin...)
		e.changed = true
	}
	s.prev[i] = c
	if c.R == wideCont {
		// Emitted together with its left half, which already advanced the cursor
		// across this column.
		return
	}
	if e.cx != x || e.cy != y {
		e.buf = append(e.buf, 0x1b, '[')
		e.buf = appendUint(e.buf, y+1)
		e.buf = append(e.buf, ';')
		e.buf = appendUint(e.buf, x+1)
		e.buf = append(e.buf, 'H')
		e.cx, e.cy = x, y
	}
	if c.FG != e.fg {
		e.fg = c.FG
		if e.fg == colorDefault {
			e.buf = append(e.buf, "\x1b[39m"...)
		} else {
			e.buf = append(e.buf, "\x1b[38;5;"...)
			e.buf = appendUint(e.buf, int(e.fg))
			e.buf = append(e.buf, 'm')
		}
	}
	if c.BG != e.bg {
		e.bg = c.BG
		if e.bg == colorDefault {
			e.buf = append(e.buf, "\x1b[49m"...)
		} else {
			e.buf = append(e.buf, "\x1b[48;5;"...)
			e.buf = appendUint(e.buf, int(e.bg))
			e.buf = append(e.buf, 'm')
		}
	}
	r := c.R
	if r == 0 {
		r = ' '
	}
	if r < utf8.RuneSelf {
		e.buf = append(e.buf, byte(r))
		e.cx++
	} else {
		e.buf = utf8.AppendRune(e.buf, r)
		e.cx += runeWidth(r)
	}
}

func (s *Screen) Flush() {
	e := frameEncoder{buf: s.buf[:0], fg: -2, bg: -2, cx: -1, cy: -1}
	// Erase the previous player cell first. Row-major output alone draws a body
	// moving upward before it clears the lower cell, which looks like a doubled
	// player on terminals that ignore synchronized-output markers.
	if old := s.prevMotionAnchor; old >= 0 && old != s.motionAnchor && old < len(s.cur) {
		s.appendChangedCell(&e, old%s.W, old/s.W)
	}
	for y := 0; y < s.H; y++ {
		base := y * s.W
		for x := 0; x < s.W; x++ {
			// Keep the common unchanged-cell check in this tight loop. Calling
			// the full encoder for every screen cell costs more than the ordered
			// anchor write this path exists to support.
			i := base + x
			if s.cur[i] != s.prev[i] {
				s.appendChangedCell(&e, x, y)
			}
		}
	}
	e.buf = append(e.buf, seqReset...)
	if e.changed {
		e.buf = append(e.buf, seqSyncEnd...)
	}
	s.prevMotionAnchor = s.motionAnchor
	s.buf = e.buf[:0]
	s.out.Write(e.buf)
	s.out.Flush()
}

// ---- terminal reply parsing -------------------------------------------------
//
// Pure byte inspection, kept out of the platform files so it stays testable on
// whatever machine is building rather than only on the one that can run it.

// kittyReplyBefore reports whether b holds a "CSI ? <flags> u" reply.
func kittyReplyBefore(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0x1b || b[i+1] != '[' || b[i+2] != '?' {
			continue
		}
		for j := i + 3; j < len(b); j++ {
			if b[j] == 'u' {
				return true
			}
			if b[j] < '0' || b[j] > '9' {
				break
			}
		}
	}
	return false
}

// indexDA1 finds the start of a "CSI ? ... c" device-attributes reply.
func indexDA1(b []byte) int {
	for i := 0; i+2 < len(b); i++ {
		if b[i] != 0x1b || b[i+1] != '[' {
			continue
		}
		for j := i + 2; j < len(b); j++ {
			if b[j] == 'c' {
				return i
			}
			if !(b[j] >= '0' && b[j] <= '9' || b[j] == ';' || b[j] == '?') {
				break
			}
		}
	}
	return -1
}

// dropDA1 removes the device-attributes reply starting at i, returning any real
// user input that arrived alongside it.
func dropDA1(b []byte, i int) []byte {
	for j := i; j < len(b); j++ {
		if b[j] == 'c' {
			return append(append([]byte{}, b[:i]...), b[j+1:]...)
		}
	}
	return b[:i]
}
