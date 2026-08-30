package main

import "time"

// Decoding Windows console input records.
//
// This file has no build tag on purpose. The Windows console hands the game a
// stream of fixed-size INPUT_RECORDs, and turning those into the game's own
// events is ordinary logic with no system calls in it — so it is kept apart
// from the syscalls in input_windows.go and tested on whatever machine happens
// to be building. The alternative is code that can only be exercised by running
// it on Windows, which is exactly the code that ends up wrong.

// INPUT_RECORD event types.
const (
	winKeyEvent    = 0x0001
	winMouseEvent  = 0x0002
	winResizeEvent = 0x0004
	winFocusEvent  = 0x0010
)

// PowerShell's console host can produce mouse-move records much faster than
// the game renders. Keep only the newest position between short flushes so
// those records cannot queue in front of keyboard input for several frames.
const winMouseFlushInterval = time.Second / 120

type winEventCoalescer struct {
	pendingMouse Event
	hasMouse     bool
	lastFlush    time.Time
}

func (c *winEventCoalescer) offer(ev Event, now time.Time, emit func(Event)) {
	if ev.Kind == EvMouse && ev.Action == MouseMove {
		c.pendingMouse, c.hasMouse = ev, true
		c.flushDue(now, emit)
		return
	}
	c.flush(emit)
	emit(ev)
	c.lastFlush = now
}

func (c *winEventCoalescer) flushDue(now time.Time, emit func(Event)) {
	if c.hasMouse && c.lastFlush.IsZero() {
		c.lastFlush = now
		return
	}
	if c.hasMouse && now.Sub(c.lastFlush) >= winMouseFlushInterval {
		c.flush(emit)
		c.lastFlush = now
	}
}

func (c *winEventCoalescer) flush(emit func(Event)) {
	if !c.hasMouse {
		return
	}
	emit(c.pendingMouse)
	c.hasMouse = false
}

// Mouse event flags and button bits.
const (
	winMouseMoved = 0x0001
	winMouseWheel = 0x0004

	winBtn1 = 0x0001 // leftmost
	winBtn2 = 0x0002 // rightmost
	winBtn3 = 0x0004 // middle
)

// Control key state bits.
const (
	winRightCtrl = 0x0004
	winLeftCtrl  = 0x0008
)

// Virtual key codes for the keys that do not arrive as characters.
const (
	vkTab    = 0x09
	vkReturn = 0x0D
	vkEscape = 0x1B
	vkSpace  = 0x20
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28
	vkC      = 0x43
)

// winRecord is one INPUT_RECORD. The payload is kept as raw bytes rather than
// as a Go union because the C type is a union of four differently shaped
// structs; reading fields out by offset is both simpler and immune to whatever
// the compiler decides about alignment. The record is 20 bytes on every
// architecture Windows runs on, since no member is wider than a DWORD.
type winRecord struct {
	eventType uint16
	_         uint16
	data      [16]byte
}

func (r *winRecord) u16(off int) uint16 {
	return uint16(r.data[off]) | uint16(r.data[off+1])<<8
}

func (r *winRecord) i16(off int) int16 { return int16(r.u16(off)) }

func (r *winRecord) u32(off int) uint32 {
	return uint32(r.u16(off)) | uint32(r.u16(off+2))<<16
}

// winKeyState tracks which keys are currently held.
//
// Windows repeats a held key by sending more key-down records, exactly like a
// terminal's auto-repeat. The game already distinguishes a first press from a
// repeat — the kitty protocol reports both — so the same distinction is
// reproduced here rather than passing every repeat off as a fresh press.
type winKeyState map[uint16]bool

// decodeKey turns a key record into an event. ok is false for records the game
// has no use for, such as a bare modifier going down.
func decodeKey(r *winRecord, held winKeyState) (ev Event, ok bool) {
	down := r.u32(0) != 0
	vk := r.u16(6)
	ch := rune(r.u16(10))
	// Some console hosts omit UnicodeChar on key-up records. Movement releases
	// must still carry their letter or WASD remains held until focus changes.
	if !down && ch == 0 && vk >= 'A' && vk <= 'Z' {
		ch = rune(vk)
	}
	ctrl := r.u32(12)&(winLeftCtrl|winRightCtrl) != 0

	ev = Event{Kind: EvKey, Src: SrcTTY}
	switch {
	case !down:
		ev.KeyAct = KeyRelease
		delete(held, vk)
	case held[vk]:
		ev.KeyAct = KeyRepeat
	default:
		ev.KeyAct = KeyPress
		held[vk] = true
	}

	switch vk {
	case vkUp:
		ev.Key = KeyUp
	case vkDown:
		ev.Key = KeyDown
	case vkLeft:
		ev.Key = KeyLeft
	case vkRight:
		ev.Key = KeyRight
	case vkEscape:
		ev.Key = KeyEscape
	case vkReturn:
		ev.Key = KeyEnter
	case vkTab:
		ev.Key = KeyTab
	case vkSpace:
		ev.Key, ev.Rune = KeySpace, ' '
	case vkC:
		if ctrl {
			ev.Key = KeyCtrlC
			return ev, true
		}
		fallthrough
	default:
		if ch < ' ' {
			// A modifier, a function key, or anything else with no character:
			// nothing downstream can act on it.
			return ev, false
		}
		// Lower-cased because movement is matched on the character, and holding
		// shift or leaving caps lock on must not stop you walking.
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		ev.Rune = ch
	}
	return ev, true
}

// decodeMouse turns a mouse record into an event, given the button state from
// the previous record and the console window's top-left corner.
//
// The console reports which buttons are down, not which one just changed, so a
// press and a release are both "the button bits are different from last time".
// Coordinates are relative to the screen buffer, which is not the same thing as
// the visible window once the buffer is taller than the window.
func decodeMouse(r *winRecord, prev uint32, originX, originY int) (ev Event, buttons uint32, ok bool) {
	rawButtons := r.u32(4)
	buttons = rawButtons & (winBtn1 | winBtn2 | winBtn3)
	flags := r.u32(12)
	ev = Event{
		Kind: EvMouse,
		MX:   int(r.i16(0)) - originX,
		MY:   int(r.i16(2)) - originY,
	}

	switch {
	case flags&winMouseWheel != 0:
		ev.Action = MousePress
		if int16(rawButtons>>16) > 0 {
			ev.Button = BtnWheelUp
		} else {
			ev.Button = BtnWheelDown
		}
	case flags&winMouseMoved != 0:
		ev.Action = MouseMove
		ev.Button = winButton(buttons)
	case buttons&^prev != 0:
		ev.Action = MousePress
		ev.Button = winButton(buttons &^ prev)
	case prev&^buttons != 0:
		ev.Action = MouseRelease
		ev.Button = winButton(prev &^ buttons)
	default:
		// A wheel tick or a double-click flag with no button change.
		return ev, buttons, false
	}
	return ev, buttons, true
}

func winButton(bits uint32) int {
	switch {
	case bits&winBtn1 != 0:
		return BtnLeft
	case bits&winBtn3 != 0:
		return BtnMiddle
	case bits&winBtn2 != 0:
		return BtnRight
	}
	return BtnNone
}
