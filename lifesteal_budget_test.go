package main

import (
	"math"
	"testing"
)

func TestLifestealBudgetCapsBurstAndRefills(t *testing.T) {
	g := arena(t, 81)
	g.player.hp = 1
	g.player.lifesteal = 1
	g.addEnemy(enemyDefs[5], g.player.pos.Add(Vec{2, 0}))
	cap := g.player.lifestealCapPerSecond()
	g.hurtEnemy(&g.enemies[0], 20, Vec{})
	g.hurtEnemy(&g.enemies[0], 20, Vec{})
	if got := g.player.hp - 1; math.Abs(got-cap) > 1e-9 {
		t.Fatalf("burst healed %.3f HP; budget is %.3f", got, cap)
	}
	g.enemies = g.enemies[:0]
	g.player.invuln = 10
	g.Update(0.5)
	if got := g.player.lifestealBudget; math.Abs(got-cap*0.5) > 1e-9 {
		t.Fatalf("half-second refill = %.3f, want %.3f", got, cap*0.5)
	}
}

func TestLifestealCapScalesWithProgression(t *testing.T) {
	p := newPlayer()
	base := p.lifestealCapPerSecond()
	p.level, p.maxHP = 20, 180
	late := p.lifestealCapPerSecond()
	if math.Abs(base-2.5) > 1e-9 || late < 7 || late > 7.2 {
		t.Fatalf("unexpected cap scaling: early %.2f late %.2f", base, late)
	}
}

func TestSecondaryDamageDoesNotLifesteal(t *testing.T) {
	g := arena(t, 82)
	g.player.hp = 10
	g.player.lifesteal = 1
	g.addEnemy(enemyDefs[5], g.player.pos.Add(Vec{2, 0}))
	g.hurtEnemyFrom(&g.enemies[0], 20, Vec{}, damageSecondary)
	if g.player.hp != 10 {
		t.Fatalf("secondary damage healed the player to %.1f", g.player.hp)
	}
}
