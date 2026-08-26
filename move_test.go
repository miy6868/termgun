package main

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestOppositeKeysLastWins is the regression test for the reported bug: hold A,
// press D while still holding A, then release A. The player must move right the
// whole time — never stall because the two directions cancelled out.
func TestOppositeKeysLastWins(t *testing.T) {
	for _, trueState := range []bool{false, true} {
		m := newMovement(trueState)
		now := 0.0

		m.press(dirLeft, now, false)
		if d := m.dir(now); !approx(d.X, -1) {
			t.Fatalf("trueState=%v: holding A moved %v, want left", trueState, d)
		}

		// D pressed while A is still down.
		now += 0.05
		m.press(dirRight, now, false)
		if d := m.dir(now); !approx(d.X, 1) {
			t.Fatalf("trueState=%v: A+D moved %v, want right (last key wins)", trueState, d)
		}

		// A released; D is still held.
		now += 0.05
		if trueState {
			m.release(dirLeft)
		}
		m.press(dirRight, now, true) // auto-repeat keeps D alive
		if d := m.dir(now); !approx(d.X, 1) {
			t.Fatalf("trueState=%v: after releasing A, moved %v, want right", trueState, d)
		}

		// And the reverse order works too.
		now += 0.05
		m.press(dirLeft, now, false)
		if d := m.dir(now); !approx(d.X, -1) {
			t.Fatalf("trueState=%v: pressing A last moved %v, want left", trueState, d)
		}
	}
}

// TestAutoRepeatDoesNotStealPriority guards a subtle failure of last-key-wins:
// while both keys are down the OS keeps repeating one of them, and those
// repeats must not hand priority back to the older key.
func TestAutoRepeatDoesNotStealPriority(t *testing.T) {
	m := newMovement(true)
	now := 0.0
	m.press(dirLeft, now, false)
	now += 0.05
	m.press(dirRight, now, false)

	for i := 0; i < 20; i++ {
		now += 0.03
		m.press(dirLeft, now, true) // A repeating underneath
		if d := m.dir(now); !approx(d.X, 1) {
			t.Fatalf("repeat %d: direction flipped to %v; D was pressed later", i, d)
		}
	}
}

// TestDiagonalMovement checks that two axes held at once produce a normalised
// diagonal rather than one axis winning.
func TestDiagonalMovement(t *testing.T) {
	m := newMovement(true)
	m.press(dirUp, 0, false)
	m.press(dirRight, 0, false)

	d := m.dir(0)
	if d.X <= 0 || d.Y >= 0 {
		t.Fatalf("W+D gave %v, want up and right", d)
	}
	if !approx(d.len(), 1) {
		t.Fatalf("diagonal has length %.4f, want 1 (no speed boost)", d.len())
	}
	if !approx(d.X, -d.Y) {
		t.Fatalf("diagonal %v is not symmetric", d)
	}

	m.release(dirUp)
	if d := m.dir(0); !approx(d.X, 1) || !approx(d.Y, 0) {
		t.Fatalf("after releasing W, got %v, want right only", d)
	}
}

// TestReleaseStopsImmediately is what the kitty protocol buys us: no coasting
// while a hold timer runs out.
func TestReleaseStopsImmediately(t *testing.T) {
	m := newMovement(true)
	m.press(dirRight, 0, false)
	m.release(dirRight)
	if d := m.dir(0); d.len() != 0 {
		t.Fatalf("still moving %v after the key was released", d)
	}
}

// TestLegacyHoldBridgesRepeatDelay covers the compatibility path: a key must
// still count as held across the gap before the OS starts auto-repeating,
// otherwise movement stutters after the first step.
func TestLegacyHoldBridgesRepeatDelay(t *testing.T) {
	m := newMovement(false)
	m.press(dirRight, 0, false)
	if d := m.dir(0.25); !approx(d.X, 1) {
		t.Fatalf("stopped moving %v after 0.25s; the repeat delay was not bridged", d)
	}
	if d := m.dir(0.40); d.len() != 0 {
		t.Fatalf("still moving at 0.40s with no repeat; got %v", d)
	}

	// Once repeats arrive, the key should expire promptly after the last one.
	m2 := newMovement(false)
	now := 0.0
	m2.press(dirRight, now, false)
	for i := 0; i < 10; i++ {
		now += 0.03
		m2.press(dirRight, now, true)
	}
	if d := m2.dir(now + 0.02); !approx(d.X, 1) {
		t.Fatalf("dropped the key between repeats: %v", d)
	}
	if d := m2.dir(now + 0.15); d.len() != 0 {
		t.Fatalf("coasted 0.15s past the last repeat: %v", d)
	}
}

