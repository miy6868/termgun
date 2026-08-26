package main

import (
	"strings"
	"testing"
)

// barrelArena is an open room with no enemies, level hazards or loot.
func barrelArena(t *testing.T, seed int64) *Game {
	t.Helper()
	g := arena(t, seed)
	g.barrels = g.barrels[:0]
	return g
}

// TestBarrelExplodesWhenShot is the basic contract.
func TestBarrelExplodesWhenShot(t *testing.T) {
	g := barrelArena(t, 30)
	g.barrels = append(g.barrels, Barrel{pos: Vec{22.5, 4.5}, hp: barrelHP})
	// A Brute, so a single blast cannot kill it and empty the slice.
	g.addEnemy(enemyDefs[5], Vec{23.5, 4.5})
	hp := g.enemies[0].hp

	g.hurtBarrel(&g.barrels[0], barrelHP)
	if !g.barrels[0].lit {
		t.Fatal("a barrel that took lethal damage did not light")
	}
	// The fuse gives the player time to read what is about to happen.
	g.updateBarrels(barrelFuse / 2)
	if len(g.barrels) == 0 {
		t.Fatal("the barrel went off instantly; the fuse is what makes a chain readable")
	}
	step(g, 1.0)

	if len(g.barrels) != 0 {
		t.Error("the barrel never detonated")
	}
	if g.enemies[0].hp >= hp {
		t.Error("the blast did not hurt an enemy standing next to it")
	}
}

// TestBarrelsChain is the play the feature exists for: line them up, shoot one.
func TestBarrelsChain(t *testing.T) {
	g := barrelArena(t, 31)
	for i := 0; i < 4; i++ {
		g.barrels = append(g.barrels, Barrel{pos: Vec{18.5 + float64(i)*3, 4.5}, hp: barrelHP})
	}
	g.player.pos = Vec{4.5, 4.5} // well clear of the blast

	g.hurtBarrel(&g.barrels[0], barrelHP)
	step(g, 3.0)

	if len(g.barrels) != 0 {
		t.Errorf("%d of 4 barrels survived; the chain did not carry", len(g.barrels))
	}
}

// TestBarrelChainDoesNotRunAway guards the obvious way to write an infinite
// loop: a barrel catching its own blast and relighting forever.
func TestBarrelChainDoesNotRunAway(t *testing.T) {
	g := barrelArena(t, 32)
	g.barrels = append(g.barrels, Barrel{pos: Vec{20.5, 4.5}, hp: barrelHP})
	g.player.pos = Vec{4.5, 4.5}

	g.hurtBarrel(&g.barrels[0], barrelHP)
	step(g, 5.0)

	if len(g.barrels) != 0 {
		t.Errorf("%d barrels left after five seconds; one relit itself", len(g.barrels))
	}
}

// TestBarrelHurtsThePlayerToo keeps it a hazard rather than a free bomb.
func TestBarrelHurtsThePlayerToo(t *testing.T) {
	g := barrelArena(t, 33)
	g.barrels = append(g.barrels, Barrel{pos: Vec{16.0, 4.5}, hp: barrelHP})
	hp := g.player.hp

	g.hurtBarrel(&g.barrels[0], barrelHP)
	step(g, 1.0)

	if g.player.hp >= hp {
		t.Error("standing on top of an exploding barrel cost nothing")
	}
}

// TestEnemyFireCanSetOffBarrels means the room is live for both sides.
func TestEnemyFireCanSetOffBarrels(t *testing.T) {
	g := barrelArena(t, 34)
	g.barrels = append(g.barrels, Barrel{pos: Vec{20.5, 4.5}, hp: barrelHP})
	g.bullets = []Bullet{{
		pos: Vec{19.5, 4.5}, vel: Vec{40, 0}, dmg: barrelHP, life: 1,
		glyph: 'o', color: 203, friendly: false,
	}}
	g.updateBullets(1.0 / 60)

	if !g.barrels[0].lit {
		t.Error("an enemy round passed straight through a barrel")
	}
}

// ---- cracked walls ----------------------------------------------------------

