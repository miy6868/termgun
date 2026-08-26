package main

import (
	"math"
	"testing"
)

// camTest runs a scripted stretch of play and reports, for each frame, whether
// the world scrolled by at least one cell.
func camTest(t *testing.T, zoom, frames int, step func(g *Game, i int)) (shifts int, gaps []int) {
	t.Helper()
	g := newGameWithInput(7, InputKitty)
	g.zoom = zoom
	g.enemies = g.enemies[:0]
	s := newTestScreen(120, 40)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{8, 2})
	for i := 0; i < 20; i++ { // let the camera settle
		g.Update(1.0 / 60)
		g.Draw(s)
	}

	cell := func() (int, int) {
		z := float64(zoom)
		return int(math.Floor(g.camX * z)), int(math.Floor(g.camY * z))
	}
	px, py := cell()
	last := -1
	for i := 0; i < frames; i++ {
		step(g, i)
		g.Update(1.0 / 60)
		g.Draw(s)
		x, y := cell()
		if x != px || y != py {
			shifts++
			if last >= 0 {
				gaps = append(gaps, i-last)
			}
			last = i
		}
		px, py = x, y
	}
	return shifts, gaps
}

// TestAimingDoesNotScrollTheWorld is the big one for how the game feels. The
// camera used to lead towards the cursor, which meant that simply sweeping the
// mouse around dragged the entire playfield sideways — and since the player is
// aiming with the mouse the whole time, the world was sliding underneath them
// constantly. Standing still must mean a completely still screen.
func TestAimingDoesNotScrollTheWorld(t *testing.T) {
	shifts, _ := camTest(t, 2, 600, func(g *Game, i int) {
		a := float64(i) * 0.05
		g.aim = g.player.pos.Add(Vec{math.Cos(a) * 12, math.Sin(a) * 6})
	})
	if shifts != 0 {
		t.Errorf("the world scrolled on %d of 600 frames while the player stood still "+
			"and only moved the mouse; it should not move at all", shifts)
	}
}

// TestDodgingDoesNotScrollTheWorld covers the dead zone. Short movements back
// and forth — which is what dodging is — must not shift the world, or every
// dodge fights a moving background.
func TestDodgingDoesNotScrollTheWorld(t *testing.T) {
	for _, zoom := range []int{1, 2, 3} {
		shifts, _ := camTest(t, zoom, 600, func(g *Game, i int) {
			d := dirRight
			if (i/12)%2 == 1 {
				d = dirLeft
			}
			g.move.press(d, g.elapsed, false)
		})
		if shifts != 0 {
			t.Errorf("zoom x%d: the world scrolled on %d of 600 frames while the player "+
				"dodged inside the dead zone", zoom, shifts)
		}
	}
}

// TestSustainedScrollIsEvenlyPaced is what "smooth" means in a terminal. The
// screen can only ever scroll a whole cell, so the eye does not judge the size
// of each jump — it judges the rhythm. Walking at a constant speed must produce
// evenly spaced scroll steps; easing the camera made it accelerate and spread
// them unevenly, which is what reads as stutter.
func TestSustainedScrollIsEvenlyPaced(t *testing.T) {
	for _, zoom := range []int{2, 3} {
		_, gaps := camTest(t, zoom, 240, func(g *Game, i int) {
			g.move.press(dirRight, g.elapsed, false)
		})
		if len(gaps) < 10 {
			t.Fatalf("zoom x%d: only %d scroll steps, too few to judge pacing", zoom, len(gaps))
		}
		lo, hi := gaps[0], gaps[0]
		for _, d := range gaps {
			if d < lo {
				lo = d
			}
			if d > hi {
				hi = d
			}
		}
		// A constant speed rarely divides evenly into frames, so neighbouring
		// gaps alternating by one frame is the best achievable. Anything wider
		// means the camera is speeding up and slowing down.
		if hi-lo > 1 {
			t.Errorf("zoom x%d: scroll steps are %d-%d frames apart (%v); "+
				"the pacing must be even", zoom, lo, hi, gaps)
		}
	}
}

// TestCameraKeepsPlayerOnScreen is the limit on how large the dead zone may be.
func TestCameraKeepsPlayerOnScreen(t *testing.T) {
	for _, zoom := range []int{1, 2, 3, 4} {
		g := newGameWithInput(12, InputKitty)
		g.zoom = zoom
		s := newTestScreen(120, 40)
		g.mouseSet = false
		for i := 0; i < 900; i++ {
			d := []int{dirRight, dirDown, dirLeft, dirUp}[(i/60)%4]
			g.move.press(d, g.elapsed, false)
			g.Update(1.0 / 60)
			g.Draw(s)

			x, y := g.worldToScreen(g.player.pos)
			if x < g.viewX || x >= g.viewX+g.viewW || y < g.viewY || y >= g.viewY+g.viewH {
				t.Fatalf("zoom x%d frame %d: player drawn at (%d,%d), outside the "+
					"playfield x[%d,%d) y[%d,%d)",
					zoom, i, x, y, g.viewX, g.viewX+g.viewW, g.viewY, g.viewY+g.viewH)
			}
		}
	}
}

// TestShakeDoesNotMoveTheCamera checks the screen shake is a separate offset.
// Folding it into the camera let a single explosion shove the camera out of its
// dead zone, so the world kept drifting after the shake had finished.
func TestShakeDoesNotMoveTheCamera(t *testing.T) {
	g := newGameWithInput(9, InputKitty)
	g.zoom = 2
	s := newTestScreen(120, 40)
	g.mouseSet = false
	for i := 0; i < 30; i++ {
		g.Update(1.0 / 60)
		g.Draw(s)
	}
	camX, camY := g.camX, g.camY

	g.shake = 0.5
	shook := false
	for i := 0; i < 30; i++ {
		g.Draw(s)
		if g.shakeX != 0 || g.shakeY != 0 {
			shook = true
		}
		if g.camX != camX || g.camY != camY {
			t.Fatalf("the shake moved the camera to %.3f,%.3f from %.3f,%.3f",
				g.camX, g.camY, camX, camY)
		}
	}
	if !shook {
		t.Error("a shake of 0.5 never offset the screen")
	}
}

func TestScreenShakeCanBeDisabled(t *testing.T) {
	g := newGameWithInput(10, InputKitty)
	g.screenShake = false
	g.shake = 1
	s := newTestScreen(120, 40)
	for i := 0; i < 20; i++ {
		g.Draw(s)
		if g.shakeX != 0 || g.shakeY != 0 {
			t.Fatalf("disabled screen shake offset the view by %d,%d", g.shakeX, g.shakeY)
		}
	}
}
