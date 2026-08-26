package main

import (
	"fmt"
	"testing"
)

// Walls are two tiles thick, and bodies were ending up a layer inside them —
// enemies parked in the rock still shooting, and the player on the wrong side
// of a wall. There were two separate ways in, and neither was a collision bug:
// one put bodies there at birth, the other stepped straight over the wall.

// TestNothingTunnelsThroughAWall covers the second one. Collision resolves a
// whole frame of movement in one go and only ever tests the destination, so a
// long enough step skips everything in between. main.go caps a stalled frame at
// 0.1s and says in a comment that this stops entities teleporting through
// walls — but 0.1s of dash is 5.1 tiles, which clears a two-tile wall outright.
func TestNothingTunnelsThroughAWall(t *testing.T) {
	dash := func(dt float64) (float64, float64) {
		g := arena(t, 1)
		for _, x := range []int{20, 21} { // a two-tile-thick wall
			for y := 0; y < g.level.H; y++ {
				g.level.tiles[g.level.idx(x, y)] = TileWall
			}
		}
		g.player.pos = Vec{18.5, 4.5}
		step := Vec{1, 0}.Scale(g.player.baseSpeed() * 3.4).unvisual().Scale(dt)
		g.moveWithCollision(&g.player.pos, step, g.player.radius)
		return step.X, g.player.pos.X
	}

	// The worst frame the game will ever hand to Update.
	asked, got := dash(0.1)
	if got >= 20 {
		t.Errorf("a %.1f tile dash step put the player at x=%.2f, past a wall "+
			"spanning x=20..21", asked, got)
	}
	if g := arena(t, 1); g.level.Solid(int(got), 4) {
		t.Errorf("the player came to rest at x=%.2f, inside a wall", got)
	}
}

// TestSpawnsLandOnFloor covers the first way in. Plenty of code picks a spawn
// point by arithmetic — a splitting elite dropping fragments beside itself, a
// boss calling swarmlings onto a ring, an ambush scattering a wave across a
// room — and any of those can land in rock. A body born inside a wall is stuck
// forever, because from there every direction it tries is blocked too.
func TestSpawnsLandOnFloor(t *testing.T) {
	g := arena(t, 2)
	wall := Vec{0.5, 0.5} // the map border, solid
	if !g.level.SolidAtPoint(wall) {
		t.Fatal("test setup: expected the border to be solid")
	}
	g.addEnemy(enemyDefs[0], wall)
	if g.level.SolidAtPoint(g.enemies[len(g.enemies)-1].pos) {
		t.Error("an enemy spawned into solid rock and can never move out of it")
	}
}

// TestNearestFreeStaysPut keeps the correction from moving spawns that were
// already fine, which would quietly undo every deliberate placement.
func TestNearestFreeStaysPut(t *testing.T) {
	g := arena(t, 3)
	for _, p := range []Vec{{16.5, 4.5}, {17.25, 5.75}, {20.5, 6.5}} {
		if g.level.SolidAtPoint(p) {
			continue
		}
		if got := g.level.NearestFree(p); got != p {
			t.Errorf("NearestFree moved %v to %v; it was already on floor", p, got)
		}
	}
}

// TestAmbushDoesNotSealSomeoneIntoADoor is the third route in, and the only one
// where the body never moved: the doors close over tiles that are being stood
// on, and a door is solid.
func TestAmbushDoesNotSealSomeoneIntoADoor(t *testing.T) {
	g := NewGame(4)
	g.enterDepth(3)
	var room Rect
	var doors [][2]int
	for i := range g.level.rooms {
		if d := g.level.Openings(g.level.rooms[i]); len(d) > 0 {
			g.ambushRoom, room, doors = i, g.level.rooms[i], d
			break
		}
	}
	if len(doors) == 0 {
		t.Fatal("no room on this floor has an opening to seal")
	}
	// Park an enemy right in the doorway, mid-chase.
	g.addEnemy(enemyDefs[0], Vec{float64(doors[0][0]) + 0.5, float64(doors[0][1]) + 0.5})
	cx, cy := room.center()
	g.player.pos = Vec{float64(cx) + 0.5, float64(cy) + 0.5}

	g.startAmbush(room)

	for i := range g.enemies {
		if g.level.SolidAtPoint(g.enemies[i].pos) {
			t.Errorf("%s was sealed inside a door tile at %v",
				g.enemies[i].def.Name, g.enemies[i].pos)
		}
	}
	if g.level.SolidAtPoint(g.player.pos) {
		t.Errorf("the player was sealed inside a door tile at %v", g.player.pos)
	}
}

// TestNobodyEndsUpInsideAWall is the one that actually found all of this: play
// whole runs and check every body, every frame. Unit tests only catch the ways
// in that somebody already thought of.
func TestNobodyEndsUpInsideAWall(t *testing.T) {
	bad := map[string]int{}
	for seed := int64(1); seed <= 12; seed++ {
		g := NewGame(seed)
		g.mouseSet = true
		for frame := 0; frame < 60*400 && g.depth < 8; frame++ {
			switch g.state {
			case StateLevelUp:
				g.handleKey(Event{Kind: EvKey, Rune: '1'})
				continue
			case StateDead:
				frame = 60 * 400
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
			g.startDash() // dash constantly: the longest steps the game ever takes
			// Stall every so often. A steady 1/60 never produces a step long
			// enough to skip a wall, so a soak that only ever runs at a clean
			// frame rate cannot find the bug this test exists for. 0.1s is the
			// worst frame main.go will pass through.
			dt := 1.0 / 60
			if frame%97 == 0 {
				dt = 0.1
			}
			g.Update(dt)

			if g.level.SolidAtPoint(g.player.pos) {
				bad[fmt.Sprintf("player in %v", g.level.At(
					int(g.player.pos.X), int(g.player.pos.Y)))]++
			}
			for i := range g.enemies {
				if p := g.enemies[i].pos; g.level.SolidAtPoint(p) {
					bad[fmt.Sprintf("%s in %v", g.enemies[i].def.Name,
						g.level.At(int(p.X), int(p.Y)))]++
				}
			}
			if vdist(g.player.pos, g.stairsPos) < 2.0 && !g.ambushOn {
				g.tryStairs()
			}
		}
	}
	for what, frames := range bad {
		t.Errorf("%d frames with a body inside a wall: %s", frames, what)
	}
}
