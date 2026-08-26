package main

import (
	"bufio"
	"io"
	"testing"
)

const benchmarkRunFrames = 3000

// botFrame drives one frame the way soak_test's bot does — walk the flow field
// towards the stairs, shoot whatever is nearest, descend on arrival — so the
// benchmarks below measure the same code path a real session runs, with a live
// dungeon, enemies and bullets rather than an empty level. fov_test.go uses it
// too, for the same reason: cache bugs only show up under real movement.
func botFrame(g *Game) {
	switch g.state {
	case StateLevelUp:
		g.handleKey(Event{Kind: EvKey, Rune: '1'})
		return
	case StateDead:
		return
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
	// Aim the flow field at the stairs so the bot actually descends. Without
	// this it walks the field the game built towards the player, never gets
	// anywhere, and never reaches an ambush or a new floor — which is most of
	// what makes these runs worth measuring against.
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
}

// benchGame keeps a run bounded and repeatable instead of letting benchmark
// calibration descend forever into increasingly expensive floors.
func benchGame(g *Game, i int) *Game {
	if g == nil || g.state == StateDead || i%benchmarkRunFrames == 0 {
		g = NewGame(42)
		g.mouseSet = true
	}
	return g
}

// BenchmarkFrame is the whole per-frame budget: simulation, draw, and the diff
// the renderer writes out. At 60 fps a frame has 16.7ms, so this is the number
// to check before and after anything that touches the hot path.
func BenchmarkFrame(b *testing.B) {
	var g *Game
	s := NewScreen(120, 40, bufio.NewWriter(io.Discard))
	for i := 0; i < b.N; i++ {
		g = benchGame(g, i)
		botFrame(g)
		g.Draw(s)
		s.Flush()
	}
}

// BenchmarkUpdate isolates the simulation: movement, collision, AI, pathing.
func BenchmarkUpdate(b *testing.B) {
	var g *Game
	for i := 0; i < b.N; i++ {
		g = benchGame(g, i)
		botFrame(g)
	}
}

// BenchmarkDraw isolates rendering, including the HUD, which is the part that
// historically allocated on every frame.
func BenchmarkDraw(b *testing.B) {
	g := benchGame(nil, 0)
	s := NewScreen(120, 40, bufio.NewWriter(io.Discard))
	for i := 0; i < 200; i++ {
		botFrame(g)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Draw(s)
	}
}
