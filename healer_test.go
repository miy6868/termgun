package main

import "testing"

// The Medic never attacks: its whole threat is undoing the player's damage.
// These tests pin both halves of that — it actually heals the wounded, and it
// gets out of range instead of standing still to be killed.

func TestMedicHealsWoundedAllies(t *testing.T) {
	g := arena(t, 30)
	g.addEnemy(enemyDefs[7], Vec{24.5, 4.5}) // Medic
	g.addEnemy(enemyDefs[4], Vec{26.5, 4.5}) // Turret: Speed 0, so the patient stays put
	patient := &g.enemies[1]
	patient.hp = patient.maxHP * 0.3
	before := patient.hp

	step(g, 2.0)

	if got := g.enemies[1].hp; got < before+g.enemies[1].maxHP*0.05 {
		t.Fatalf("a wounded ally next to a medic for 2s healed %.2f -> %.2f (max %.0f)",
			before, got, g.enemies[1].maxHP)
	}
}

func TestMedicIgnoresHealthyAllies(t *testing.T) {
	g := arena(t, 31)
	g.addEnemy(enemyDefs[7], Vec{24.5, 4.5})
	g.addEnemy(enemyDefs[4], Vec{26.5, 4.5}) // full HP

	step(g, 1.5)

	if got := g.enemies[1].hp; got > g.enemies[1].maxHP {
		t.Fatalf("the medic overhealed: %.1f / %.0f", got, g.enemies[1].maxHP)
	}
}

func TestMedicFleesThePlayer(t *testing.T) {
	g := arena(t, 32)
	g.addEnemy(enemyDefs[7], Vec{19.5, 4.5}) // inside the panic ring
	start := vdist(g.enemies[0].pos, g.player.pos)

	step(g, 1.0)

	if d := vdist(g.enemies[0].pos, g.player.pos); d <= start {
		t.Fatalf("the medic did not open the distance (%.1f -> %.1f)", start, d)
	}
}
