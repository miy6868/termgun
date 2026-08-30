package main

import "unicode/utf8"

// inputSource is a key-state reader that has to be shut down with the game.
type inputSource interface{ Close() }

type EventKind int

const (
	EvKey EventKind = iota
	EvMouse
	EvResize
	EvFocus
	// EvStop reports that the platform's primary event source ended. Producers
	// never close the shared channel because resize and device sources may still
	// be publishing to it.
	EvStop
)

// EventSource says where a key event came from. Device events carry true
// press/release state but arrive regardless of which window has focus.
type EventSource int

const (
	SrcTTY EventSource = iota
	SrcDevice
)

type MouseAction int

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMove
)

const (
	BtnLeft      = 0
	BtnMiddle    = 1
	BtnRight     = 2
	BtnNone      = 3
	BtnWheelUp   = 4
	BtnWheelDown = 5
)

// Key codes for keys that arrive as escape sequences. Printable keys are
// delivered in Rune instead.
type Key int

const (
	KeyNone Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEscape
	KeyEnter
	KeySpace
	KeyTab
	KeyCtrlC
)

// KeyAction distinguishes the three things a key can do. Only terminals
// speaking the kitty keyboard protocol report anything but KeyPress.
type KeyAction int

const (
	KeyPress KeyAction = iota
	KeyRepeat
	KeyRelease
)

type Event struct {
	Kind   EventKind
	Rune   rune
	Key    Key
	KeyAct KeyAction
	Src    EventSource
	Dir    int  // movement slot, for device events (see dirFor)
	Focus  bool // EvFocus: whether the terminal gained focus

	// Mouse fields, in 0-based terminal cell coordinates.
	MX, MY int
	Action MouseAction
	Button int

	W, H int // resize
}

// parseEvent decodes the first event in b. ok is false when b holds only an
// incomplete sequence and more bytes are needed.
func parseEvent(b []byte) (ev Event, used int, ok bool) {
	if len(b) == 0 {
		return ev, 0, false
	}
	if b[0] != 0x1b {
		return parseRune(b)
	}
	if len(b) == 1 {
		return ev, 0, false // could be ESC or the start of a sequence
	}
	if b[1] != '[' && b[1] != 'O' {
		// Alt+key; treat as a plain ESC press and let the rune follow.
		return Event{Kind: EvKey, Key: KeyEscape}, 1, true
	}
	if len(b) == 2 {
		return ev, 0, false
	}
	if b[1] == '[' && b[2] == '<' {
		return parseSGRMouse(b)
	}

	// Generic CSI: parameters then a final byte in the range @..~
	for i := 2; i < len(b); i++ {
		c := b[i]
		if c >= '@' && c <= '~' {
			params := parseParams(b[2:i])
			ev := csiEvent(c, params)
			return ev, i + 1, true
		}
	}
	return ev, 0, false
}

// param is one CSI parameter along with its colon-separated sub-parameters,
// which the kitty protocol uses to attach the event type to the modifier field.
type param struct {
	sub [3]int
	n   int
}

func (p param) at(i int) int {
	if i < p.n {
		return p.sub[i]
	}
	return 0
}

func parseParams(b []byte) []param {
	var out []param
	cur := param{n: 1}
	seen := false
	flush := func() {
		if seen {
			out = append(out, cur)
		}
		cur = param{n: 1}
		seen = false
	}
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			seen = true
			if cur.n <= len(cur.sub) {
				cur.sub[cur.n-1] = cur.sub[cur.n-1]*10 + int(c-'0')
			}
		case c == ':':
			seen = true
			if cur.n < len(cur.sub) {
				cur.n++
			}
		case c == ';':
			flush()
		default:
			// '?' and '>' prefixes belong to terminal replies, not key events.
			return nil
		}
	}
	flush()
	return out
}

