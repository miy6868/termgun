package main

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// fakeTerm is a minimal terminal emulator: it replays the escape stream the
// renderer produces and, crucially, advances the cursor by two columns for
// double-width runes the way a real terminal does. Comparing its grid against
// the Screen's own buffer is what catches desync — the bug that left shredded
// half-drawn cells on screen forever, because a diff renderer never repaints a
// cell it believes is already correct.
type fakeTerm struct {
	w, h   int
	grid   []rune
	cx, cy int
}

func TestLowHealthVignetteDoesNotOverlapBossBar(t *testing.T) {
	g := arena(t, 141)
	g.player.hp = g.player.maxHP * 0.10
	g.addEnemy(bossDef, Vec{20.5, 4.5})
	g.enemies[0].alert = true
	s := newTestScreen(80, 24)

	g.Draw(s)
	if strings.Contains(screenRow(s, hudTop), bossDef.Name) {
		t.Fatal("boss bar still occupies the low-health border row")
	}
	if !strings.Contains(screenRow(s, hudTop+1), bossDef.Name) {
		t.Fatal("boss bar was not moved to the protected inner row")
	}
}

func TestFlushPresentsChangedFrameSynchronously(t *testing.T) {
	var out bytes.Buffer
	s := NewScreen(20, 5, bufio.NewWriter(&out))
	s.out.Flush() // discard Resize's initial clear sequence
	out.Reset()
	s.Clear()
	s.Str(0, 0, "frame", 231, colorDefault)
	s.Flush()

	b := out.Bytes()
	if !bytes.Contains(b, []byte(seqSyncBegin)) || !bytes.HasSuffix(b, []byte(seqSyncEnd)) {
		t.Fatalf("changed frame is not enclosed by synchronized-output markers: %q", b)
	}
}

func TestFlushErasesOldPlayerBeforeDrawingNewPosition(t *testing.T) {
	var out bytes.Buffer
	s := NewScreen(20, 5, bufio.NewWriter(&out))
	s.Clear()
	s.Set(5, 3, '@', colPlayer, colorDefault)
	s.MarkMotionAnchor(5, 3)
	s.Flush()
	out.Reset()

	// Moving upward puts the new cell earlier in a row-major diff. A terminal
	// without synchronized-output support must still see the old body erased
	// before the new one appears, or both are visible during the frame.
	s.Clear()
	s.Set(5, 3, '.', colFloorLit, colorDefault)
	s.Set(5, 2, '@', colPlayer, colorDefault)
	s.MarkMotionAnchor(5, 2)
	s.Flush()
	frame := out.Bytes()
	erased, drawn := bytes.IndexByte(frame, '.'), bytes.IndexByte(frame, '@')
	if erased < 0 || drawn < 0 {
		t.Fatalf("player move did not emit both cells: %q", frame)
	}
	if erased > drawn {
		t.Fatalf("new player was emitted before the old position was erased: %q", frame)
	}
}

// termWidth is deliberately an independent implementation of the width rule.
// If the emulator reused the renderer's own runeWidth, the two would agree even
// when both are wrong, and the test would prove nothing.
func termWidth(r rune) int {
	switch {
	case r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x3040 && r <= 0x30FF, // kana
		r >= 0x3130 && r <= 0x318F, // Hangul compatibility jamo
		r >= 0x4E00 && r <= 0x9FFF, // CJK unified ideographs
		r >= 0xFF01 && r <= 0xFF60: // fullwidth forms
		return 2
	}
	return 1
}

func newFakeTerm(w, h int) *fakeTerm {
	f := &fakeTerm{w: w, h: h, grid: make([]rune, w*h)}
	for i := range f.grid {
		f.grid[i] = ' '
	}
	return f
}

