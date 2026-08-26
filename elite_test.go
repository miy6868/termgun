package main

import "testing"

func addElite(g *Game, def EnemyDef, at Vec, kind EliteKind) *Enemy {
	g.addEliteEnemy(def, at, kind)
	return &g.enemies[len(g.enemies)-1]
}

// TestEliteIsVisiblyDifferent is the readability requirement. An elite that
// looks like an ordinary enemy is just an unfair health bar.
func TestEliteIsVisiblyDifferent(t *testing.T) {
	for _, kind := range eliteKinds {
		base := enemyDefs[0] // Grunt, glyph 'g'
		up := makeElite(base, kind)
		if up.Glyph == base.Glyph {
			t.Errorf("%v: the glyph is unchanged (%c)", kind, up.Glyph)
		}
		if up.Glyph < 'A' || up.Glyph > 'Z' {
			t.Errorf("%v: glyph %c is not uppercase", kind, up.Glyph)
		}
		if up.Color == base.Color {
			t.Errorf("%v: the colour is unchanged", kind)
		}
		if up.Name == base.Name {
			t.Errorf("%v: the name is unchanged", kind)
		}
		if up.HP <= base.HP {
			t.Errorf("%v: an elite is not tougher (%.0f vs %.0f)", kind, up.HP, base.HP)
		}
	}
}

// TestEliteRegenHealsOnlyOutOfCombat makes the counter-play sustained pressure
// rather than a damage check.
func TestEliteRegenHealsOnlyOutOfCombat(t *testing.T) {
	g := arena(t, 20)
	e := addElite(g, enemyDefs[5], Vec{25.5, 4.5}, EliteRegen)
	e.hp = e.maxHP * 0.4
	wounded := e.hp

	step(g, 3.0)
	e = &g.enemies[0]
	if e.hp <= wounded {
		t.Fatalf("a regenerating elite left alone did not heal (%.1f -> %.1f)", wounded, e.hp)
	}

	// Now keep hitting it: the regeneration must not kick in between hits.
	healed := e.hp
	for i := 0; i < 60; i++ {
		g.hurtEnemy(e, 0.01, Vec{})
		g.Update(1.0 / 60)
		e = &g.enemies[0]
	}
	if e.hp > healed {
		t.Errorf("it healed %.2f while under continuous fire", e.hp-healed)
	}
}

// TestEliteSplitSpawnsWeakerChildren checks the split happens at all — it is
// the one elite whose effect runs during the death sweep, where an earlier
// version silently dropped the children.
func TestEliteSplitSpawnsWeakerChildren(t *testing.T) {
	g := arena(t, 21)
	e := addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteSplit)
	parentHP := e.maxHP
	e.hp = 0
	g.reapEnemies()

	if len(g.enemies) != 2 {
		t.Fatalf("a splitting elite left %d enemies, want 2", len(g.enemies))
	}
	for i := range g.enemies {
		c := &g.enemies[i]
		if c.elite != EliteNone {
			t.Error("a fragment is itself an elite; splits would cascade")
		}
		if c.maxHP >= parentHP {
			t.Errorf("fragment has %.0f hp, parent had %.0f; fragments must be weaker",
				c.maxHP, parentHP)
		}
	}
}

// TestEliteBurstAnswersHits verifies the reactive shot survives the bullet
// sweep it is fired from.
func TestEliteBurstAnswersHits(t *testing.T) {
	g := arena(t, 22)
	e := addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteBurst)
	g.bullets = g.bullets[:0]

	g.hurtEnemy(e, 1, Vec{})
	if len(g.bullets) == 0 {
		t.Fatal("a burst elite did not answer a hit")
	}

	// And it must not fire on every single tick of damage.
	n := len(g.bullets)
	g.hurtEnemy(e, 1, Vec{})
	if len(g.bullets) > n {
		t.Error("the burst has no cooldown; sustained fire would bury the player")
	}
}

// TestEliteBurstSurvivesTheBulletSweep is the regression for the aliasing bug:
// the burst is triggered from inside updateBullets, which compacts the same
// slice the new bullets are appended to.
func TestEliteBurstSurvivesTheBulletSweep(t *testing.T) {
	g := arena(t, 23)
	e := addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteBurst)
	e.hp = e.maxHP // survive the hit
	g.bullets = []Bullet{{
		pos: e.pos.Sub(Vec{1, 0}), vel: Vec{40, 0}, dmg: 1, life: 1,
		glyph: '*', color: 46, friendly: true,
	}}
	g.updateBullets(1.0 / 60)

	hostile := 0
	for _, b := range g.bullets {
		if !b.friendly {
			hostile++
		}
	}
	if hostile == 0 {
		t.Fatal("the burst fired during the bullet sweep was lost; " +
			"appending to the slice being compacted drops it")
	}
}

