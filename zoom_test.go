package main

import (
	"math"
	"testing"
)

func zoomedGame(t *testing.T, zoom int) (*Game, *Screen) {
	t.Helper()
	g := newGameWithInput(21, InputKitty)
	g.zoom = zoom
	s := newTestScreen(120, 40)
	g.Draw(s)
	if g.zoomEff != zoom {
		t.Fatalf("zoom x%d was reduced to x%d on a 120x40 screen", zoom, g.zoomEff)
	}
	return g, s
}

// TestTilesStaySquare is the invariant that makes the zoom safe to change.
// The simulation counts a vertical world unit double (aspect), which is what
// makes one tile look square in a terminal cell twice as tall as it is wide.
// Scaling the axes by different amounts would turn every circle in the game
// into an ellipse, so a tile must always cover an equal number of columns and
// rows.
func TestTilesStaySquare(t *testing.T) {
	for zoom := minZoom; zoom <= maxZoom; zoom++ {
		g, _ := zoomedGame(t, zoom)
		base := g.player.pos
		x0, y0 := g.worldToScreen(base)
		x1, _ := g.worldToScreen(base.Add(Vec{1, 0}))
		_, y1 := g.worldToScreen(base.Add(Vec{0, 1}))

		if cols, rows := x1-x0, y1-y0; cols != rows {
			t.Errorf("zoom x%d: one tile is %d columns but %d rows; tiles must stay square",
				zoom, cols, rows)
		} else if cols != zoom {
			t.Errorf("zoom x%d: one tile spans %d cells, want %d", zoom, cols, zoom)
		}
	}
}

// TestZoomEnlargesTheWorld checks the feature actually does something: at a
// higher zoom the same stretch of dungeon covers more of the screen.
func TestZoomEnlargesTheWorld(t *testing.T) {
	var prev int
	for zoom := minZoom; zoom <= maxZoom; zoom++ {
		g, _ := zoomedGame(t, zoom)
		x0, _ := g.worldToScreen(g.player.pos)
		x1, _ := g.worldToScreen(g.player.pos.Add(Vec{10, 0}))
		span := x1 - x0
		if zoom > minZoom && span <= prev {
			t.Errorf("zoom x%d draws 10 tiles across %d cells, x%d used %d; "+
				"zooming in must make things bigger", zoom, span, zoom-1, prev)
		}
		prev = span
	}
}

// TestWallsFillTheirTile guards the drawing loop. A wall has to cover its whole
// block, or a zoomed-in level shows gaps that look walkable but are not.
func TestWallsFillTheirTile(t *testing.T) {
	for zoom := minZoom; zoom <= maxZoom; zoom++ {
		g := newGameWithInput(21, InputKitty)
		g.zoom = zoom
		g.level = testLevel([]string{
			"##########",
			"#........#",
			"#........#",
			"#........#",
			"##########",
		})
		g.player.pos = Vec{5.5, 2.5}
		g.enemies = g.enemies[:0]
		g.mouseSet = false
		g.level.ComputeFOV(int(g.player.pos.X), int(g.player.pos.Y), 15)
		s := newTestScreen(120, 40)
		// Two frames: the camera eases towards its target, so the first frame is
		// still in transit.
		g.Draw(s)
		g.Draw(s)

		wx, wy := 3, 0 // a wall tile along the top
		x0, y0 := g.worldToScreen(Vec{float64(wx), float64(wy)})
		for dy := 0; dy < zoom; dy++ {
			for dx := 0; dx < zoom; dx++ {
				x, y := x0+dx, y0+dy
				if y < g.viewY || y >= g.viewY+g.viewH {
					continue
				}
				if got := s.cur[y*s.W+x].R; got != '#' {
					t.Fatalf("zoom x%d: wall tile (%d,%d) has %q at offset (%d,%d), want '#'",
						zoom, wx, wy, got, dx, dy)
				}
			}
		}
	}
}

// TestAimingSurvivesZoom is the regression that would ruin the game quietly:
// the mouse reports terminal cells, so if screenToWorld ignores the zoom the
// player aims at a point several tiles away from their cursor.
func TestAimingSurvivesZoom(t *testing.T) {
	for zoom := minZoom; zoom <= maxZoom; zoom++ {
		g, _ := zoomedGame(t, zoom)
		for _, target := range []Vec{
			g.player.pos.Add(Vec{6, 3}),
			g.player.pos.Add(Vec{-9, 2}),
			g.player.pos.Add(Vec{2, -4}),
		} {
			mx, my := g.worldToScreen(target)
			back := g.screenToWorld(mx, my)
			if math.Abs(back.X-target.X) > 1 || math.Abs(back.Y-target.Y) > 1 {
				t.Errorf("zoom x%d: clicking the cell drawn for %v aims at %v",
					zoom, target, back)
			}
		}
	}
}