// TestKittyKeyParsing decodes the press/repeat/release encoding.
func TestKittyKeyParsing(t *testing.T) {
	cases := []struct {
		in     string
		rune   rune
		key    Key
		action KeyAction
	}{
		{"\x1b[97u", 'a', KeyNone, KeyPress},
		{"\x1b[97;1:1u", 'a', KeyNone, KeyPress},
		{"\x1b[97;1:2u", 'a', KeyNone, KeyRepeat},
		{"\x1b[97;1:3u", 'a', KeyNone, KeyRelease},
		{"\x1b[100;1:3u", 'd', KeyNone, KeyRelease},
		{"\x1b[32;1:1u", ' ', KeySpace, KeyPress},
		{"\x1b[27;1:1u", 0, KeyEscape, KeyPress},
		{"\x1b[99;5:1u", 0, KeyCtrlC, KeyPress},
		{"\x1b[1;1:3A", 0, KeyUp, KeyRelease},
		{"\x1b[63;1:1u", '?', KeyNone, KeyPress},
	}
	for _, c := range cases {
		ev, used, ok := parseEvent([]byte(c.in))
		if !ok || used != len(c.in) {
			t.Fatalf("%q: parse failed (ok=%v used=%d)", c.in, ok, used)
		}
		if ev.Rune != c.rune || ev.Key != c.key || ev.KeyAct != c.action {
			t.Errorf("%q: got rune=%q key=%v action=%v, want rune=%q key=%v action=%v",
				c.in, ev.Rune, ev.Key, ev.KeyAct, c.rune, c.key, c.action)
		}
	}
}

// TestKittyDetection checks the capability handshake reads replies correctly.
func TestKittyDetection(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  bool
	}{
		{"kitty then DA1", "\x1b[?11u\x1b[?62;c", true},
		{"DA1 only", "\x1b[?62;c", false},
		{"nothing", "", false},
		{"kitty flags zero", "\x1b[?0u\x1b[?62;c", true},
	}
	for _, c := range cases {
		b := []byte(c.reply)
		got := kittyReplyBefore(b)
		if i := indexDA1(b); i >= 0 {
			got = kittyReplyBefore(b[:i])
		}
		if got != c.want {
			t.Errorf("%s: detected=%v, want %v", c.name, got, c.want)
		}
	}

	// User input typed during the handshake must survive.
	b := []byte("\x1b[?11u\x1b[?62;cwd")
	i := indexDA1(b)
	if i < 0 {
		t.Fatal("DA1 reply not found")
	}
	if left := dropDA1(b, i); string(left) != "\x1b[?11uwd" {
		t.Fatalf("leftover input was mangled: %q", left)
	}
}

// TestGameMovementIntegration drives the whole path: key events in, player
// velocity out.
func TestGameMovementIntegration(t *testing.T) {
	g := newGameWithInput(1, InputKitty)
	g.level = testLevel([]string{
		"###########",
		"#.........#",
		"#.........#",
		"#.........#",
		"#.........#",
		"###########",
	})
	g.player.pos = Vec{5.5, 3.0}
	start := g.player.pos

	// Hold W and D: the player should end up up-and-right of where they began.
	g.handleKey(Event{Kind: EvKey, Rune: 'w'})
	g.handleKey(Event{Kind: EvKey, Rune: 'd'})
	for i := 0; i < 30; i++ {
		g.Update(1.0 / 60)
	}
	if g.player.pos.X <= start.X || g.player.pos.Y >= start.Y {
		t.Fatalf("diagonal input moved the player to %v from %v", g.player.pos, start)
	}

	// Releasing W leaves pure horizontal motion.
	g.handleKey(Event{Kind: EvKey, Rune: 'w', KeyAct: KeyRelease})
	mid := g.player.pos
	for i := 0; i < 30; i++ {
		g.Update(1.0 / 60)
	}
	if g.player.pos.Y < mid.Y-0.05 {
		t.Fatalf("player kept drifting up after W was released: %v -> %v", mid, g.player.pos)
	}
}

