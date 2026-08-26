package main

import (
	"bufio"
	"io"
	"math"
	"math/rand"
	"testing"
)

func newTestScreen(w, h int) *Screen {
	return NewScreen(w, h, bufio.NewWriter(io.Discard))
}

// TestSimulateRun drives a full run headlessly: the player moves, shoots,
// descends floors and takes damage, and nothing may panic or leak out of the
// map along the way.
func TestSimulateRun(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		g := NewGame(seed)
		s := newTestScreen(120, 40)
		g.Draw(s) // establishes the viewport used by screenToWorld

		const dt = 1.0 / 60
		for frame := 0; frame < 60*45; frame++ {
			// Wander, and always shoot at the nearest visible enemy.
			if frame%7 == 0 {
				switch g.rng.Intn(4) {
				case 0:
					g.pressMove('w', KeyNone)
				case 1:
					g.pressMove('a', KeyNone)
				case 2:
					g.pressMove('s', KeyNone)
				case 3:
					g.pressMove('d', KeyNone)
				}
			}
			if e := nearestEnemy(g); e != nil {
				g.aim = e.pos
				g.mouseSet = true
				g.firing = true
			} else {
				g.firing = false
			}
			if frame%600 == 0 {
				g.startDash()
			}

			g.Update(dt)
			if frame%3 == 0 {
				g.Draw(s)
				s.Flush()
			}

			// Resolve level-up menus so the run keeps going.
			if g.state == StateLevelUp {
				g.handleKey(Event{Kind: EvKey, Rune: '1'})
			}
			if g.state == StateDead {
				break
			}

			// Invariants.
			p := g.player.pos
			if math.IsNaN(p.X) || math.IsNaN(p.Y) {
				t.Fatalf("seed %d frame %d: player position became NaN", seed, frame)
			}
			if g.level.SolidAtPoint(p) {
				t.Fatalf("seed %d frame %d: player at %v is inside a wall", seed, frame, p)
			}
			for i := range g.enemies {
				if g.level.SolidAtPoint(g.enemies[i].pos) {
					t.Fatalf("seed %d frame %d: %s escaped into a wall at %v",
						seed, frame, g.enemies[i].def.Name, g.enemies[i].pos)
				}
			}

			// Take the stairs whenever we happen to be near them.
			if vdist(g.player.pos, g.stairsPos) < 2.0 {
				g.tryStairs()
			}
		}
		t.Logf("seed %d: depth B%d, kills %d, score %d, hp %.0f, state %d",
			seed, g.depth, g.kills, g.score, g.player.hp, g.state)
	}
}

func nearestEnemy(g *Game) *Enemy {
	var best *Enemy
	bestD := 1e9
	for i := range g.enemies {
		e := &g.enemies[i]
		if !g.level.Visible(int(e.pos.X), int(e.pos.Y)) {
			continue
		}
		if d := vdist(e.pos, g.player.pos); d < bestD {
			best, bestD = e, d
		}
	}
	return best
}

// TestCombatKills confirms shooting actually removes an enemy and pays out.
func TestCombatKills(t *testing.T) {
	g := NewGame(42)
	s := newTestScreen(120, 40)
	g.Draw(s)

	g.enemies = g.enemies[:0]
	target := g.player.pos.Add(Vec{6, 0})
	g.addEnemy(enemyDefs[0], target)
	g.aim = target
	g.mouseSet = true
	g.firing = true
	g.player.weapon = wpRailgun

	for i := 0; i < 600 && len(g.enemies) > 0; i++ {
		g.aim = g.enemies[0].pos
		g.Update(1.0 / 60)
	}
	if len(g.enemies) != 0 {
		t.Fatalf("enemy survived sustained railgun fire (hp %.1f)", g.enemies[0].hp)
	}
	if g.kills != 1 || g.score == 0 {
		t.Fatalf("kill was not scored: kills=%d score=%d", g.kills, g.score)
	}
}

