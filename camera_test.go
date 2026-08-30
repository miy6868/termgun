package main

import (
	"math"
	"strings"
	"testing"
)

// camTest runs a scripted stretch of play and reports, for each frame, whether
// the world scrolled by at least one cell.
func camTest(t *testing.T, zoom, frames int, step func(g *Game, i int)) (shifts int, gaps []int) {
	t.Helper()
	g := newGameWithInput(7, InputKitty)
	g.zoom = zoom
	g.enemies = g.enemies[:0]
	// Camera cadence must not depend on which way a generated spawn room opens.
	// Give this camera-only fixture a long, explicit runway across the map.
	runwayY := g.level.H / 2
	for x := 1; x < g.level.W-1; x++ {
		g.level.tiles[g.level.idx(x, runwayY)] = TileFloor
		g.level.tiles[g.level.idx(x, runwayY+1)] = TileFloor
	}
	g.level.invalidateTerrain()
	g.player.pos = Vec{2.5, float64(runwayY) + 0.5}
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

func TestPlayerDoesNotJitterWhileCameraTracks(t *testing.T) {
	for _, zoom := range []int{1, 2, 3, 4} {
		for _, axis := range []string{"horizontal", "vertical"} {
			g := newGameWithInput(17, InputKitty)
			g.zoom = zoom
			g.enemies = g.enemies[:0]
			g.mouseSet = false
			s := newTestScreen(60*zoom, 20*zoom+hudTop+hudBottom)
			g.player.pos = Vec{float64(g.level.W) / 2, float64(g.level.H) / 2}
			g.Draw(s)

			lastCamX, lastCamY := g.camCell()
			anchor, tracked := -1, 0
			for i := 0; i < 160; i++ {
				if axis == "horizontal" {
					g.player.pos.X += 0.10
				} else {
					g.player.pos.Y += 0.10
				}
				g.Draw(s)
				camX, camY := g.camCell()
				if camX == lastCamX && camY == lastCamY {
					continue
				}
				px, py := g.worldToScreen(g.player.pos)
				position := px
				if axis == "vertical" {
					position = py
				}
				if anchor < 0 {
					anchor = position
				} else if position != anchor {
					t.Fatalf("zoom x%d %s tracking jittered between screen cells %d and %d",
						zoom, axis, anchor, position)
				}
				tracked++
				lastCamX, lastCamY = camX, camY
			}
			if tracked < 4 {
				t.Fatalf("zoom x%d %s fixture produced only %d tracked cells", zoom, axis, tracked)
			}
		}
	}
}

func TestAddingDiagonalAxisKeepsTrackedPlayerAnchored(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second int
	}{
		{"W then D", dirUp, dirRight},
		{"D then W", dirRight, dirUp},
		{"S then A", dirDown, dirLeft},
		{"A then S", dirLeft, dirDown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGameWithInput(19, InputKitty)
			rows := make([]string, 80)
			for i := range rows {
				rows[i] = strings.Repeat(".", 120)
			}
			g.level = testLevel(rows)
			g.player.pos = Vec{60.5, 40.5}
			g.enemies = g.enemies[:0]
			g.mouseSet = false
			g.zoom = 2
			s := newTestScreen(60, 24)
			g.Draw(s)
			g.camX = g.player.pos.X - float64(g.tilesW)/2
			g.camY = g.player.pos.Y - float64(g.tilesH)/2
			g.Draw(s)

			startCamX, startCamY := g.camCell()
			g.move.press(tc.first, g.elapsed, false)
			for i := 0; i < 90; i++ {
				g.Update(1.0 / 60)
				g.Draw(s)
			}
			beforeCamX, beforeCamY := g.camCell()
			if beforeCamX == startCamX && beforeCamY == startCamY {
				t.Fatal("fixture did not start camera tracking")
			}
			anchorX, anchorY := g.worldToScreen(g.player.pos)

			// Once the first axis owns the camera, the second must join the same
			// scroll instead of sending the player across another dead zone.
			g.move.press(tc.second, g.elapsed, false)
			for i := 0; i < 60; i++ {
				g.Update(1.0 / 60)
				g.Draw(s)
				px, py := g.worldToScreen(g.player.pos)
				if px != anchorX || py != anchorY {
					t.Fatalf("staged diagonal moved the tracked player from (%d,%d) "+
						"to (%d,%d) at frame %d", anchorX, anchorY, px, py, i)
				}
			}
		})
	}
}

