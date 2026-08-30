package main

import (
	"strings"
	"testing"
)

func minimapRect(g *Game, s *Screen) (ox, oy, mw, mh int) {
	mw = minInt(g.level.W/2, s.W/3)
	mh = minInt(g.level.H/2, g.viewH-2)
	return s.W - mw - 2, hudTop + 1, mw, mh
}

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

		ox, oy, mw, mh := minimapRect(g, s)
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

func TestMinimapAggregatesOddWorldCells(t *testing.T) {
	g := NewGame(2)
	g.showMap = true
	s := newTestScreen(80, 30)

	oldX, oldY := int(g.stairsPos.X), int(g.stairsPos.Y)
	g.level.tiles[g.level.idx(oldX, oldY)] = TileFloor
	for i := range g.level.explored {
		g.level.explored[i] = false
	}
	// Both coordinates are odd, which the old upper-left sampler skipped.
	g.level.tiles[g.level.idx(3, 3)] = TileStairs
	g.level.explored[g.level.idx(3, 3)] = true
	g.stairsPos = Vec{3.5, 3.5}
	g.player.pos = Vec{6.5, 6.5}
	g.Draw(s)

	ox, oy, mw, mh := minimapRect(g, s)
	found := false
	for y := oy; y < oy+mh; y++ {
		for x := ox; x < ox+mw; x++ {
			found = found || s.cur[y*s.W+x].R == '>'
		}
	}
	if !found {
		t.Fatal("minimap dropped stairs placed in the odd cell of a 2x2 block")
	}
}

func TestMinimapShowsExploredLandmarksAndAct(t *testing.T) {
	g := NewGame(3)
	g.enterDepth(6)
	g.showMap = true
	g.enemies, g.pickups = nil, nil
	for i := range g.level.explored {
		g.level.explored[i] = true
	}
	s := newTestScreen(240, 80)
	g.Draw(s)

	ox, oy, mw, mh := minimapRect(g, s)
	if top := screenRow(s, oy-1); !strings.Contains(top, actName(g.depth)) {
		t.Fatalf("minimap border has no act name: %q", top)
	}
	wants := map[rune]bool{'t': false, '!': false, '$': false}
	for y := oy; y < oy+mh; y++ {
		for x := ox; x < ox+mw; x++ {
			if _, ok := wants[s.cur[y*s.W+x].R]; ok {
				wants[s.cur[y*s.W+x].R] = true
			}
		}
	}
	for marker, found := range wants {
		if !found {
			t.Errorf("explored minimap is missing %q landmark", marker)
		}
	}
}
