package main

import "testing"

// The first two act bosses teach their own scripts and B15 has a final owner.

func TestBossFloorsAlternate(t *testing.T) {
	g := NewGame(7)
	for _, tc := range []struct {
		depth int
		want  string
	}{
		{5, "OVERSEER"},
		{10, "MATRIARCH"},
		{15, "CORE WARDEN"},
	} {
		g.enterDepth(tc.depth)
		b := g.bossEnemy()
		if b == nil {
			t.Fatalf("B%d spawned without a living boss", tc.depth)
		}
		if b.def.Name != tc.want {
			t.Errorf("B%d spawned %s, want %s", tc.depth, b.def.Name, tc.want)
		}
	}
}

func TestCoreWardenFiresDistinctDoubleRing(t *testing.T) {
	g := arena(t, 35)
	g.addEnemy(coreBossDef, Vec{25.5, 4.5})
	c := &g.enemies[0]
	c.alert = true
	c.phase = 0.1
	g.coreAttack(c, Vec{1, 0})
	if len(g.bullets) != 24 {
		t.Fatalf("CORE WARDEN double ring fired %d bullets, want 24", len(g.bullets))
	}
}

// The MATRIARCH's first script in her cycle is the brood wave. Her escorts are
// what make her floor read differently from the OVERSEER's bullet walls.
func TestMatriarchSummonsBrood(t *testing.T) {
	g := arena(t, 33)
	def := queenDef
	def.HP *= 2 // survive anything the arena throws at her
	g.addEnemy(def, Vec{25.5, 4.5})
	q := &g.enemies[0]
	q.alert = true
	q.phase = 0.1 // first script of the cycle: summon
	n := len(g.enemies)

	step(g, 0.5)

	if len(g.enemies) <= n {
		t.Fatal("the MATRIARCH summoned no brood")
	}
	for i := n; i < len(g.enemies); i++ {
		if g.enemies[i].def.Behavior == BeBoss {
			t.Error("the brood included another boss")
		}
	}
}

// Summoning can grow the enemy slice. The boss's own state must still be
// written back when that append moves the slice to a new backing array;
// otherwise it summons again every frame because its cooldown never sticks.
func TestMatriarchSummonPreservesBossState(t *testing.T) {
	g := arena(t, 34)
	g.addEnemy(queenDef, Vec{25.5, 4.5})
	g.enemies[0].alert = true
	g.enemies[0].phase = 0.1 // brood script

	// Force the brood append to reallocate, which exposes stale Enemy pointers.
	g.enemies = append([]Enemy(nil), g.enemies...)
	g.Update(1.0 / 60)

	q := enemyByName(g, queenDef.Name)
	if q == nil {
		t.Fatal("the MATRIARCH disappeared while summoning")
	}
	if q.cool <= 0 {
		t.Fatal("the summon cooldown was lost when the enemy slice grew")
	}
	if q.phase <= 0.1 {
		t.Fatal("the boss phase update was lost when the enemy slice grew")
	}
}
