package main

import "testing"

// canMove reports how many of the four directions actually move a body.
func canMove(g *Game, pos Vec, radius float64) int {
	n := 0
	for _, d := range []Vec{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		p := pos
		g.moveWithCollision(&p, d.unvisual().Scale(0.25), radius)
		if p.Sub(pos).visual().len() > 0.01 {
			n++
		}
	}
	return n
}

// TestAmbushDoesNotWedgeYouInTheDoorway is the one you actually feel. Walking
// into an ambush room shuts the doors behind you, and the trigger fires the
// moment your centre crosses the threshold — while your shoulder is still in
// the doorway. Standing on floor is not the same as being able to move: the body
// is wider than a point, so a door closing across it refuses most directions and
// reads as being caught in the door.
//
// Being sealed *into* a door tile is the worse version of the same thing and is
// covered by TestAmbushDoesNotSealSomeoneIntoADoor; this is the near miss, which
// is far more common because you have to walk through the gap to trigger it.
func TestAmbushDoesNotWedgeYouInTheDoorway(t *testing.T) {
	checked := 0
	for seed := int64(1); seed <= 60; seed++ {
		g := NewGame(seed)
		g.enterDepth(3)
		if g.ambushRoom < 0 {
			continue
		}
		room := g.level.rooms[g.ambushRoom]
		doors := g.level.Openings(room)
		if len(doors) == 0 {
			continue
		}
		d := doors[0]

		// Stand just across the threshold, the way you arrive walking in.
		var inside Vec
		switch {
		case d[0] < room.X:
			inside = Vec{float64(room.X) + 0.05, float64(d[1]) + 0.5}
		case d[0] >= room.X+room.W:
			inside = Vec{float64(room.X+room.W) - 0.05, float64(d[1]) + 0.5}
		case d[1] < room.Y:
			inside = Vec{float64(d[0]) + 0.5, float64(room.Y) + 0.05}
		default:
			inside = Vec{float64(d[0]) + 0.5, float64(room.Y+room.H) - 0.05}
		}
		if g.level.SolidAtPoint(inside) {
			continue
		}
		g.player.pos = inside
		g.updateAmbush(1.0 / 60)
		if !g.ambushOn {
			continue
		}
		checked++

		rx := minF(g.player.radius, maxCollisionRadius)
		if g.blocked(g.player.pos.X, g.player.pos.Y, rx, rx/aspect) {
			t.Errorf("seed %d: sealed at %v with the body still overlapping a "+
				"solid tile", seed, g.player.pos)
		}
		// Three, not four: the door you came through is shut, and should be.
		if n := canMove(g, g.player.pos, g.player.radius); n < 3 {
			t.Errorf("seed %d: sealed at %v and only %d of 4 directions move; "+
				"the player is wedged in the doorway", seed, g.player.pos, n)
		}
		for i := range g.enemies {
			e := &g.enemies[i]
			er := minF(e.def.Radius, maxCollisionRadius)
			if g.blocked(e.pos.X, e.pos.Y, er, er/aspect) {
				t.Errorf("seed %d: %s sealed at %v overlapping a solid tile",
					seed, e.def.Name, e.pos)
			}
		}
	}
	if checked < 20 {
		t.Fatalf("only %d seeds produced a sealed ambush; the test is not "+
			"exercising what it claims", checked)
	}
	t.Logf("checked %d sealed ambushes", checked)
}

// TestArrivingOnAFloorLeavesYouFree covers the plain reading of the same
// complaint: whatever a new floor drops you onto, you have to be able to walk
// off it in every direction.
func TestArrivingOnAFloorLeavesYouFree(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		g := NewGame(seed)
		for depth := 1; depth <= 6; depth++ {
			g.enterDepth(depth)
			rx := minF(g.player.radius, maxCollisionRadius)
			if g.blocked(g.player.pos.X, g.player.pos.Y, rx, rx/aspect) {
				t.Fatalf("seed %d B%d: the run starts at %v with the body "+
					"overlapping a wall", seed, depth, g.player.pos)
			}
			if n := canMove(g, g.player.pos, g.player.radius); n < 4 {
				t.Fatalf("seed %d B%d: only %d of 4 directions move on arrival",
					seed, depth, n)
			}
		}
	}
}