func (f *fakeTerm) feed(b []byte) {
	for i := 0; i < len(b); {
		if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '[' {
			j := i + 2
			for j < len(b) && !(b[j] >= '@' && b[j] <= '~') {
				j++
			}
			if j >= len(b) {
				return
			}
			params := string(b[i+2 : j])
			switch b[j] {
			case 'H':
				p := strings.Split(params, ";")
				row, col := 1, 1
				if len(p) > 0 && p[0] != "" {
					row, _ = strconv.Atoi(p[0])
				}
				if len(p) > 1 && p[1] != "" {
					col, _ = strconv.Atoi(p[1])
				}
				f.cy, f.cx = row-1, col-1
			case 'J':
				for k := range f.grid {
					f.grid[k] = ' '
				}
			}
			i = j + 1
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		i += size
		if r < ' ' {
			continue
		}
		w := termWidth(r)
		if f.cy >= 0 && f.cy < f.h && f.cx >= 0 && f.cx+w <= f.w {
			f.grid[f.cy*f.w+f.cx] = r
			if w == 2 {
				f.grid[f.cy*f.w+f.cx+1] = wideCont
			}
		}
		f.cx += w
	}
}

// diff compares the emulated terminal against what the renderer believes it drew.
func (f *fakeTerm) diff(s *Screen) string {
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			want := s.cur[y*s.W+x].R
			if want == 0 {
				want = ' '
			}
			if got := f.grid[y*f.w+x]; got != want {
				return "row " + strconv.Itoa(y) + " col " + strconv.Itoa(x) +
					": terminal has " + strconv.QuoteRune(got) +
					", renderer expected " + strconv.QuoteRune(want) +
					"\n terminal: " + strconv.Quote(f.row(y)) +
					"\n expected: " + strconv.Quote(screenRow(s, y))
			}
		}
	}
	return ""
}

