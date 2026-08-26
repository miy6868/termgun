package main

import "testing"

// Boss floors alternate scripts: the OVERSEER owns the x5 floors, the
// MATRIARCH the x10 ones. The alternation is the point — a bigger version of
// the same boss is not a new question.

func TestBossFloorsAlternate(t *testing.T) {
	g := NewGame(7)
	for _, tc := range []struct {
		depth int
		want  string
	}{
		{5, "OVERSEER"},
		{10, "MATRIARCH"},
		{15, "OVERSEER"},
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