// TestCrackedWallBlocksUntilBlown is the whole point: it is a wall, and then it
// is a route.
func TestCrackedWallBlocksUntilBlown(t *testing.T) {
	g := barrelArena(t, 35)
	g.level.tiles[g.level.idx(20, 4)] = TileCracked
	if !g.level.Solid(20, 4) {
		t.Fatal("a cracked wall is not solid, so it is not a wall")
	}

	// Plain bullets must not open it — only explosives.
	g.bullets = []Bullet{{
		pos: Vec{19.5, 4.5}, vel: Vec{60, 0}, dmg: 999, life: 1,
		glyph: '*', color: 46, friendly: true,
	}}
	for i := 0; i < 30; i++ {
		g.updateBullets(1.0 / 60)
	}
	if g.level.At(20, 4) != TileCracked {
		t.Error("an ordinary bullet broke a cracked wall; only explosives should")
	}

	g.explode(Vec{19.0, 4.5}, 4.5, 30, true)
	if g.level.Solid(20, 4) {
		t.Fatal("an explosion next to a cracked wall did not open it")
	}
	if g.level.At(20, 4) != TileRubble {
		t.Errorf("a blown wall became %v, want rubble", g.level.At(20, 4))
	}
}

// TestBreakingAWallInvalidatesPathing is the bug this would otherwise hide: the
// flow field is cached, so a new hole in the map is invisible to every enemy
// until something else happens to rebuild it.
func TestBreakingAWallInvalidatesPathing(t *testing.T) {
	g := barrelArena(t, 36)
	g.level.tiles[g.level.idx(20, 4)] = TileCracked
	g.level.BuildFlow(4, 4)
	before := g.level.flowSrc

	if !g.level.Break(20, 4) {
		t.Fatal("Break reported no change on a cracked wall")
	}
	if g.level.flowSrc == before {
		t.Error("the cached route survived the map changing shape")
	}
}

// TestCrackedWallsNeverRemoveARoute checks placement can only ever add options:
// they are carved out of existing walls, so no floor is lost.
func TestCrackedWallsNeverRemoveARoute(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		g := NewGame(seed)
		g.enterDepth(4)
		l := g.level
		for y := 0; y < l.H; y++ {
			for x := 0; x < l.W; x++ {
				if l.At(x, y) != TileCracked {
					continue
				}
				// Opening it must join two places that are already open.
				horiz := !l.Solid(x-1, y) && !l.Solid(x+1, y)
				vert := !l.Solid(x, y-1) && !l.Solid(x, y+1)
				if !horiz && !vert {
					t.Fatalf("seed %d: cracked wall at (%d,%d) opens into solid rock",
						seed, x, y)
				}
			}
		}
	}
}

// ---- acid -------------------------------------------------------------------

func acidGame(t *testing.T, seed int64) *Game {
	t.Helper()
	g := barrelArena(t, seed)
	for x := 14; x <= 18; x++ {
		g.level.tiles[g.level.idx(x, 4)] = TileAcid
	}
	return g
}

// TestAcidBurnsBothSides is what makes a pool a weapon rather than just a tax.
func TestAcidBurnsBothSides(t *testing.T) {
	g := acidGame(t, 40)
	g.player.pos = Vec{15.5, 4.5}
	g.addEnemy(enemyDefs[4], Vec{17.5, 4.5}) // Turret: stays put
	php, ehp := g.player.hp, g.enemies[0].hp

	step(g, 2.0)

	if g.player.hp >= php {
		t.Error("standing in acid did not hurt the player")
	}
	if g.enemies[0].hp >= ehp {
		t.Error("an enemy standing in acid took no damage")
	}
}

// TestAcidGrantsNoInvulnerability is the exploit this has to avoid. Routing the
// burn through the ordinary damage path would hand out invulnerability frames,
// so standing in acid would make you briefly immune to everything else.
func TestAcidGrantsNoInvulnerability(t *testing.T) {
	g := acidGame(t, 41)
	g.player.pos = Vec{15.5, 4.5}
	g.player.invuln = 0

	// Burn once.
	g.acidTick = 0
	g.updateHazards(1.0 / 60)
	if g.player.invuln > 0 {
		t.Fatal("acid granted invulnerability frames; standing in it would be a defence")
	}

	// And a real hit still lands right after.
	hp := g.player.hp
	g.damagePlayer(10, Vec{1, 0})
	if g.player.hp >= hp {
		t.Error("a hit was absorbed by invulnerability the acid should not have given")
	}
}

