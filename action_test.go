package main

import "testing"

// The dash is the game's action verb. These tests pin its two offensive
// judgments: the shoulder charge through a body, and the perfect dodge that
// pays out when the blink beats an attack by timing rather than by luck.

func TestDashStrikeHitsWhatItPassesThrough(t *testing.T) {
	g := arena(t, 34)
	g.mouseSet = true
	at := g.player.pos.Add(Vec{1.5, 0})
	g.aim = at
	g.addEnemy(enemyDefs[0], at)
	e := &g.enemies[0]

	g.startDash()
	if g.player.dashTimer <= 0 {
		t.Fatal("the dash did not start")
	}
	step(g, 0.3)

	for i := range g.enemies {
		if g.enemies[i].id != e.id {
			continue
		}
		if g.enemies[i].hp >= e.maxHP {
			t.Fatalf("dashing through a grunt left it untouched (%.1f/%.0f)",
				g.enemies[i].hp, e.maxHP)
		}
		return
	}
	// Not finding it also passes: the strike killed it and the corpse was reaped.
}

func TestDashStrikeHitsEachBodyOnce(t *testing.T) {
	g := arena(t, 35)
	g.mouseSet = true
	at := g.player.pos.Add(Vec{1.5, 0})
	g.aim = at
	def := enemyDefs[5] // Brute: too tough to die mid-dash (90 HP)
	g.addEnemy(def, at)

	g.startDash()
	step(g, 0.3)

	e := &g.enemies[0]
	if lost := e.maxHP - e.hp; lost > dashStrikeDamage+0.5 {
		t.Fatalf("one dash dealt %.1f damage; a single pass must hit once", lost)
	}
}

func TestPerfectDodgeRewardsTheTiming(t *testing.T) {
	g := arena(t, 36)
	p := &g.player
	g.mouseSet = true
	g.aim = p.pos.Add(Vec{1, 0})

	g.startDash()
	cd := p.dashCD
	g.damagePlayer(10, Vec{1, 0})

	if !p.dodgedThisDash {
		t.Fatal("a hit landing inside a fresh dash was not graded as a perfect dodge")
	}
	if p.dashCD >= cd {
		t.Errorf("the gauge was not refunded (%.2f -> %.2f)", cd, p.dashCD)
	}
	if g.hitStop <= 0 {
		t.Error("no bullet time was granted")
	}
	if p.hp != p.maxHP {
		t.Errorf("an absorbed hit still cost health (%.0f/%.0f)", p.hp, p.maxHP)
	}

	// One reward per dash: riding a burst of hits must not print it in bulk.
	g.hitStop = 0
	cd = p.dashCD
	g.damagePlayer(10, Vec{1, 0})
	if p.dashCD != cd || g.hitStop > 0 {
		t.Error("the perfect dodge paid out twice on one dash")
	}

	// And outside the window it never fires at all.
	g.startDash()
	p.lastDashStart -= perfectDodgeWindow + 0.01
	before := p.dashCD
	g.hitStop = 0
	g.damagePlayer(10, Vec{1, 0})
	if p.dashCD != before || g.hitStop > 0 {
		t.Error("a stale dash still counted as a perfect dodge")
	}
}

// Light bodies read the blink and hop aside — one clean dash must not delete
// a whole pack of the fastest enemies for free.
func TestQuickEnemiesSidestepADash(t *testing.T) {
	g := arena(t, 37)
	g.mouseSet = true
	at := g.player.pos.Add(Vec{2.5, 0})
	g.aim = g.player.pos.Add(Vec{4, 0})
	def := enemyDefs[1] // Swarmling: dodges
	g.addEnemy(def, at)
	e := &g.enemies[0]

	g.startDash()
	step(g, 0.15)

	if got := g.enemies[0].hp; got < e.maxHP {
		t.Fatalf("the swarmling ate the strike anyway (%.1f/%.0f)", got, e.maxHP)
	}
	if off := vdist(g.enemies[0].pos, Vec{g.enemies[0].pos.X, at.Y}); off < 0.4 {
		t.Errorf("the sidestep barely moved it (%.2f tiles off the dash line)", off)
	}

	// The reaction is committed: right after hopping it cannot dodge again,
	// which is the punish window.
	if g.enemies[0].dodgeCD <= 0 {
		t.Error("dodging left no cooldown; the enemy could dance forever")
	}
}

// Non-dodging bodies are what keep the shoulder charge honest.
func TestHeavyBodiesStillEatTheDash(t *testing.T) {
	g := arena(t, 38)
	g.mouseSet = true
	at := g.player.pos.Add(Vec{2.5, 0})
	g.aim = g.player.pos.Add(Vec{4, 0})
	g.addEnemy(enemyDefs[0], at) // Grunt: no dodge
	e := &g.enemies[0]

	g.startDash()
	step(g, 0.15)

	if got := g.enemies[0].hp; got >= e.maxHP {
		t.Fatal("a plain grunt sidestepped a dash it has no read on")
	}
}

// Packs fan out instead of queueing, but the fan must converge: curving is a
// approach shape, not an orbit.
func TestPacksSurroundInsteadOfQueueing(t *testing.T) {
	if flankCurve(1, 2) != 0 {
		t.Error("the flank curve is active at melee range; the pack would orbit forever")
	}
	if flankCurve(1, 20) != flankAngle || flankCurve(-1, 20) != -flankAngle {
		t.Error("the flank curve does not reach full strength far away")
	}

	g := arena(t, 39)
	g.addEnemy(enemyDefs[0], Vec{21.5, 4.5})
	g.addEnemy(enemyDefs[0], Vec{21.5, 6.5})
	if g.enemies[0].flank == g.enemies[1].flank {
		t.Fatal("two consecutive ids drew the same wing; packs cannot split")
	}

	step(g, 2.5)

	for i := range g.enemies {
		if d := vdist(g.enemies[i].pos, g.player.pos); d > 2.6 {
			t.Errorf("grunt %d never closed in (%.1f tiles away); "+
				"flanking turned into stalling", i, d)
		}
	}
}
