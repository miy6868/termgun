package main

import (
	"math"
	"strings"
	"testing"
)

// arena builds an open room with the player in the middle and nothing else in
// it, so a single enemy's behaviour can be watched in isolation.
func arena(t *testing.T, seed int64) *Game {
	t.Helper()
	g := newGameWithInput(seed, InputKitty)
	rows := []string{
		"##############################",
		"#............................#",
		"#............................#",
		"#............................#",
		"#............................#",
		"#............................#",
		"#............................#",
		"#............................#",
		"##############################",
	}
	g.level = testLevel(rows)
	g.player.pos = Vec{15.5, 4.5}
	g.enemies = g.enemies[:0]
	g.bullets = g.bullets[:0]
	g.pickups = g.pickups[:0]
	g.mouseSet = false
	g.level.ComputeFOV(int(g.player.pos.X), int(g.player.pos.Y), 30)
	return g
}

func enemyByName(g *Game, name string) *Enemy {
	for i := range g.enemies {
		if g.enemies[i].def.Name == name {
			return &g.enemies[i]
		}
	}
	return nil
}

// step advances the simulation without letting the player act.
func step(g *Game, seconds float64) {
	for t := 0.0; t < seconds; t += 1.0 / 60 {
		g.Update(1.0 / 60)
	}
}

// TestFOVIsAspectCorrect guards the shape of what the player can see. A world
// tile is drawn square but counts double vertically, so a sight radius of R has
// to reach R tiles sideways and R/2 up and down. Getting this wrong once cost
// the player half their horizontal vision, which put snipers permanently
// outside the range at which their telegraph could be seen.
func TestFOVIsAspectCorrect(t *testing.T) {
	const radius = 14
	rows := make([]string, 41)
	for i := range rows {
		rows[i] = strings.Repeat(".", 81)
	}
	l := testLevel(rows)
	cx, cy := 40, 20
	l.ComputeFOV(cx, cy, radius)

	reach := func(dx, dy int) int {
		n := 0
		for k := 1; ; k++ {
			if !l.Visible(cx+dx*k, cy+dy*k) {
				return n
			}
			n = k
		}
	}
	wantH, wantV := radius, radius/aspect
	for _, c := range []struct {
		name     string
		got      int
		want     float64
		tolerate float64
	}{
		{"right", reach(1, 0), float64(wantH), 1},
		{"left", reach(-1, 0), float64(wantH), 1},
		{"down", reach(0, 1), wantV, 1},
		{"up", reach(0, -1), wantV, 1},
	} {
		if math.Abs(float64(c.got)-c.want) > c.tolerate {
			t.Errorf("sight reaches %d tiles %s, want about %.0f", c.got, c.name, c.want)
		}
	}
	// And the horizontal reach must genuinely be the longer one.
	if reach(1, 0) <= reach(0, 1) {
		t.Errorf("sight reaches %d sideways but %d vertically; a tile counts double "+
			"vertically, so sideways must be further", reach(1, 0), reach(0, 1))
	}
}

// TestSniperTelegraphsBeforeFiring is the core of the design change: the shot
// has to be announced, and it has to be announced for long enough to react to.
func TestSniperTelegraphsBeforeFiring(t *testing.T) {
	g := arena(t, 1)
	g.addEnemy(enemyDefs[6], Vec{25.5, 4.5}) // Sniper
	e := enemyByName(g, "Sniper")
	e.alert = true

	sawWindup := false
	for i := 0; i < 240; i++ {
		g.Update(1.0 / 60)
		if e = enemyByName(g, "Sniper"); e == nil {
			t.Fatal("the sniper vanished")
		}
		if e.telegraphing() {
			sawWindup = true
			if len(g.bullets) > 0 {
				t.Fatal("the sniper fired while still winding up; the telegraph must come first")
			}
		}
		if len(g.bullets) > 0 {
			break
		}
	}
	if !sawWindup {
		t.Fatal("the sniper fired with no visible windup at all")
	}
	if len(g.bullets) == 0 {
		t.Fatal("the sniper never fired")
	}
}

// TestSniperShotIsDodgeable is what the windup buys. The shot commits to the
// direction locked at the start, so stepping off that line avoids it. If the
// enemy re-aimed at the last instant the telegraph would be decoration.
func TestSniperShotIsDodgeable(t *testing.T) {
	g := arena(t, 2)
	g.addEnemy(enemyDefs[6], Vec{25.5, 4.5})
	e := enemyByName(g, "Sniper")
	e.alert = true

	// Run until it commits.
	for i := 0; i < 600 && !e.telegraphing(); i++ {
		g.Update(1.0 / 60)
		e = enemyByName(g, "Sniper")
	}
	if !e.telegraphing() {
		t.Fatal("the sniper never wound up")
	}
	locked := e.lockDir

	// Step out of the lane while it is still aiming.
	g.player.pos = g.player.pos.Add(Vec{0, -3})
	hpBefore := g.player.hp
	step(g, 2.0)

	if g.player.hp < hpBefore {
		t.Errorf("the player was hit after leaving the telegraphed line (locked %v)", locked)
	}
}

// TestSniperShotHitsIfYouStandStill is the other half: the telegraph must be a
// real threat, or ignoring it costs nothing.
func TestSniperShotHitsIfYouStandStill(t *testing.T) {
	g := arena(t, 3)
	g.addEnemy(enemyDefs[6], Vec{25.5, 4.5})
	enemyByName(g, "Sniper").alert = true

	hpBefore := g.player.hp
	step(g, 4.0)
	if g.player.hp >= hpBefore {
		t.Error("standing directly in front of a sniper for four seconds cost no health")
	}
}

