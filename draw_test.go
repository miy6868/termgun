package main

import (
	"fmt"
	"testing"
)

// The simulation never let a body into a wall, but the screen showed one there
// anyway — the player standing in a two-tile-thick wall and walking around
// inside it, enemies the same, loot appearing in rock. It was the drawing, not
// the game: the tile grid and the things standing on it were mapped to cells
// separately, and the two disagreed whenever the camera was not on a whole tile.

// TestGlyphsStayInsideTheirOwnTile is the invariant. Everything drawn in the
// playfield derives from one integer camera, so a body in tile t can only ever
// be drawn inside t's own block of cells.
func TestGlyphsStayInsideTheirOwnTile(t *testing.T) {
	g := arena(t, 1)
	g.viewX, g.viewY, g.shakeX, g.shakeY = 0, 0, 0, 0

	for _, z := range []int{1, 2, 3, 4} {
		g.zoomEff = z
		// A whole-tile camera hides the bug entirely, which is why every render
		// test that happened to sit at 0 passed through it.
		for _, cam := range []float64{0, 0.1, 0.3, 0.5, 0.7, 0.9} {
			g.camX, g.camY = cam+12, cam+7
			for _, f := range []float64{0, 0.05, 0.25, 0.5, 0.75, 0.95} {
				const tile = 20.0
				bx, by := g.worldToScreen(Vec{tile, tile})
				ex, ey := g.worldToScreen(Vec{tile + f, tile + f})
				if ex < bx || ex >= bx+z || ey < by || ey >= by+z {
					t.Errorf("zoom %d, camera %.1f: a body at %.2f inside tile %d "+
						"is drawn at cell (%d,%d), outside that tile's block "+
						"(%d..%d, %d..%d)",
						z, cam, tile+f, int(tile), ex, ey,
						bx, bx+z-1, by, by+z-1)
				}
			}
		}
	}
}

// TestAimingHitsWhatIsDrawn keeps the mouse honest: screenToWorld has to be the
// inverse of worldToScreen, or you aim at one cell and shoot at another.
func TestAimingHitsWhatIsDrawn(t *testing.T) {
	g := arena(t, 2)
	g.viewX, g.viewY, g.shakeX, g.shakeY = 0, hudTop, 0, 0
	for _, z := range []int{1, 2, 3} {
		g.zoomEff = z
		for _, cam := range []float64{0, 0.3, 0.7} {
			g.camX, g.camY = cam+12, cam+7
			for cell := 4; cell < 40; cell += 7 {
				w := g.screenToWorld(cell, cell+hudTop)
				gx, gy := g.worldToScreen(w)
				if gx != cell || gy != cell+hudTop {
					t.Errorf("zoom %d camera %.1f: cell (%d,%d) maps to world %v, "+
						"which draws back at (%d,%d)",
						z, cam, cell, cell+hudTop, w, gx, gy)
				}
			}
		}
	}
}

// TestNothingIsDrawnInsideAWall is the test that would have caught this, and the
// reason none of the existing ones did: every soak plays the game without ever
// calling Draw, so a fault that only exists on screen is invisible to them.
// This one plays runs and reads the rendered cells.
func TestNothingIsDrawnInsideAWall(t *testing.T) {
	for _, zoom := range []int{1, 2, 3} {
		bad := map[string]int{}
		for seed := int64(1); seed <= 4; seed++ {
			g := NewGame(seed)
			g.zoom = zoom
			g.mouseSet = true
			s := newTestScreen(100, 34)

			for frame := 0; frame < 60*250 && g.depth < 5; frame++ {
				switch g.state {
				case StateLevelUp, StateWeaponCore:
					g.handleKey(Event{Kind: EvKey, Rune: '1'})
					continue
				case StateDead:
					frame = 60 * 250
					continue
				}
				best, bd := -1, 1e9
				for i := range g.enemies {
					if d := vdist(g.enemies[i].pos, g.player.pos); d < bd {
						best, bd = i, d
					}
				}
				g.firing = best >= 0 && bd < 30
				if best >= 0 {
					g.aim = g.enemies[best].pos
				}
				g.level.BuildFlow(int(g.stairsPos.X), int(g.stairsPos.Y))
				if d, ok := g.level.FlowStep(int(g.player.pos.X), int(g.player.pos.Y)); ok {
					g.move.releaseAll()
					if d.X > 0.3 {
						g.move.press(dirRight, g.elapsed, false)
					} else if d.X < -0.3 {
						g.move.press(dirLeft, g.elapsed, false)
					}
					if d.Y > 0.3 {
						g.move.press(dirDown, g.elapsed, false)
					} else if d.Y < -0.3 {
						g.move.press(dirUp, g.elapsed, false)
					}
				}
				g.Update(1.0 / 60)
				g.Draw(s)

				// Read the screen back: whatever cell the player was drawn in
				// must not be showing wall.
				check := func(who string, p Vec) {
					x, y := g.worldToScreen(p)
					if x < 0 || y < hudTop || x >= s.W || y >= s.H-hudBottom {
						return // off the playfield
					}
					w := g.screenToWorld(x, y)
					if g.level.Solid(int(w.X), int(w.Y)) {
						bad[fmt.Sprintf("%s drawn on %v", who,
							g.level.At(int(w.X), int(w.Y)))]++
					}
				}
				check("player", g.player.pos)
				for i := range g.enemies {
					check("enemy "+g.enemies[i].def.Name, g.enemies[i].pos)
				}
				for i := range g.pickups {
					check("loot", g.pickups[i].pos)
				}

				if vdist(g.player.pos, g.stairsPos) < 2.0 && !g.ambushOn {
					g.tryStairs()
				}
			}
		}
		for what, frames := range bad {
			t.Errorf("zoom %d: %d frames with something %s", zoom, frames, what)
		}
	}
}