// TestDescend walks the player down several floors.
func TestDescend(t *testing.T) {
	g := NewGame(7)
	s := newTestScreen(120, 40)
	g.Draw(s)
	for want := 2; want <= 6; want++ {
		// A living boss seals the stairs, so the floor's owner has to fall first.
		if b := g.bossEnemy(); b != nil {
			out := g.enemies[:0]
			for _, e := range g.enemies {
				if e.def.Behavior != BeBoss {
					out = append(out, e)
				}
			}
			g.enemies = out
		}
		g.player.pos = g.stairsPos
		g.tryStairs()
		if g.depth != want {
			t.Fatalf("expected to reach B%d, got B%d", want, g.depth)
		}
		if g.level.SolidAtPoint(g.player.pos) {
			t.Fatalf("B%d: spawned inside a wall", g.depth)
		}
		if len(g.enemies) == 0 {
			t.Fatalf("B%d: floor has no enemies", g.depth)
		}
	}
}

// TestBossBlocksStairs pins the boss-floor gate: the OVERSEER holds the stairs
// shut until it is dead, and its death opens them again.
func TestBossBlocksStairs(t *testing.T) {
	g := NewGame(7)
	g.enterDepth(5)
	if b := g.bossEnemy(); b == nil {
		t.Fatal("B5 spawned without a living boss")
	}
	g.player.pos = g.stairsPos
	g.tryStairs()
	if g.depth != 5 {
		t.Fatalf("descended past a living boss (depth B%d)", g.depth)
	}

	out := g.enemies[:0]
	for _, e := range g.enemies {
		if e.def.Behavior != BeBoss {
			out = append(out, e)
		}
	}
	g.enemies = out
	g.tryStairs()
	if g.depth != 6 {
		t.Fatalf("boss is dead but the stairs stayed shut (depth B%d)", g.depth)
	}
}

// TestLevelConnectivity verifies every room is reachable from the spawn point,
// so a floor can never strand the player or hide the stairs behind solid rock.
func TestLevelConnectivity(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		g := NewGame(seed)
		for depth := 0; depth < 3; depth++ {
			l := g.level
			px, py := int(g.player.pos.X), int(g.player.pos.Y)
			l.flowSrc = [2]int{-1, -1}
			l.BuildFlow(px, py)
			for _, r := range l.rooms {
				cx, cy := r.center()
				if l.Solid(cx, cy) {
					continue
				}
				if l.flow[l.idx(cx, cy)] >= flowUnreachable {
					t.Fatalf("seed %d B%d: room at (%d,%d) is unreachable", seed, l.Depth, cx, cy)
				}
			}
			sx, sy := int(g.stairsPos.X), int(g.stairsPos.Y)
			if l.flow[l.idx(sx, sy)] >= flowUnreachable {
				t.Fatalf("seed %d B%d: stairs are unreachable", seed, l.Depth)
			}
			g.enterDepth(g.depth + 1)
		}
	}
}

// TestDashSteersTowardHeldKeys pins mid-dash steering: holding a direction
// bends a live dash towards it, but only at the rate limit — the dash stays a
// committed blink rather than becoming free flight.
func TestDashSteersTowardHeldKeys(t *testing.T) {
	g := newGameWithInput(3, InputKitty)
	g.pressMove('w', KeyNone)
	g.startDash()
	if g.player.dashDir.Y >= 0 {
		t.Fatalf("dash should launch along the held key, got %v", g.player.dashDir)
	}

	g.move.release(dirUp)
	g.pressMove('d', KeyNone)
	start := math.Atan2(g.player.dashDir.Y, g.player.dashDir.X)
	for i := 0; i < 6; i++ { // 0.1s of dash, still inside the 0.16s window
		g.Update(1.0 / 60)
	}
	if g.player.dashTimer <= 0 {
		t.Fatal("the dash ended before steering could be observed")
	}
	end := math.Atan2(g.player.dashDir.Y, g.player.dashDir.X)
	da := end - start
	if da <= 0 || da > dashSteerRate*0.1+1e-9 {
		t.Fatalf("holding right for 0.1s turned an up-dash by %.3f rad; want (0, %.3f]",
			da, dashSteerRate*0.1)
	}
}