// TestBruteChargeStunsOnWallHit gives the dodge a reward. Sidestepping a charge
// should leave the brute wide open rather than simply resetting it.
func TestBruteChargeStunsOnWallHit(t *testing.T) {
	g := arena(t, 4)
	// Line the brute up so a charge runs straight into the right-hand wall.
	g.player.pos = Vec{26.5, 4.5}
	g.addEnemy(enemyDefs[5], Vec{20.5, 4.5}) // Brute
	e := enemyByName(g, "Brute")
	e.alert = true

	for i := 0; i < 600 && !e.telegraphing(); i++ {
		g.Update(1.0 / 60)
		e = enemyByName(g, "Brute")
	}
	if !e.telegraphing() {
		t.Fatal("the brute never wound up a charge")
	}
	// Get out of the lane; the charge should carry on into the wall.
	g.player.pos = Vec{20.5, 7.5}

	stunned := false
	for i := 0; i < 300; i++ {
		g.Update(1.0 / 60)
		if e = enemyByName(g, "Brute"); e != nil && e.stun > 0 {
			stunned = true
			break
		}
	}
	if !stunned {
		t.Error("the brute charged past and was never stunned by the wall")
	}
}

// TestBomberFuseCommits is what makes knockback a tool rather than a stat. Once
// lit, the fuse keeps burning, so shoving the bomber away means it detonates
// somewhere else instead of following you and resetting.
func TestBomberFuseCommits(t *testing.T) {
	g := arena(t, 5)
	g.addEnemy(enemyDefs[3], Vec{17.0, 4.5}) // Bomber, right next to the player
	e := enemyByName(g, "Bomber")
	e.alert = true

	for i := 0; i < 300 && e.fuse <= 0; i++ {
		g.Update(1.0 / 60)
		e = enemyByName(g, "Bomber")
	}
	if e == nil || e.fuse <= 0 {
		t.Fatal("the bomber never lit its fuse next to the player")
	}
	// Run away. The fuse must not go out.
	g.player.pos = Vec{4.5, 4.5}
	lit := e.fuse
	step(g, 0.3)
	if e = enemyByName(g, "Bomber"); e != nil && e.fuse < lit {
		t.Errorf("the fuse went out when the player left (%.2f -> %.2f)", lit, e.fuse)
	}
}

// TestBomberCanBeKnockedBack checks the light body actually moves when hit.
func TestBomberCanBeKnockedBack(t *testing.T) {
	g := arena(t, 6)
	g.addEnemy(enemyDefs[3], Vec{20.5, 4.5})
	b := enemyByName(g, "Bomber")
	before := b.pos.X
	g.hurtEnemy(b, 1, Vec{1, 0})
	moved := b.pos.X - before

	g.addEnemy(enemyDefs[5], Vec{10.5, 4.5}) // Brute: heavy
	br := enemyByName(g, "Brute")
	beforeBr := br.pos.X
	g.hurtEnemy(br, 1, Vec{1, 0})
	movedBr := br.pos.X - beforeBr

	if moved <= movedBr {
		t.Errorf("a bomber was shoved %.3f and a brute %.3f; light bodies must move further",
			moved, movedBr)
	}
	if moved <= 0 {
		t.Error("knockback did not move the bomber at all")
	}
}

// TestTurretPreheats covers the last telegraph.
func TestTurretPreheats(t *testing.T) {
	g := arena(t, 7)
	g.addEnemy(enemyDefs[4], Vec{25.5, 4.5}) // Turret
	e := enemyByName(g, "Turret")
	e.alert = true

	saw := false
	for i := 0; i < 240 && len(g.bullets) == 0; i++ {
		g.Update(1.0 / 60)
		if e = enemyByName(g, "Turret"); e != nil && e.telegraphing() {
			saw = true
		}
	}
	if !saw {
		t.Error("the turret fired its burst with no preheat")
	}
}

// TestTelegraphIsDrawn is the discoverability half. A windup nobody can see is
// the same as no windup at all.
func TestTelegraphIsDrawn(t *testing.T) {
	g := arena(t, 8)
	g.addEnemy(enemyDefs[6], Vec{25.5, 4.5})
	e := enemyByName(g, "Sniper")
	e.alert = true
	s := newTestScreen(120, 40)

	for i := 0; i < 600; i++ {
		g.Update(1.0 / 60)
		e = enemyByName(g, "Sniper")
		if e != nil && e.telegraphing() && e.windup < e.def.Windup*0.4 {
			break
		}
	}
	if e == nil || !e.telegraphing() {
		t.Fatal("could not catch the sniper mid-windup")
	}
	g.Draw(s)

	// The lane runs from the sniper towards the player along the locked line.
	found := 0
	for d := 1.0; d < 8; d += 0.5 {
		p := e.pos.Add(e.lockDir.unvisual().Scale(d))
		x, y := g.worldToScreen(p)
		if y >= 0 && y < s.H && x >= 0 && x < s.W && s.cur[y*s.W+x].R == ':' {
			found++
		}
	}
	if found == 0 {
		t.Error("nothing was drawn along the telegraphed line; the player cannot see it coming")
	}
}