// csiEvent turns a parsed CSI sequence into a key event. It covers both the
// legacy encodings and the kitty protocol, where the first parameter is the key
// codepoint, the second holds modifiers and the event type, and the final byte
// is 'u' for ordinary keys.
func csiEvent(final byte, params []param) Event {
	ev := Event{Kind: EvKey, KeyAct: KeyPress}
	if len(params) >= 2 {
		switch params[1].at(1) {
		case 2:
			ev.KeyAct = KeyRepeat
		case 3:
			ev.KeyAct = KeyRelease
		}
	}
	mods := 0
	if len(params) >= 2 {
		if m := params[1].at(0); m > 0 {
			mods = m - 1
		}
	}

	switch final {
	case 'I':
		return Event{Kind: EvFocus, Focus: true}
	case 'O':
		return Event{Kind: EvFocus, Focus: false}
	case 'A':
		ev.Key = KeyUp
	case 'B':
		ev.Key = KeyDown
	case 'C':
		ev.Key = KeyRight
	case 'D':
		ev.Key = KeyLeft
	case 'u':
		if len(params) == 0 {
			return Event{Kind: EvKey, Key: KeyNone}
		}
		code := rune(params[0].at(0))
		switch code {
		case 27:
			ev.Key = KeyEscape
		case 13:
			ev.Key = KeyEnter
		case 9:
			ev.Key = KeyTab
		case 32:
			ev.Key, ev.Rune = KeySpace, ' '
		default:
			if mods&4 != 0 && code == 'c' { // ctrl+c
				ev.Key = KeyCtrlC
			} else {
				ev.Rune = code
			}
		}
	default:
		ev.Key = KeyNone
	}
	return ev
}

// parseSGRMouse decodes "\x1b[<b;x;yM" (press/move) and "...m" (release).
func parseSGRMouse(b []byte) (ev Event, used int, ok bool) {
	end := -1
	for i := 3; i < len(b); i++ {
		if b[i] == 'M' || b[i] == 'm' {
			end = i
			break
		}
	}
	if end < 0 {
		return ev, 0, false
	}
	var nums [3]int
	idx := 0
	for i := 3; i < end; i++ {
		c := b[i]
		switch {
		case c >= '0' && c <= '9':
			if idx < 3 {
				nums[idx] = nums[idx]*10 + int(c-'0')
			}
		case c == ';':
			idx++
		}
	}
	code := nums[0]
	ev = Event{
		Kind: EvMouse,
		MX:   nums[1] - 1,
		MY:   nums[2] - 1,
	}
	switch {
	case code&32 != 0: // motion flag
		ev.Action = MouseMove
		ev.Button = code & 3
	case b[end] == 'm':
		ev.Action = MouseRelease
		ev.Button = code & 3
	default:
		ev.Action = MousePress
		ev.Button = code & 3
	}
	// Wheel events are edge-triggered presses. Keeping their direction turns the
	// wheel into a natural weapon selector instead of silently discarding it.
	if code&64 != 0 {
		ev.Action = MousePress
		if code&1 == 0 {
			ev.Button = BtnWheelUp
		} else {
			ev.Button = BtnWheelDown
		}
	}
	return ev, end + 1, true
}

func parseRune(b []byte) (ev Event, used int, ok bool) {
	switch b[0] {
	case 3:
		return Event{Kind: EvKey, Key: KeyCtrlC}, 1, true
	case '\r', '\n':
		return Event{Kind: EvKey, Key: KeyEnter}, 1, true
	case '\t':
		return Event{Kind: EvKey, Key: KeyTab}, 1, true
	case ' ':
		return Event{Kind: EvKey, Key: KeySpace, Rune: ' '}, 1, true
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size <= 1 {
		if len(b) < 4 {
			return ev, 0, false // possibly a truncated multi-byte rune
		}
		return Event{Kind: EvKey, Key: KeyNone}, 1, true
	}
	return Event{Kind: EvKey, Rune: r}, size, true
}