// TestStairsAreNotAlwaysInTheSameCorner pins the exit lottery: the stairs go
// in one of the farthest rooms, chosen at random, so no fixed compass
// direction solves every floor. It also checks the off-center placement — the
// stairs tile must still be a real floor tile inside its room.
func TestStairsAreNotAlwaysInTheSameCorner(t *testing.T) {
	sign := func(v int) int {
		if v < 0 {
			return -1
		}
		if v > 0 {
			return 1
		}
		return 0
	}
	dirs := map[[2]int]int{}
	for seed := int64(1); seed <= 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		l := GenerateLevel(3, rng)
		sx, sy := l.rooms[0].center()
		ex, ey := l.PlaceStairs(sx, sy, rng)
		if l.At(ex, ey) != TileStairs {
			t.Fatalf("seed %d: PlaceStairs left a %v tile, not stairs", seed, l.At(ex, ey))
		}
		if sr := l.RoomAt(Vec{float64(ex) + 0.5, float64(ey) + 0.5}); sr < 0 {
			t.Fatalf("seed %d: the stairs at (%d,%d) are not inside any room", seed, ex, ey)
		}
		dirs[[2]int{sign(ex - sx), sign(ey - sy)}]++
	}
	if len(dirs) < 3 {
		t.Fatalf("across 40 seeds the stairs only ever sat in %v — the exit is predictable", dirs)
	}
	for d, n := range dirs {
		if n > 28 { // more than 7 of 10 runs share one direction
			t.Fatalf("direction %v held for %d of 40 seeds — the exit is predictable", d, n)
		}
	}
}

func TestParseMouse(t *testing.T) {
	cases := []struct {
		in     string
		action MouseAction
		btn    int
		x, y   int
	}{
		{"\x1b[<0;10;5M", MousePress, BtnLeft, 9, 4},
		{"\x1b[<0;10;5m", MouseRelease, BtnLeft, 9, 4},
		{"\x1b[<32;7;3M", MouseMove, BtnLeft, 6, 2},
		{"\x1b[<35;7;3M", MouseMove, BtnNone, 6, 2},
		{"\x1b[<2;1;1M", MousePress, BtnRight, 0, 0},
	}
	for _, c := range cases {
		ev, used, ok := parseEvent([]byte(c.in))
		if !ok || used != len(c.in) {
			t.Fatalf("%q: parse failed (ok=%v used=%d)", c.in, ok, used)
		}
		if ev.Kind != EvMouse || ev.Action != c.action || ev.Button != c.btn || ev.MX != c.x || ev.MY != c.y {
			t.Fatalf("%q: got %+v, want action=%v btn=%d (%d,%d)", c.in, ev, c.action, c.btn, c.x, c.y)
		}
	}

	// A sequence split across two reads must not be consumed early.
	if _, _, ok := parseEvent([]byte("\x1b[<0;10;")); ok {
		t.Fatal("incomplete mouse sequence was accepted")
	}
}

func TestParseKeys(t *testing.T) {
	ev, _, _ := parseEvent([]byte("w"))
	if ev.Rune != 'w' {
		t.Fatalf("expected 'w', got %+v", ev)
	}
	ev, _, _ = parseEvent([]byte("\x1b[A"))
	if ev.Key != KeyUp {
		t.Fatalf("expected KeyUp, got %+v", ev)
	}
	ev, _, _ = parseEvent([]byte{3})
	if ev.Key != KeyCtrlC {
		t.Fatalf("expected KeyCtrlC, got %+v", ev)
	}
}

// TestScreenDiff makes sure the renderer only emits changed cells.
func TestScreenDiff(t *testing.T) {
	var sink countingWriter
	s := NewScreen(20, 5, bufio.NewWriter(&sink))
	s.Clear()
	s.Str(0, 0, "hello", 15, colorDefault)
	s.Flush()
	first := sink.n
	sink.n = 0
	s.Flush() // nothing changed
	if sink.n > 8 {
		t.Fatalf("redundant redraw wrote %d bytes (first frame was %d)", sink.n, first)
	}
}

type countingWriter struct{ n int }

func (c *countingWriter) Write(p []byte) (int, error) { c.n += len(p); return len(p), nil }

// ---- movement ---------------------------------------------------------------

// testLevel builds a level from an ASCII sketch ('#' wall, everything else floor).
func testLevel(rows []string) *Level {
	l := &Level{W: len(rows[0]), H: len(rows)}
	l.tiles = make([]Tile, l.W*l.H)
	l.visible = make([]bool, l.W*l.H)
	l.explored = make([]bool, l.W*l.H)
	l.flow = make([]int32, l.W*l.H)
	l.flowSrc = [2]int{-1, -1}
	for y, row := range rows {
		for x, c := range row {
			if c != '#' {
				l.tiles[l.idx(x, y)] = TileFloor
			}
		}
	}
	l.rooms = []Rect{{1, 1, l.W - 2, l.H - 2}}
	return l
}

