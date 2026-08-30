package main

import (
	"testing"
	"time"
)

// The Windows console hands the game a packed C union and the field offsets are
// the whole game: get one wrong and every key is mis-read, silently. These tests
// build the exact bytes the console produces so the layout is checked here
// rather than discovered on someone's machine.

func keyRecord(down bool, vk uint16, ch rune, ctrl bool) winRecord {
	r := winRecord{eventType: winKeyEvent}
	put32 := func(off int, v uint32) {
		r.data[off] = byte(v)
		r.data[off+1] = byte(v >> 8)
		r.data[off+2] = byte(v >> 16)
		r.data[off+3] = byte(v >> 24)
	}
	put16 := func(off int, v uint16) {
		r.data[off] = byte(v)
		r.data[off+1] = byte(v >> 8)
	}
	if down {
		put32(0, 1)
	}
	put16(4, 1) // wRepeatCount
	put16(6, vk)
	put16(8, 0) // wVirtualScanCode
	put16(10, uint16(ch))
	if ctrl {
		put32(12, winLeftCtrl)
	}
	return r
}

func mouseRecord(x, y int16, buttons, flags uint32) winRecord {
	r := winRecord{eventType: winMouseEvent}
	put32 := func(off int, v uint32) {
		r.data[off] = byte(v)
		r.data[off+1] = byte(v >> 8)
		r.data[off+2] = byte(v >> 16)
		r.data[off+3] = byte(v >> 24)
	}
	put32(0, uint32(uint16(x))|uint32(uint16(y))<<16)
	put32(4, buttons)
	put32(8, 0)
	put32(12, flags)
	return r
}

// TestWindowsKeysCarryReleases is the reason the console is read directly rather
// than as VT input: a plain terminal never says when a key came up, which is
// what makes diagonal movement guesswork.
func TestWindowsKeysCarryReleases(t *testing.T) {
	held := winKeyState{}

	ev, ok := decodeKey(ptr(keyRecord(true, 'W', 'w', false)), held)
	if !ok || ev.Rune != 'w' || ev.KeyAct != KeyPress {
		t.Fatalf("first press decoded as %+v (ok=%v)", ev, ok)
	}
	// Windows repeats a held key by sending more downs; that is a repeat, not a
	// second press.
	ev, _ = decodeKey(ptr(keyRecord(true, 'W', 'w', false)), held)
	if ev.KeyAct != KeyRepeat {
		t.Errorf("a held key repeating decoded as %v, want KeyRepeat", ev.KeyAct)
	}
	ev, _ = decodeKey(ptr(keyRecord(false, 'W', 'w', false)), held)
	if ev.KeyAct != KeyRelease {
		t.Errorf("key up decoded as %v, want KeyRelease", ev.KeyAct)
	}
	// And after the release it is a fresh press again.
	ev, _ = decodeKey(ptr(keyRecord(true, 'W', 'w', false)), held)
	if ev.KeyAct != KeyPress {
		t.Errorf("press after release decoded as %v, want KeyPress", ev.KeyAct)
	}
}

// TestWindowsMovementKeysSurviveShift covers caps lock and a held shift: the
// character arrives upper-cased and movement is matched on the character.
func TestWindowsMovementKeysSurviveShift(t *testing.T) {
	for _, c := range []rune{'W', 'A', 'S', 'D'} {
		ev, ok := decodeKey(ptr(keyRecord(true, uint16(c), c, false)), winKeyState{})
		if !ok {
			t.Fatalf("%c was dropped", c)
		}
		if dirFor(ev.Rune, ev.Key) < 0 {
			t.Errorf("%c decoded as %q, which is not a movement key", c, ev.Rune)
		}
	}
}

// TestWindowsMovementReleaseWithoutCharacter covers console hosts that leave
// UnicodeChar empty on key-up even though the virtual-key code is still set.
func TestWindowsMovementReleaseWithoutCharacter(t *testing.T) {
	held := winKeyState{'W': true}
	ev, ok := decodeKey(ptr(keyRecord(false, 'W', 0, false)), held)
	if !ok || ev.Rune != 'w' || ev.KeyAct != KeyRelease {
		t.Fatalf("characterless W release decoded as %+v (ok=%v)", ev, ok)
	}
	if held['W'] {
		t.Error("characterless W release left the key held")
	}
}

// TestWindowsArrowsAndSpecials checks the keys that arrive with no character.
func TestWindowsArrowsAndSpecials(t *testing.T) {
	cases := []struct {
		vk   uint16
		want Key
	}{
		{vkUp, KeyUp}, {vkDown, KeyDown}, {vkLeft, KeyLeft}, {vkRight, KeyRight},
		{vkEscape, KeyEscape}, {vkReturn, KeyEnter}, {vkTab, KeyTab},
		{vkSpace, KeySpace},
	}
	for _, c := range cases {
		ev, ok := decodeKey(ptr(keyRecord(true, c.vk, 0, false)), winKeyState{})
		if !ok || ev.Key != c.want {
			t.Errorf("virtual key %#x decoded as key %v (ok=%v), want %v",
				c.vk, ev.Key, ok, c.want)
		}
	}
	// Arrows have to work as movement too.
	for _, c := range []uint16{vkUp, vkDown, vkLeft, vkRight} {
		ev, _ := decodeKey(ptr(keyRecord(true, c, 0, false)), winKeyState{})
		if dirFor(ev.Rune, ev.Key) < 0 {
			t.Errorf("arrow %#x does not map to a direction", c)
		}
	}
}