// ---- device (kernel) input --------------------------------------------------

// TestDeviceInputMovesPlayer checks kernel key events drive movement, including
// a real diagonal, in a terminal that reports nothing itself.
func TestDeviceInputMovesPlayer(t *testing.T) {
	g := newGameWithInput(1, InputDevice)
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirUp, KeyAct: KeyPress})
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyPress})

	d := g.moveDir()
	if d.X <= 0 || d.Y >= 0 {
		t.Fatalf("device W+D gave %v, want up and right", d)
	}

	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirUp, KeyAct: KeyRelease})
	if d := g.moveDir(); !approx(d.X, 1) || !approx(d.Y, 0) {
		t.Fatalf("after releasing W, got %v, want right only", d)
	}
}

// TestUnfocusedDeviceInputIgnored is the safeguard that makes reading the
// kernel keyboard acceptable: those events keep arriving while the player is
// typing in another window, and must not move the character.
func TestUnfocusedDeviceInputIgnored(t *testing.T) {
	g := newGameWithInput(1, InputDevice)
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyPress})
	if g.moveDir().len() == 0 {
		t.Fatal("focused device input did not move the player")
	}

	// Terminal loses focus: held keys are dropped and new ones ignored.
	g.HandleEvent(Event{Kind: EvFocus, Focus: false})
	if d := g.moveDir(); d.len() != 0 {
		t.Fatalf("kept moving %v after losing focus", d)
	}
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirLeft, KeyAct: KeyPress})
	if d := g.moveDir(); d.len() != 0 {
		t.Fatalf("unfocused keystroke moved the player %v", d)
	}

	// Focus returns and input works again.
	g.HandleEvent(Event{Kind: EvFocus, Focus: true})
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirLeft, KeyAct: KeyPress})
	if d := g.moveDir(); !approx(d.X, -1) {
		t.Fatalf("input did not resume after regaining focus: %v", d)
	}
}

// TestDeviceModeIgnoresTerminalMovementKeys guards against double input: the
// terminal still delivers WASD, and those stale presses would fight the real
// key state coming from the kernel.
func TestDeviceModeIgnoresTerminalMovementKeys(t *testing.T) {
	g := newGameWithInput(1, InputDevice)
	g.HandleEvent(Event{Kind: EvKey, Rune: 'd'}) // from the tty
	if d := g.moveDir(); d.len() != 0 {
		t.Fatalf("terminal keystroke moved the player %v while device input was active", d)
	}

	// Non-movement keys must still work from the terminal.
	g.HandleEvent(Event{Kind: EvKey, Rune: '6'})
	if g.player.weapon != wpMelee {
		t.Fatal("terminal non-movement keys stopped working in device mode")
	}
}

// TestFocusIgnoredWithoutDeviceInput: in terminal-driven modes, losing focus
// should not be able to strand held keys, but focus events must not break
// anything either.
func TestFocusEventsParse(t *testing.T) {
	for _, c := range []struct {
		in    string
		focus bool
	}{
		{"\x1b[I", true},
		{"\x1b[O", false},
	} {
		ev, used, ok := parseEvent([]byte(c.in))
		if !ok || used != len(c.in) {
			t.Fatalf("%q: parse failed", c.in)
		}
		if ev.Kind != EvFocus || ev.Focus != c.focus {
			t.Errorf("%q: got %+v, want focus=%v", c.in, ev, c.focus)
		}
	}
}

// TestInputModeNames keeps the player-facing description honest.
func TestInputModeNames(t *testing.T) {
	if !InputKitty.trueState() || !InputDevice.trueState() {
		t.Error("precise modes must report true key state")
	}
	if InputCompat.trueState() {
		t.Error("compat mode cannot know true key state")
	}
	for _, m := range []InputMode{InputCompat, InputKitty, InputDevice} {
		if m.String() == "" {
			t.Errorf("input mode %d has no description", m)
		}
	}
}