// TestZoomIsPresentationOnly checks the zoom cannot change the run. Anything
// else would make it a difficulty setting rather than a view setting.
func TestZoomIsPresentationOnly(t *testing.T) {
	run := func(zoom int) (Vec, int, float64) {
		g := newGameWithInput(77, InputKitty)
		g.zoom = zoom
		s := newTestScreen(120, 40)
		g.mouseSet = true
		for step := 0; step < 400; step++ {
			g.aim = g.player.pos.Add(Vec{7, 2})
			g.firing = step%3 == 0
			g.move.press(dirRight, g.elapsed, false)
			g.Update(1.0 / 60)
			g.Draw(s)
		}
		return g.player.pos, g.kills, g.player.hp
	}
	wantPos, wantKills, wantHP := run(minZoom)
	for zoom := minZoom + 1; zoom <= maxZoom; zoom++ {
		pos, kills, hp := run(zoom)
		if pos != wantPos || kills != wantKills || hp != wantHP {
			t.Errorf("zoom x%d ends at %v/%d kills/%.1f hp, x%d ends at %v/%d/%.1f; "+
				"zoom must not affect the simulation",
				zoom, pos, kills, hp, minZoom, wantPos, wantKills, wantHP)
		}
	}
}

// TestSmallTerminalsCapTheZoom stops a high zoom from shrinking the view to a
// keyhole where enemies arrive with no warning.
func TestSmallTerminalsCapTheZoom(t *testing.T) {
	g := newGameWithInput(5, InputKitty)
	g.zoom = maxZoom
	s := newTestScreen(minCols, minRows)
	g.Draw(s)

	if g.tilesW < minTilesW || g.tilesH < minTilesH {
		t.Errorf("at %dx%d the view is only %dx%d tiles, want at least %dx%d",
			minCols, minRows, g.tilesW, g.tilesH, minTilesW, minTilesH)
	}
	if g.zoomEff > g.zoom {
		t.Errorf("effective zoom x%d exceeds the requested x%d", g.zoomEff, g.zoom)
	}
	// The request itself survives, so widening the terminal restores it.
	if g.zoom != maxZoom {
		t.Errorf("the requested zoom was overwritten with x%d", g.zoom)
	}
	big := newTestScreen(200, 60)
	g.Draw(big)
	if g.zoomEff != maxZoom {
		t.Errorf("a large terminal shows x%d, want the requested x%d", g.zoomEff, maxZoom)
	}
}

// TestZoomIsDiscoverable keeps the setting visible. A zoom nobody can find is
// no better than no zoom: the HUD has to show the current level and settings,
// and the help overlay has to explain them.
func TestZoomIsDiscoverable(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		g := NewGame(6)
		s := newTestScreen(w, 30)
		g.Draw(s)
		row := screenRow(s, 1)
		if !contains(row, "배율") || !contains(row, "[O]") {
			t.Errorf("width %d: no zoom readout on the status row: %q", w, row)
		}
		if !contains(row, "x2") {
			t.Errorf("width %d: the current zoom level is not shown: %q", w, row)
		}
	}

	g := NewGame(6)
	s := newTestScreen(120, 40)
	g.handleKey(Event{Kind: EvKey, Rune: '?'})
	g.Draw(s)
	found := false
	for y := 0; y < s.H; y++ {
		if contains(screenRow(s, y), "배율") {
			found = true
		}
	}
	if !found {
		t.Error("the help overlay never mentions the zoom")
	}
}

// TestZoomKeysClamp covers the controls at both ends.
func TestZoomKeysClamp(t *testing.T) {
	g := newGameWithInput(6, InputKitty)
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	for i := 0; i < 4; i++ {
		g.handleKey(Event{Kind: EvKey, Key: KeyDown})
	}
	for i := 0; i < 10; i++ {
		g.handleKey(Event{Kind: EvKey, Key: KeyRight})
	}
	if g.zoom != maxZoom {
		t.Errorf("zooming in repeatedly reached x%d, want x%d", g.zoom, maxZoom)
	}
	for i := 0; i < 10; i++ {
		g.handleKey(Event{Kind: EvKey, Key: KeyLeft})
	}
	if g.zoom != minZoom {
		t.Errorf("zooming out repeatedly reached x%d, want x%d", g.zoom, minZoom)
	}
}

// TestZoomSurvivesRestart keeps a view preference from being silently undone
// every time the player dies.
func TestZoomSurvivesRestart(t *testing.T) {
	g := newGameWithInput(8, InputKitty)
	g.setZoom(maxZoom)
	g.state = StateDead
	g.handleKey(Event{Kind: EvKey, Rune: 'r'})

	if g.state != StatePlaying {
		t.Fatalf("restart left the game in state %v", g.state)
	}
	if g.zoom != maxZoom {
		t.Errorf("restarting reset the zoom to x%d, want the chosen x%d", g.zoom, maxZoom)
	}
}