// TestEliteAuraBuffsNeighbours covers the last rule, and that it wears off.
func TestEliteAuraBuffsNeighbours(t *testing.T) {
	g := arena(t, 24)
	// Index rather than hold pointers: adding the next enemy can reallocate the
	// slice and leave a stale one pointing at the old array.
	addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteAura)
	g.addEnemy(enemyDefs[0], Vec{21.5, 4.5}) // in range
	g.addEnemy(enemyDefs[0], Vec{24.5, 4.5}.Add(Vec{0, 20}))

	g.Update(1.0 / 60)
	leader, near, far := &g.enemies[0], &g.enemies[1], &g.enemies[2]
	if near.buffed <= 0 {
		t.Error("an enemy standing next to the aura elite was not buffed")
	}
	if far.buffed > 0 {
		t.Error("an enemy far outside the aura radius was buffed")
	}
	if near.speed() <= near.def.Speed || near.damage() <= near.def.Damage {
		t.Error("the buff does not actually change speed or damage")
	}
	if leader.buffed > 0 {
		t.Error("the aura elite buffs itself; auras must not stack on their source")
	}

	// Kill the leader and the buff must lapse.
	g.enemies[0].hp = 0
	g.reapEnemies()
	step(g, 1.0)
	for i := range g.enemies {
		if g.enemies[i].buffed > 0 {
			t.Error("the buff outlived the elite that granted it")
		}
	}
}

// TestElitesAppearWithDepth checks the pacing: none on the first floor, and a
// real chance later.
func TestElitesAppearWithDepth(t *testing.T) {
	if eliteChance(1) != 0 {
		t.Errorf("B1 can spawn elites (chance %.2f); the first floor should teach the basics",
			eliteChance(1))
	}
	if eliteChance(8) <= eliteChance(3) {
		t.Error("elites do not get more common with depth")
	}

	seen := map[EliteKind]int{}
	for seed := int64(1); seed <= 40; seed++ {
		g := NewGame(seed)
		g.enterDepth(6)
		for i := range g.enemies {
			if k := g.enemies[i].elite; k != EliteNone {
				seen[k]++
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("40 runs of B6 produced no elites at all")
	}
	for _, k := range eliteKinds {
		if seen[k] == 0 {
			t.Errorf("elite kind %v never appeared across 40 floors", k)
		}
	}
}

// TestEliteShieldAbsorbsThenBreaks pins the pool: it soaks damage first, the
// remainder spills through a breaking hit, and the pool refills only after the
// elite has been left alone.
func TestEliteShieldAbsorbsThenBreaks(t *testing.T) {
	g := arena(t, 26)
	e := addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteShield)
	full := e.maxHP
	if e.shield <= 0 {
		t.Fatal("a shield elite spawned with no shield")
	}

	g.hurtEnemy(e, 5, Vec{})
	e = &g.enemies[0]
	if e.hp != full {
		t.Fatalf("the shield let %.1f damage through to the body", full-e.hp)
	}
	if e.shield >= full*shieldFrac {
		t.Error("the shield absorbed nothing")
	}

	g.hurtEnemy(e, 9999, Vec{})
	if g.enemies[0].hp >= full {
		t.Error("a depleted shield kept absorbing; big hits must punch through")
	}
}

func TestEliteShieldRefillsOutOfCombat(t *testing.T) {
	g := arena(t, 27)
	e := addElite(g, enemyDefs[0], Vec{20.5, 4.5}, EliteShield)
	g.hurtEnemy(e, 1, Vec{})
	e = &g.enemies[0]
	e.shield = e.maxHP * shieldFrac * 0.2 // mostly broken

	step(g, shieldRegenDelay+1)

	if got := g.enemies[0].shield; got < e.maxHP*shieldFrac*0.5 {
		t.Errorf("the shield did not refill out of combat (%.1f / %.1f)",
			got, e.maxHP*shieldFrac)
	}
}

// TestEliteKillsAreWorthMore keeps the risk paying.
func TestEliteKillsAreWorthMore(t *testing.T) {
	base := enemyDefs[5]
	up := makeElite(base, EliteRegen)
	if up.Score <= base.Score || up.XP <= base.XP {
		t.Errorf("elite is worth %d score / %d xp, base is %d / %d",
			up.Score, up.XP, base.Score, base.XP)
	}

	g := arena(t, 25)
	e := addElite(g, base, Vec{20.5, 4.5}, EliteRegen)
	e.hp = 0
	before := len(g.pickups)
	g.reapEnemies()
	if len(g.pickups) <= before {
		t.Error("an elite dropped nothing")
	}
}
