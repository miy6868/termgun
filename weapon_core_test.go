package main

import (
	"strings"
	"testing"
)

func TestActBossOffersCoreForKillingWeapon(t *testing.T) {
	g := NewGame(5050)
	g.depth = floorsPerAct
	g.enemies = g.enemies[:0]
	def := bossDef
	def.HP, def.XP = 1, 0
	g.addEnemy(def, g.player.pos.Add(Vec{4, 0}))
	g.enemies[0].hp = 0
	g.enemies[0].lastHitWeapon = wpPistol
	g.reapEnemies()

	if g.state != StateWeaponCore {
		t.Fatalf("boss death entered state %v, want core selection", g.state)
	}
	if g.coreWeapon != wpPistol || len(g.coreChoices) != len(weaponCores) {
		t.Fatalf("core offer weapon=%d choices=%v", g.coreWeapon, g.coreChoices)
	}

	g.handleKey(Event{Kind: EvKey, Rune: '1'})
	if !g.player.hasCore(wpPistol, CoreEcho) || g.state != StatePlaying {
		t.Fatalf("selecting echo left core=%v state=%v", g.player.cores[wpPistol], g.state)
	}
}

func TestCoreSelectionQueuesLevelUpFromSameFrame(t *testing.T) {
	g := NewGame(5051)
	g.depth = floorsPerAct
	g.enemies = g.enemies[:0]
	boss := bossDef
	boss.HP, boss.XP = 1, 0
	g.addEnemy(boss, g.player.pos.Add(Vec{4, 0}))
	grunt := enemyDefs[0]
	grunt.XP = g.player.xpNext
	g.addEnemy(grunt, g.player.pos.Add(Vec{6, 0}))
	for i := range g.enemies {
		g.enemies[i].hp = 0
		g.enemies[i].lastHitWeapon = wpPistol
	}
	g.reapEnemies()

	if g.state != StateWeaponCore || len(g.perkChoices) == 0 {
		t.Fatalf("same-frame rewards lost their order: state=%v perks=%d", g.state, len(g.perkChoices))
	}
	g.handleKey(Event{Kind: EvKey, Rune: '1'})
	if g.state != StateLevelUp {
		t.Fatalf("core selection resumed state %v, want level-up", g.state)
	}
}

func TestEchoCoreAddsEveryThirdVolley(t *testing.T) {
	g := NewGame(6060)
	g.enemies = g.enemies[:0]
	g.player.cores[wpPistol] = CoreEcho
	g.player.ammo[wpPistol] = 10
	g.player.weapon = wpPistol

	wantBullets := []int{1, 2, 4}
	for i, want := range wantBullets {
		g.shoot(g.player.pos, Vec{1, 0})
		if got := len(g.bullets); got != want {
			t.Fatalf("after trigger %d bullets=%d, want %d", i+1, got, want)
		}
	}
	if g.bullets[len(g.bullets)-1].color != 51 {
		t.Fatal("echo projectile is not visibly distinct")
	}
}

func TestRuptureCoreDamagesNearbyEnemy(t *testing.T) {
	g := NewGame(7070)
	g.enemies = g.enemies[:0]
	g.player.cores[wpPistol] = CoreRupture
	deadAt := g.player.pos.Add(Vec{5, 0})
	g.addEnemy(enemyDefs[0], deadAt)
	g.addEnemy(enemyDefs[0], deadAt.Add(Vec{2, 0}))
	g.enemies[0].hp = 0
	g.enemies[0].lastHitWeapon = wpPistol
	before := g.enemies[1].hp
	g.reapEnemies()

	if len(g.enemies) != 1 || g.enemies[0].hp >= before {
		t.Fatalf("rupture left nearby enemy hp %.1f, want below %.1f", g.enemies[0].hp, before)
	}
	if g.enemies[0].lastHitWeapon != wpPistol {
		t.Fatal("rupture did not preserve weapon attribution for a chain")
	}
}

func TestCoreMarkerKeepsEveryWeaponVisibleAtMinimumWidth(t *testing.T) {
	g := NewGame(8080)
	g.player.cores[wpPistol] = CoreEcho
	s := newTestScreen(minCols, minRows)
	g.Draw(s)
	row := screenRow(s, 1)
	for _, want := range []string{"1PIST*", "2SMG", "3SHOT", "4RAIL", "5LNCH", "6MELE", "[?]"} {
		if !strings.Contains(row, want) {
			t.Errorf("cored HUD is missing %q from %q", want, row)
		}
	}
}
