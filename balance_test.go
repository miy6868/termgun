package main

import (
	"strings"
	"testing"
)

// Health is the one resource a run is played against. These tests exist because
// it stopped being one: enough of the healing stacked that a competent player
// walked out of every fight with more health than they walked in with, and the
// bar only ever went up.

// TestLifestealIgnoresOverkill is the bug that drove it. Lifesteal used to pay
// out on the number rolled rather than the health removed, so the wider the
// weapon the more it healed: eight shotgun pellets into a 10 HP Swarmling deal
// 72, and 62 of that was landing on a corpse.
func TestLifestealIgnoresOverkill(t *testing.T) {
	g := arena(t, 1)
	g.player.lifesteal = 0.06
	g.player.hp = 10
	g.addEnemy(enemyDefs[1], Vec{16.5, 4.5}) // Swarmling, 10 HP
	before := g.player.hp

	for i := 0; i < 8; i++ { // one shotgun blast: 8 pellets x 9 damage
		if len(g.enemies) == 0 {
			break
		}
		g.hurtEnemy(&g.enemies[0], 9, Vec{1, 0})
	}

	healed := g.player.hp - before
	if max := enemyDefs[1].HP * g.player.lifesteal; healed > max+0.01 {
		t.Errorf("healed %.1f killing a %.0f HP enemy; overkill is being paid out "+
			"(cap %.1f)", healed, enemyDefs[1].HP, max)
	}
}

// TestKillingSomethingCostsMoreThanItHeals is the player-facing version of the
// complaint: walking into a fight has to be able to lose you the run. One touch
// from an enemy must cost more than draining that same enemy pays back.
func TestKillingSomethingCostsMoreThanItHeals(t *testing.T) {
	for i, def := range enemyDefs {
		g := arena(t, int64(100+i))
		g.player.lifesteal = 0.04 // one pick of 흡혈 회로
		g.player.hp = 1
		g.addEnemy(def, Vec{16.5, 4.5})

		for len(g.enemies) > 0 {
			g.hurtEnemy(&g.enemies[0], 5, Vec{1, 0})
			g.reapEnemies()
		}

		healed := g.player.hp - 1
		if healed >= def.Damage {
			t.Errorf("%s: killing one heals %.1f but hitting you costs %.1f; "+
				"fighting it pays for itself", def.Name, healed, def.Damage)
		}
	}
}

// TestNoPerkRefillsTheBar covers the removed 응급 처치. A perk that hands back a
// full bar erases whatever the floor just cost you, which makes every earlier
// decision about health retroactively free.
func TestNoPerkRefillsTheBar(t *testing.T) {
	for _, perk := range allPerks {
		g := arena(t, 7)
		g.player.hp = 1
		perk.Apply(&g.player)
		if g.player.hp >= g.player.maxHP {
			t.Errorf("%q took a player from 1 HP to full (%.0f/%.0f)",
				perk.Name, g.player.hp, g.player.maxHP)
		}
		if strings.Contains(perk.Desc, "전부 회복") {
			t.Errorf("%q still advertises a full heal", perk.Name)
		}
	}
}

// TestHealthNeverExceedsMax keeps the bar meaningful: every top-up in the game
// has to clamp, or the gauge renders past its own end.
func TestHealthNeverExceedsMax(t *testing.T) {
	apply := func(name string, f func(p *Player)) {
		g := arena(t, 8)
		g.player.hp = g.player.maxHP
		f(&g.player)
		if g.player.hp > g.player.maxHP {
			t.Errorf("%s left the player at %.0f/%.0f", name,
				g.player.hp, g.player.maxHP)
		}
	}
	for _, perk := range allPerks {
		apply("perk "+perk.Name, perk.Apply)
	}
	for _, offer := range shrineOffers {
		apply("shrine "+offer.Desc, offer.Apply)
	}
	apply("medkit", func(p *Player) { p.hp = minF(p.hp+medkitHeal, p.maxHP) })
}

// TestMedkitIsNotMostOfABar keeps a single pickup from undoing a whole fight.
func TestMedkitIsNotMostOfABar(t *testing.T) {
	g := arena(t, 9)
	if frac := medkitHeal / g.player.maxHP; frac > 0.25 {
		t.Errorf("a medkit restores %.0f%% of a fresh bar", frac*100)
	}
}