// TestWindowsCtrlCQuits keeps the one key that has to work no matter what.
func TestWindowsCtrlCQuits(t *testing.T) {
	ev, ok := decodeKey(ptr(keyRecord(true, vkC, 3, true)), winKeyState{})
	if !ok || ev.Key != KeyCtrlC {
		t.Errorf("ctrl+c decoded as %+v, want KeyCtrlC", ev)
	}
	// Plain 'c' must not.
	ev, _ = decodeKey(ptr(keyRecord(true, vkC, 'c', false)), winKeyState{})
	if ev.Key == KeyCtrlC {
		t.Error("an unmodified c quit the game")
	}
}

// TestWindowsBareModifiersAreDropped: a shift going down carries no character
// and means nothing to the game.
func TestWindowsBareModifiersAreDropped(t *testing.T) {
	if _, ok := decodeKey(ptr(keyRecord(true, 0x10, 0, false)), winKeyState{}); ok {
		t.Error("a bare shift press produced an event")
	}
}

// TestWindowsMouseTracksTheWindow covers the two things that are easy to get
// wrong: the console reports button *state* rather than what changed, and it
// reports coordinates against the screen buffer rather than the visible window.
func TestWindowsMouseTracksTheWindow(t *testing.T) {
	// Moving with nothing held, in a window scrolled down 5 rows.
	ev, buttons, ok := decodeMouse(ptr(mouseRecord(20, 12, 0, winMouseMoved)), 0, 0, 5)
	if !ok || ev.Action != MouseMove {
		t.Fatalf("move decoded as %+v (ok=%v)", ev, ok)
	}
	if ev.MX != 20 || ev.MY != 7 {
		t.Errorf("mouse at buffer (20,12) with the window starting at row 5 "+
			"decoded as (%d,%d), want (20,7)", ev.MX, ev.MY)
	}

	// Press: the bit is newly set.
	ev, buttons, ok = decodeMouse(ptr(mouseRecord(20, 12, winBtn1, 0)), buttons, 0, 5)
	if !ok || ev.Action != MousePress || ev.Button != BtnLeft {
		t.Fatalf("left press decoded as %+v (ok=%v)", ev, ok)
	}
	// Release: the state goes to zero, and which button it was has to come from
	// the previous state.
	ev, _, ok = decodeMouse(ptr(mouseRecord(20, 12, 0, 0)), buttons, 0, 5)
	if !ok || ev.Action != MouseRelease || ev.Button != BtnLeft {
		t.Errorf("left release decoded as %+v (ok=%v); the button it released "+
			"is only knowable from the previous state", ev, ok)
	}
}

func TestWindowsWheelKeepsDirection(t *testing.T) {
	up := uint32(uint16(120)) << 16
	downDelta := int16(-120)
	down := uint32(uint16(downDelta)) << 16
	for _, c := range []struct {
		buttons uint32
		want    int
	}{{up, BtnWheelUp}, {down, BtnWheelDown}} {
		ev, _, ok := decodeMouse(ptr(mouseRecord(5, 5, c.buttons, winMouseWheel)), 0, 0, 0)
		if !ok || ev.Action != MousePress || ev.Button != c.want {
			t.Errorf("wheel decoded as %+v (ok=%v), want button %d", ev, ok, c.want)
		}
	}
}

func TestWindowsMouseFloodDoesNotDelayKeys(t *testing.T) {
	var got []Event
	emit := func(ev Event) { got = append(got, ev) }
	var c winEventCoalescer
	start := time.Unix(1, 0)

	for x := 0; x < 100; x++ {
		c.offer(Event{Kind: EvMouse, Action: MouseMove, MX: x}, start, emit)
	}
	c.offer(Event{Kind: EvKey, Rune: 'w'}, start, emit)

	if len(got) != 2 {
		t.Fatalf("mouse flood emitted %d events, want one move and one key", len(got))
	}
	if got[0].Kind != EvMouse || got[0].MX != 99 {
		t.Errorf("coalesced mouse event = %+v, want latest position", got[0])
	}
	if got[1].Kind != EvKey || got[1].Rune != 'w' {
		t.Errorf("event after mouse flood = %+v, want key", got[1])
	}
}

// TestWindowsInputModeIsPrecise: the console reports releases, so movement must
// not fall back to inferring held keys from auto-repeat.
func TestWindowsInputModeIsPrecise(t *testing.T) {
	if !InputWinConsole.trueState() {
		t.Error("the Windows console mode must report true key state")
	}
	if InputWinConsole.String() == "" {
		t.Error("the Windows console mode has no description for the player")
	}
}

func ptr(r winRecord) *winRecord { return &r }