// TestCorridorTraversal is the regression test for walking into a corridor and
// stopping dead: the way in is clear ahead but a wall sits alongside, and the
// body only fits if it is nearly perfectly centred. Whatever vertical offset
// the player enters with, they must get through.
func TestCorridorTraversal(t *testing.T) {
	rows := []string{
		"####################",
		"#........###########",
		"#..................#", // one-tile-tall corridor from x=9 rightwards
		"#........###########",
		"####################",
	}
	for _, offset := range []float64{0.10, 0.30, 0.50, 0.70, 0.90} {
		g := NewGame(1)
		g.level = testLevel(rows)
		g.player = newPlayer()
		g.player.pos = Vec{5.5, 2.0 + offset}

		startX := g.player.pos.X
		for i := 0; i < 600; i++ {
			g.moveWithCollision(&g.player.pos, Vec{0.12, 0}, g.player.radius)
		}
		if g.player.pos.X < 15 {
			t.Errorf("entering at y offset %.2f: stuck at x=%.2f (started %.2f), never got down the corridor",
				offset, g.player.pos.X, startX)
		}
	}
}

// TestVerticalCorridorTraversal is the same case rotated, since the collision
// box is asymmetric (cells are twice as tall as they are wide).
func TestVerticalCorridorTraversal(t *testing.T) {
	rows := []string{
		"##########",
		"#........#",
		"#####.####",
		"#####.####",
		"#####.####",
		"#........#",
		"##########",
	}
	for _, offset := range []float64{0.10, 0.30, 0.50, 0.70, 0.90} {
		g := NewGame(1)
		g.level = testLevel(rows)
		g.player = newPlayer()
		g.player.pos = Vec{5.0 + offset, 1.5}

		for i := 0; i < 600; i++ {
			g.moveWithCollision(&g.player.pos, Vec{0, 0.06}, g.player.radius)
		}
		if g.player.pos.Y < 5 {
			t.Errorf("entering at x offset %.2f: stuck at y=%.2f", offset, g.player.pos.Y)
		}
	}
}

// TestWallsStillSolid makes sure the corner assist did not turn walls into
// something the player can squeeze through.
func TestWallsStillSolid(t *testing.T) {
	rows := []string{
		"#######",
		"#..#..#",
		"#..#..#",
		"#..#..#",
		"#######",
	}
	g := NewGame(1)
	g.level = testLevel(rows)
	g.player = newPlayer()
	g.player.pos = Vec{2.5, 2.5}
	for i := 0; i < 2000; i++ {
		g.moveWithCollision(&g.player.pos, Vec{0.2, 0.05}, g.player.radius)
		g.moveWithCollision(&g.player.pos, Vec{0.2, -0.05}, g.player.radius)
	}
	if g.player.pos.X > 3 {
		t.Fatalf("player pushed through a solid wall to x=%.2f", g.player.pos.X)
	}
	if g.level.SolidAtPoint(g.player.pos) {
		t.Fatalf("player ended up inside a wall at %v", g.player.pos)
	}
}

// TestPhysicalRouteToStairs walks real generated floors with the real movement
// code — not just the tile graph — so a route that a body cannot actually fit
// through counts as a failure.
// The seed range is wide on purpose: a pathing flaw that strands the player
// showed up on roughly 4% of floors, and a ten-seed sample walked straight past
// it.
func TestPhysicalRouteToStairs(t *testing.T) {
	for seed := int64(1); seed <= 120; seed++ {
		g := NewGame(seed)
		for floor := 0; floor < 3; floor++ {
			g.level.flowSrc = [2]int{-1, -1}
			g.level.BuildFlow(int(g.stairsPos.X), int(g.stairsPos.Y))

			const dt = 1.0 / 60
			speed := g.player.baseSpeed()
			arrived := false
			stalls := 0
			last := g.player.pos
			for step := 0; step < 60*90 && !arrived; step++ {
				dir, ok := g.level.FlowStep(int(g.player.pos.X), int(g.player.pos.Y))
				if !ok {
					break
				}
				g.moveWithCollision(&g.player.pos, dir.unvisual().Scale(speed*dt), g.player.radius)
				if step%30 == 0 {
					if vdist(g.player.pos, last) < 0.2 {
						stalls++
					} else {
						stalls = 0
					}
					last = g.player.pos
				}
				if stalls > 8 {
					t.Fatalf("seed %d B%d: player wedged at %v (%.1f tiles from the stairs)",
						seed, g.depth, g.player.pos, vdist(g.player.pos, g.stairsPos))
				}
				arrived = vdist(g.player.pos, g.stairsPos) < 2.0
			}
			if !arrived {
				t.Fatalf("seed %d B%d: could not physically reach the stairs from spawn (stopped %.1f tiles away)",
					seed, g.depth, vdist(g.player.pos, g.stairsPos))
			}
			g.tryStairs()
		}
	}
}