// At a level edge the bounds clamp can leave the player outside the ordinary
// camera dead zone. If a staged diagonal copied that edge position onto its new
// axis, the camera followed the player while preserving an invalid anchor and
// then snapped back to the level edge as soon as movement stopped.
func TestAddingDiagonalAxisAtLevelEdgeDoesNotSnapOnStop(t *testing.T) {
	g := newGameWithInput(21, InputKitty)
	rows := make([]string, 80)
	for i := range rows {
		rows[i] = strings.Repeat(".", 120)
	}
	g.level = testLevel(rows)
	g.player.pos = Vec{118.5, 40.5}
	g.enemies = g.enemies[:0]
	g.mouseSet = false
	g.zoom = 2
	s := newTestScreen(60, 24)
	g.Draw(s)

	// Start against the right edge, then make the vertical axis own the camera.
	g.camX = float64(g.level.W - g.tilesW)
	g.camY = g.player.pos.Y - float64(g.tilesH)/2
	g.Draw(s)
	g.move.press(dirDown, g.elapsed, false)
	for i := 0; i < 30; i++ {
		g.player.pos.Y += 0.1
		g.Draw(s)
	}
	if g.camLockY == 0 {
		t.Fatal("fixture did not start vertical camera tracking")
	}

	// Add movement away from the edge. That axis must cross back into the dead
	// zone as player motion; dragging the camera with it stores a large correction
	// which is otherwise applied in one frame when the keys are released.
	g.move.press(dirLeft, g.elapsed, false)
	for i := 0; i < 40; i++ {
		g.player.pos.X -= 0.1
		g.player.pos.Y += 0.1
		g.Draw(s)
	}
	g.move.releaseAll()
	beforeX, beforeY := g.camCell()
	g.Draw(s)
	afterX, afterY := g.camCell()
	if afterX != beforeX || afterY != beforeY {
		t.Fatalf("camera snapped on stop at the level edge: (%d,%d)->(%d,%d)",
			beforeX, beforeY, afterX, afterY)
	}
}

func TestCameraNeverJumpsDuringInputTransitions(t *testing.T) {
	g := newGameWithInput(23, InputKitty)
	rows := make([]string, 80)
	for i := range rows {
		rows[i] = strings.Repeat(".", 120)
	}
	g.level = testLevel(rows)
	g.player.pos = Vec{60.5, 40.5}
	g.enemies = g.enemies[:0]
	g.mouseSet = false
	g.zoom = 2
	s := newTestScreen(60, 24)
	g.Draw(s)
	g.camX = g.player.pos.X - float64(g.tilesW)/2
	g.camY = g.player.pos.Y - float64(g.tilesH)/2
	g.Draw(s)

	prevPlayerX := int(math.Floor(g.player.pos.X * float64(g.zoomEff)))
	prevPlayerY := int(math.Floor(g.player.pos.Y * float64(g.zoomEff)))
	prevCamX, prevCamY := g.camCell()
	for frame := 0; frame < 1800; frame++ {
		if frame%73 == 0 {
			g.move.releaseAll()
			first := (frame / 73) % numDirs
			g.move.press(first, g.elapsed, false)
			if frame%146 != 0 {
				g.move.press((first+1)%numDirs, g.elapsed, false)
			}
		}
		dt := 1.0 / 60
		if frame%211 == 0 {
			dt = 0.1
		}
		g.Update(dt)
		g.Draw(s)

		playerX := int(math.Floor(g.player.pos.X * float64(g.zoomEff)))
		playerY := int(math.Floor(g.player.pos.Y * float64(g.zoomEff)))
		camX, camY := g.camCell()
		if absInt(camX-prevCamX) > absInt(playerX-prevPlayerX) ||
			absInt(camY-prevCamY) > absInt(playerY-prevPlayerY) {
			t.Fatalf("frame %d: camera jumped (%d,%d)->(%d,%d) while player cells moved "+
				"(%d,%d)->(%d,%d)", frame, prevCamX, prevCamY, camX, camY,
				prevPlayerX, prevPlayerY, playerX, playerY)
		}
		prevPlayerX, prevPlayerY = playerX, playerY
		prevCamX, prevCamY = camX, camY
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
