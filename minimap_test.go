package main

import "testing"

func TestMinimapKeepsPlayerVisibleAtMapEdge(t *testing.T) {
	g := NewGame(1)
	g.level = GenerateLevel(20, g.rng)
	g.showMap = true
	s := newTestScreen(60, 20)

	for _, pos := range []Vec{
		{1, 1},
		{float64(g.level.W - 2), 1},
		{1, float64(g.level.H - 2)},
		{float64(g.level.W - 2), float64(g.level.H - 2)},
	} {
		g.player.pos = pos
		g.Draw(s)

		mw := minInt(g.level.W/2, s.W/3)
		mh := minInt(g.level.H/2, g.viewH-2)
		ox, oy := s.W-mw-2, hudTop+1
		found := false
		for y := oy; y < oy+mh; y++ {
			for x := ox; x < ox+mw; x++ {
				found = found || s.cur[y*s.W+x].R == '@'
			}
		}
		if !found {
			t.Fatalf("player marker at %v is outside the minimap viewport", pos)
		}
	}
}
