package main

import "testing"

// TestBotCanProgress plays whole runs with a crude bot: walk the flow field
// towards the stairs, shoot whatever is nearest, descend on arrival. It ignores
// every defensive mechanic in the game — no dashing, no dodging telegraphs, no
// backing off — so it is a floor on difficulty, not a ceiling. What it is
// really guarding is that a full run never deadlocks: an ambush that cannot be
// cleared, a floor with no reachable exit, or a crash in the new systems would
// all show up here rather than in a player's session.
func TestBotCanProgress(t *testing.T) {
	depths := map[int]int{}
	deepest := 0
	for seed := int64(1); seed <= 25; seed++ {
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
			g.Update(1.0 / 60)
			if vdist(g.player.pos, g.stairsPos) < 2.0 && !g.ambushOn {
				g.tryStairs()
			}
			if g.depth > deepest {
				deepest = g.depth
			}
		}
		depths[g.depth]++
	}
	if deepest < 4 {
		t.Errorf("the deepest of 25 bot runs was only B%d (%v); the floors may be "+
			"unwinnable or a run may be getting stuck", deepest, depths)
	}
	past := 0
	for d, n := range depths {
		if d >= 3 {
			past += n
		}
	}
	if past < 5 {
		t.Errorf("only %d of 25 runs got past B2 (%v)", past, depths)
	}
	t.Logf("deepest B%d, depth reached: %v", deepest, depths)
}