// ---- weapons ----------------------------------------------------------------

// TestWeaponMustBePickedUp is the regression test for being able to select
// weapons that were never found.
func TestWeaponMustBePickedUp(t *testing.T) {
	g := NewGame(9)
	if !g.player.owned[wpPistol] {
		t.Fatal("the run should start with the pistol")
	}
	if !g.player.owned[wpMelee] {
		t.Fatal("melee should always be available")
	}
	for i := range weapons {
		if i != wpPistol && i != wpMelee && g.player.owned[i] {
			t.Fatalf("%s is owned at the start of a run", weapons[i].Name)
		}
	}

	for i := range weapons {
		if i == wpMelee {
			continue
		}
		g.handleKey(Event{Kind: EvKey, Rune: rune('1' + i)})
		if i != wpPistol && g.player.weapon == i {
			t.Errorf("switched to %s ([%d]) without picking it up", weapons[i].Name, i+1)
		}
	}
	if g.player.weapon != wpPistol {
		t.Fatalf("equipped weapon changed to %s", weapons[g.player.weapon].Name)
	}

	// After walking over the pickup, the slot works without replacing the
	// weapon already in hand.
	g.pickups = []Pickup{{pos: g.player.pos, kind: PickWeapon, weapon: wpRailgun}}
	g.updatePickups(0.016)
	if !g.player.owned[wpRailgun] {
		t.Fatal("picking up the railgun did not grant it")
	}
	if g.player.weapon != wpPistol {
		t.Fatal("picking up the railgun replaced the equipped pistol")
	}
	g.handleKey(Event{Kind: EvKey, Rune: '4'})
	if g.player.weapon != wpRailgun {
		t.Fatal("could not switch to the owned railgun")
	}
}

// TestFiringUsesOwnedWeaponOnly guards the actual effect of the bug: an
// unowned weapon must not be able to put its bullets on the map.
func TestFiringUsesOwnedWeaponOnly(t *testing.T) {
	g := NewGame(4)
	s := newTestScreen(120, 40)
	g.Draw(s)
	g.handleKey(Event{Kind: EvKey, Rune: '3'}) // shotgun, not picked up
	g.mouseSet = true
	g.firing = true
	g.aim = g.player.pos.Add(Vec{5, 0})
	g.Update(1.0 / 60)

	if len(g.bullets) != weapons[wpPistol].Pellets {
		t.Fatalf("fired %d projectiles; the pistol should have fired %d",
			len(g.bullets), weapons[wpPistol].Pellets)
	}
}

// TestWeaponDropsFavourNewOnes checks floors keep offering something you do
// not already have.
func TestWeaponDropsFavourNewOnes(t *testing.T) {
	g := NewGame(2)
	for i := range weapons {
		g.player.owned[i] = i == wpPistol
	}
	for n := 0; n < 40; n++ {
		if w := g.rollWeapon(); g.player.owned[w] {
			t.Fatalf("rolled %s, which is already owned", weapons[w].Name)
		}
	}
	// With everything owned it must still return a valid weapon.
	for i := range weapons {
		g.player.owned[i] = true
	}
	for n := 0; n < 40; n++ {
		w := g.rollWeapon()
		if w < 0 || w >= len(weapons) {
			t.Fatalf("rollWeapon returned an invalid index %d", w)
		}
		if w == wpMelee || w == wpPistol {
			t.Fatalf("rollWeapon offered %s as a floor pickup", weapons[w].Name)
		}
	}
}