func (f *fakeTerm) row(y int) string {
	var sb strings.Builder
	for x := 0; x < f.w; x++ {
		if r := f.grid[y*f.w+x]; r != wideCont {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func screenRow(s *Screen, y int) string {
	var sb strings.Builder
	for x := 0; x < s.W; x++ {
		r := s.cur[y*s.W+x].R
		if r == wideCont {
			continue
		}
		if r == 0 {
			r = ' '
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// TestTerminalStaysInSync plays several frames — including the Korean message
// log, the perk overlay and the game-over box — and checks after every flush
// that a width-aware terminal ends up with exactly the intended grid.
func TestTerminalStaysInSync(t *testing.T) {
	const w, h = 100, 30
	var out bytes.Buffer
	s := NewScreen(w, h, bufio.NewWriter(&out))
	term := newFakeTerm(w, h)

	g := NewGame(3)
	g.Draw(s)

	check := func(label string) {
		t.Helper()
		s.Flush()
		term.feed(out.Bytes())
		out.Reset()
		if d := term.diff(s); d != "" {
			t.Fatalf("%s: terminal out of sync\n%s", label, d)
		}
	}
	check("first frame")

	// Long Hangul messages of differing lengths: a shorter one must fully erase
	// the longer one underneath it.
	for _, m := range []string{
		"의료 키트 +30 HP",
		"!! OVERSEER 의 기척이 느껴진다 !!",
		"무기 습득: Shotgun",
		"레벨 3 달성!",
		"특성 획득: 화력 증강",
	} {
		g.log(m, 213)
		g.Update(1.0 / 60)
		g.Draw(s)
		check("message " + m)
	}

	g.player.xp = g.player.xpNext
	g.gainXP(1) // opens the perk overlay
	if g.state != StateLevelUp {
		t.Fatal("expected the level-up overlay to open")
	}
	g.Draw(s)
	check("perk overlay")

	g.handleKey(Event{Kind: EvKey, Rune: '1'})
	g.Draw(s)
	check("overlay dismissed")

	g.handleKey(Event{Kind: EvKey, Rune: '?'})
	g.Draw(s)
	check("help overlay")
	g.handleKey(Event{Kind: EvKey, Rune: ' '})
	g.Draw(s)
	check("help dismissed")

	g.state = StateDead
	g.Draw(s)
	check("game over box")

	g.showMap = true
	g.state = StatePlaying
	for i := 0; i < 30; i++ {
		g.Update(1.0 / 60)
		g.Draw(s)
		check("minimap frame")
	}
}

// TestPlayfieldIsNarrow guards the rule that keeps the map aligned in every
// terminal: nothing drawn in the play area may be a wide or ambiguous-width
// rune, since CJK locales render ambiguous glyphs double-width.
func TestPlayfieldIsNarrow(t *testing.T) {
	g := NewGame(11)
	s := newTestScreen(100, 30)
	g.mouseSet = true
	g.firing = true

	for i := 0; i < 400; i++ {
		if e := nearestEnemy(g); e != nil {
			g.aim = e.pos
		}
		g.pressMove('d', KeyNone)
		g.Update(1.0 / 60)
		g.Draw(s)
		for y := g.viewY; y < g.viewY+g.viewH; y++ {
			for x := 0; x < s.W; x++ {
				r := s.cur[y*s.W+x].R
				if r > 0x7F {
					t.Fatalf("frame %d: non-ASCII glyph %q at (%d,%d) in the play area", i, r, x, y)
				}
			}
		}
	}
}

// TestUIAvoidsAmbiguousWidthGlyphs keeps the screen buffer and CJK terminals
// in agreement. Hangul has a stable width of two and ASCII a stable width of
// one, but symbols such as arrows and the middle dot are locale-dependent.
// If one reaches an overlay, every cell after it can be shifted on a Korean
// terminal even though the renderer believes the row is aligned.
func TestUIAvoidsAmbiguousWidthGlyphs(t *testing.T) {
	g := NewGame(12)
	s := newTestScreen(120, 44)
	cases := []struct {
		state State
		page  int
	}{
		{StatePlaying, 0},
		{StateWeaponCore, 0},
		{StateHelp, 0},
		{StateHelp, 1},
		{StateHelp, 2},
		{StateSettings, 0},
		{StatePaused, 0},
		{StateDead, 0},
		{StateVictory, 0},
	}
	for _, tc := range cases {
		g.state, g.helpPage = tc.state, tc.page
		g.Draw(s)
		for y := 0; y < s.H; y++ {
			for x := 0; x < s.W; x++ {
				r := s.cur[y*s.W+x].R
				if r > 0x7f && (r < 0xac00 || r > 0xd7a3) {
					t.Fatalf("state %d page %d: locale-dependent glyph %q at (%d,%d)",
						tc.state, tc.page, r, x, y)
				}
			}
		}
	}
}

func TestRuneWidth(t *testing.T) {
	for _, c := range []struct {
		r rune
		w int
	}{
		{'a', 1}, {'#', 1}, {'@', 1}, {' ', 1},
		{'층', 2}, {'가', 2}, {'한', 2}, {'漢', 2}, {'あ', 2}, {'ｱ', 1},
	} {
		if got := runeWidth(c.r); got != c.w {
			t.Errorf("runeWidth(%q) = %d, want %d", c.r, got, c.w)
		}
	}
	if got := strWidth("B1층 | 처치 3"); got != 13 {
		t.Errorf("strWidth = %d, want 13", got)
	}
}

// TestWidePairsAreAtomic checks that overwriting either half of a wide rune
// clears both, so no orphaned half-glyph is left on screen.
func TestWidePairsAreAtomic(t *testing.T) {
	s := newTestScreen(10, 2)
	s.Clear()
	s.Set(2, 0, '층', 15, colorDefault)
	if s.cur[2].R != '층' || s.cur[3].R != wideCont {
		t.Fatalf("wide rune did not claim two cells: %v %v", s.cur[2].R, s.cur[3].R)
	}

	s.Set(3, 0, 'x', 15, colorDefault) // overwrite the right half
	if s.cur[2].R != ' ' {
		t.Fatalf("left half survived as %q", s.cur[2].R)
	}

	s.Set(6, 0, '가', 15, colorDefault)
	s.Set(6, 0, 'y', 15, colorDefault) // overwrite the left half
	if s.cur[7].R != ' ' {
		t.Fatalf("right half survived as %q", s.cur[7].R)
	}

	// A wide rune must never straddle the right edge.
	s.Set(9, 0, '층', 15, colorDefault)
	if s.cur[9].R != ' ' {
		t.Fatalf("wide rune was placed in the last column as %q", s.cur[9].R)
	}
}

// TestStatusBarFitsEveryWidth checks that the top bar degrades gracefully:
// the floor/score block must survive at any supported terminal width, and the
// two halves must never overwrite each other.
func TestStatusBarFitsEveryWidth(t *testing.T) {
	g := NewGame(5)
	g.depth = 12
	g.kills = 137
	g.score = 98765
	for _, w := range []int{60, 64, 72, 80, 100, 120, 200} {
		s := newTestScreen(w, 30)
		g.Draw(s)
		row := screenRow(s, 0)
		if !strings.Contains(row, "B12층") {
			t.Errorf("width %d: floor indicator missing from status bar: %q", w, row)
		}
		if strWidth(row) != w {
			t.Errorf("width %d: status bar is %d columns wide: %q", w, strWidth(row), row)
		}
		// A half-drawn wide rune at the edge would mean a torn cell.
		if s.cur[w-1].R == wideCont && s.cur[w-2].R == ' ' {
			t.Errorf("width %d: orphaned wide-rune half at the right edge", w)
		}
	}
}

// TestHUDShowsEveryMechanic is the discoverability guarantee: a player who has
// never read the README must be able to see, from the status area alone, that
// they have health, a dash, a level, a weapon list keyed to the number row, and
// a help key. None of these may be dropped at any supported terminal width.
func TestHUDShowsEveryMechanic(t *testing.T) {
	for _, w := range []int{60, 72, 80, 100, 120, 160} {
		g := NewGame(6)
		g.player.owned[wpShotgun] = true
		g.player.weapon = wpShotgun
		s := newTestScreen(w, 30)
		g.Draw(s)
		vitals, slots := screenRow(s, 0), screenRow(s, 1)

		for _, want := range []string{"HP", "DASH", "Lv"} {
			if !strings.Contains(vitals, want) {
				t.Errorf("width %d: %q missing from the status row: %q", w, want, vitals)
			}
		}
		if !strings.Contains(vitals, "B1층") {
			t.Errorf("width %d: floor indicator missing: %q", w, vitals)
		}
		// Every weapon and its key, including ones not picked up yet.
		for i := range weapons {
			key := strconv.Itoa(i + 1)
			if !strings.Contains(slots, key+weapons[i].Short) &&
				!strings.Contains(slots, key+" "+weapons[i].Name) {
				t.Errorf("width %d: weapon slot %s (%s) missing from %q", w, key, weapons[i].Name, slots)
			}
		}
		if !strings.Contains(slots, "[?]") {
			t.Errorf("width %d: help hint missing from %q", w, slots)
		}
		for i, row := range []string{vitals, slots} {
			if strWidth(row) != w {
				t.Errorf("width %d: HUD row %d is %d columns: %q", w, i, strWidth(row), row)
			}
		}
	}
}

// TestHelpOverlayOpens checks the advertised help key actually works and that
// the overlay names the core controls.
func TestHelpOverlayOpens(t *testing.T) {
	g := NewGame(6)
	s := newTestScreen(100, 30)
	g.Draw(s)

	g.handleKey(Event{Kind: EvKey, Rune: '?'})
	if g.state != StateHelp {
		t.Fatalf("[?] did not open the help overlay (state %d)", g.state)
	}
	g.Update(1.0 / 60)
	if g.elapsed != 0 {
		t.Error("the game kept simulating while the help overlay was open")
	}
	g.Draw(s)

	var screen strings.Builder
	for y := 0; y < 30; y++ {
		screen.WriteString(screenRow(s, y))
		screen.WriteByte('\n')
	}
	for _, want := range []string{"W A S D", "마우스", "대시", "계단", "지도"} {
		if !strings.Contains(screen.String(), want) {
			t.Errorf("help overlay does not mention %q", want)
		}
	}

	g.handleKey(Event{Kind: EvKey, Rune: 'x'})
	if g.state != StatePlaying {
		t.Fatalf("a keypress did not dismiss the help overlay (state %d)", g.state)
	}
}
