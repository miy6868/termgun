package main

import "math"

// Direction slots. Movement is tracked per key rather than as a single vector
// so that holding two directions produces a real diagonal.
const (
	dirUp = iota
	dirLeft
	dirDown
	dirRight
	numDirs
)

const (
	// How long a keypress counts as "still held" when the terminal cannot
	// report releases. It has to outlast the gap before the OS starts
	// auto-repeating, or movement stutters after the first step.
	legacyHold = 0.30
	// Once repeats are arriving we know their period and can expire the key
	// promptly after the last one, so stopping stays crisp.
	legacyRepeatFactor = 3.0
	legacyMinHold      = 0.10

	// Keyboard movement should feel like direct intent, not a heavy body being
	// pushed across the floor. Separate response rates keep starts immediate,
	// stops precise, and reversals decisive without making velocity frame-rate
	// dependent.
	moveStartResponse = 36.0
	moveStopResponse  = 160.0
	moveTurnResponse  = 60.0
)

// movement resolves the keyboard into a direction vector.
//
// Two problems make this less trivial than it looks. Terminals without the
// kitty protocol never report key releases, so a press is only known to be
// "held" for a while afterwards. And pressing left and right at once must not
// cancel to a standstill — the later key wins, which is what every twin-stick
// game does and what a player pressing D while still holding A expects.
type movement struct {
	trueState bool // the terminal reports releases, so held[] is exact

	held   [numDirs]bool
	expiry [numDirs]float64 // legacy mode: when the key stops counting
	lastAt [numDirs]float64 // legacy mode: previous press, to measure repeats
	seq    [numDirs]uint64  // press order, for last-key-wins
	clock  uint64
}

func newMovement(trueState bool) *movement {
	return &movement{trueState: trueState}
}

// press registers a key going down. isRepeat marks an auto-repeat rather than a
// fresh press: a repeat only refreshes the key, because a key held down must not
// steal priority back from one the player pressed after it. Terminals without
// the kitty protocol cannot label repeats, but they also only ever repeat the
// most recently pressed key, so treating everything as a fresh press is right
// there.
func (m *movement) press(d int, now float64, isRepeat bool) {
	if d < 0 {
		return
	}
	if !isRepeat {
		m.clock++
		m.seq[d] = m.clock
	}
	if m.trueState {
		m.held[d] = true
		return
	}
	hold := legacyHold
	if gap := now - m.lastAt[d]; gap > 0 && gap < legacyHold {
		hold = clampF(gap*legacyRepeatFactor, legacyMinHold, legacyHold)
	}
	m.lastAt[d] = now
	m.expiry[d] = now + hold
}

func (m *movement) release(d int) {
	if d < 0 {
		return
	}
	m.held[d] = false
	m.expiry[d] = 0
	m.seq[d] = 0
}

// releaseAll drops every held direction, used when the window loses focus so
// the player does not keep walking while typing somewhere else.
func (m *movement) releaseAll() {
	for d := 0; d < numDirs; d++ {
		m.release(d)
	}
}

func (m *movement) active(d int, now float64) bool {
	if m.trueState {
		return m.held[d]
	}
	return now < m.expiry[d]
}

// axis picks between two opposing directions: whichever was pressed last wins.
func (m *movement) axis(neg, pos int, now float64) float64 {
	n, p := m.active(neg, now), m.active(pos, now)
	switch {
	case n && p:
		if m.seq[neg] > m.seq[pos] {
			return -1
		}
		return 1
	case n:
		return -1
	case p:
		return 1
	}
	return 0
}

// dir is the current movement direction as a unit vector in visual space.
func (m *movement) dir(now float64) Vec {
	v := Vec{
		X: m.axis(dirLeft, dirRight, now),
		Y: m.axis(dirUp, dirDown, now),
	}
	return v.norm()
}

// dirFor maps a key event onto a direction slot, or -1 if it is not a
// movement key.
func dirFor(r rune, k Key) int {
	switch {
	case r == 'w' || r == 'W' || k == KeyUp:
		return dirUp
	case r == 's' || r == 'S' || k == KeyDown:
		return dirDown
	case r == 'a' || r == 'A' || k == KeyLeft:
		return dirLeft
	case r == 'd' || r == 'D' || k == KeyRight:
		return dirRight
	}
	return -1
}

// steerMovementVelocity grades each axis separately. Releasing W while still
// holding D must brake the vertical component immediately; grading the vector
// as a whole would mistake that diagonal-to-horizontal change for acceleration.
func steerMovementVelocity(current, target Vec, dt float64) Vec {
	axis := func(now, want float64) float64 {
		response := moveStartResponse
		if math.Abs(want) < 1e-9 {
			response = moveStopResponse
		} else if now*want < 0 {
			response = moveTurnResponse
		}
		blend := 1 - math.Exp(-response*dt)
		return now + (want-now)*blend
	}
	return Vec{X: axis(current.X, target.X), Y: axis(current.Y, target.Y)}
}