// TestAcidSlowsMovement covers the other half of the hazard.
func TestAcidSlowsMovement(t *testing.T) {
	travel := func(inAcid bool) float64 {
		g := acidGame(t, 42)
		if !inAcid {
			for x := 14; x <= 18; x++ {
				g.level.tiles[g.level.idx(x, 4)] = TileFloor
			}
		}
		g.player.pos = Vec{15.5, 4.5}
		g.player.hp = 1e6 // survive the burn; we are measuring speed
		g.player.maxHP = 1e6
		start := g.player.pos.X
		for i := 0; i < 40; i++ {
			g.move.press(dirRight, g.elapsed, false)
			g.Update(1.0 / 60)
		}
		return g.player.pos.X - start
	}
	wet, dry := travel(true), travel(false)
	if wet >= dry {
		t.Errorf("moved %.2f through acid and %.2f on clear floor; acid must slow you",
			wet, dry)
	}
}

// TestAcidNeverCoversTheStairs stops a floor demanding a health toll to leave.
func TestAcidNeverCoversTheStairs(t *testing.T) {
	for seed := int64(1); seed <= 60; seed++ {
		g := NewGame(seed)
		for depth := 2; depth <= 5; depth++ {
			g.enterDepth(depth)
			if g.level.At(int(g.stairsPos.X), int(g.stairsPos.Y)) != TileStairs {
				t.Fatalf("seed %d B%d: the stairs tile was overwritten", seed, depth)
			}
		}
	}
}

// TestSpawnIsNeverInAcid guards the arrival point. Acid pools are painted
// during generation, before the player is placed, so nothing stops one landing
// on the spot every run starts from — 3.4% of floors did exactly that, dealing
// damage on arrival with no warning and no decision attached.
func TestSpawnIsNeverInAcid(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		g := NewGame(seed)
		for depth := 2; depth <= 6; depth++ {
			g.enterDepth(depth)
			if g.inAcid(g.player.pos) {
				t.Fatalf("seed %d B%d: the run starts standing in acid at %v",
					seed, depth, g.player.pos)
			}
		}
	}
}

// TestTerrainIsDocumented keeps the new pieces discoverable: three new glyphs
// appear on the map and none of them mean anything without the help screen.
func TestTerrainIsDocumented(t *testing.T) {
	g := NewGame(6)
	s := newTestScreen(120, 44)
	g.handleKey(Event{Kind: EvKey, Rune: '?'})
	g.handleKey(Event{Kind: EvKey, Key: KeyRight})
	g.handleKey(Event{Kind: EvKey, Key: KeyRight})
	g.Draw(s)

	joined := ""
	for y := 0; y < s.H; y++ {
		joined += screenRow(s, y) + "\n"
	}
	for _, want := range []string{"0 폭발통", "% 균열 벽", "~ 산성"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the help overlay never explains %q", want)
		}
	}
}

// TestFloorsHaveInteractiveTerrain checks the generator actually produces it.
func TestFloorsHaveInteractiveTerrain(t *testing.T) {
	barrels, cracked, acid := 0, 0, 0
	for seed := int64(1); seed <= 20; seed++ {
		g := NewGame(seed)
		g.enterDepth(4)
		barrels += len(g.barrels)
		for _, tile := range g.level.tiles {
			switch tile {
			case TileCracked:
				cracked++
			case TileAcid:
				acid++
			}
		}
	}
	if barrels == 0 {
		t.Error("no barrels on any floor")
	}
	if cracked == 0 {
		t.Error("no breakable walls on any floor")
	}
	if acid == 0 {
		t.Error("no acid on any floor")
	}
	t.Logf("across 20 B4 floors: %d barrels, %d cracked walls, %d acid tiles",
		barrels, cracked, acid)
}
