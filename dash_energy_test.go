package main

import (
	"math"
	"testing"
)

func TestDashEnergyAllowsThreeShortSteps(t *testing.T) {
	g := arena(t, 61)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{10, 0})
	for i := 0; i < 3; i++ {
		g.startDash()
		if g.player.dashTimer <= 0 {
			t.Fatalf("dash %d did not start", i+1)
		}
		g.Update(dashDuration + dashRecovery + 0.01)
	}
	if got := g.player.dashEnergy; math.Abs(got-10) > 0.01 {
		t.Fatalf("three dashes left %.2f energy; want 10", got)
	}
	g.startDash()
	if g.player.dashTimer > 0 {
		t.Fatal("a fourth dash started without enough energy")
	}
}

func TestDashDistanceIsShortAndFixed(t *testing.T) {
	g := arena(t, 62)
	g.mouseSet = true
	start := g.player.pos
	g.aim = start.Add(Vec{10, 0})
	g.startDash()
	g.Update(dashDuration + dashRecovery)
	if got := vdist(start, g.player.pos); math.Abs(got-dashDistance) > 0.15 {
		t.Fatalf("dash moved %.2f visual units; want %.2f", got, dashDistance)
	}
}

func TestTightDashChainAcceleratesAndExtendsSteps(t *testing.T) {
	g := arena(t, 65)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{20, 0})

	wantSpeed := []float64{
		dashDistance / dashDuration,
		(dashDistance / dashDuration) * 1.25,
		(dashDistance / dashDuration) * 1.50,
	}
	for i, want := range wantSpeed {
		// Keep every measured step away from the arena wall while preserving
		// the live momentum counter between presses.
		g.player.pos = Vec{10.5, 4.5}
		start := g.player.pos
		g.startDash()
		wantDistance := []float64{dashDistance, 4.10, 4.50}[i]
		if math.Abs(g.player.dashSpeed-want) > 1e-9 {
			t.Fatalf("dash %d speed = %.2f, want %.2f", i+1, g.player.dashSpeed, want)
		}
		if math.Abs(g.player.dashStepDistance-wantDistance) > 1e-9 {
			t.Fatalf("dash %d distance = %.2f, want %.2f", i+1, g.player.dashStepDistance, wantDistance)
		}
		g.Update(g.player.dashTimer)
		if got := vdist(start, g.player.pos); math.Abs(got-wantDistance) > 0.15 {
			t.Fatalf("accelerated dash %d moved %.2f, want %.2f", i+1, got, wantDistance)
		}
		g.Update(dashRecovery)
	}
}

func TestDashStraightnessScalesDirectionBonus(t *testing.T) {
	right := Vec{1, 0}
	diagonal := Vec{1, 1}.norm()
	if got := dashStraightness(right, right); math.Abs(got-1) > 1e-9 {
		t.Fatalf("same-direction straightness = %.2f, want 1", got)
	}
	if got := dashStraightness(right, diagonal); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("45-degree straightness = %.2f, want 0.5", got)
	}
	for name, dir := range map[string]Vec{"perpendicular": {0, 1}, "reverse": {-1, 0}} {
		if got := dashStraightness(right, dir); got != 0 {
			t.Fatalf("%s straightness = %.2f, want 0", name, got)
		}
	}
}

func TestLateDashLosesMomentumBonus(t *testing.T) {
	g := arena(t, 66)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{20, 0})
	g.startDash()
	g.Update(g.player.dashTimer)
	g.Update(dashMomentumFadeGap + 0.01)
	g.startDash()
	if g.player.dashMomentum != 1 || math.Abs(g.player.dashSpeed-dashDistance/dashDuration) > 1e-9 {
		t.Fatalf("late dash retained momentum: chain %d speed %.2f",
			g.player.dashMomentum, g.player.dashSpeed)
	}
}

func TestDashRecoveryCarriesVulnerableMomentum(t *testing.T) {
	g := arena(t, 67)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{10, 0})
	g.startDash()
	g.Update(g.player.dashTimer)
	start := g.player.pos
	dt := dashRecovery / 2
	g.Update(dt)
	if got := vdist(start, g.player.pos); got <= g.player.baseSpeed()*dt ||
		got > g.player.baseSpeed()*dashCoastStartMul*dt+0.01 {
		t.Fatalf("recovery coast moved %.2f", got)
	}
	if g.player.invuln > 0 {
		t.Fatal("recovery coast restored dash invulnerability")
	}
}

func TestSingleDashCoastSettlesAtRunSpeed(t *testing.T) {
	g := arena(t, 69)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{10, 0})
	g.startDash()
	g.Update(g.player.dashTimer)
	start := g.player.pos
	g.Update(dashCoastDuration)
	want := g.player.baseSpeed() * dashCoastDuration * (1 + (dashCoastStartMul-1)/3)
	if got := vdist(start, g.player.pos); math.Abs(got-want) > 0.05 {
		t.Fatalf("full coast moved %.2f, want %.2f", got, want)
	}
	if g.player.dashCoast != 0 || math.Abs(g.player.vel.len()-g.player.baseSpeed()) > 1e-9 {
		t.Fatalf("single dash did not settle at run speed: coast %.3f speed %.2f",
			g.player.dashCoast, g.player.vel.len())
	}
}

func TestBufferedChainStartsMovementAndInvulnerabilityTogether(t *testing.T) {
	g := arena(t, 68)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{20, 0})
	g.startDash()
	g.startDash() // queue the second press during the first step
	g.Update(g.player.dashTimer)
	g.Update(dashRecovery)
	if g.player.dashTimer > 0 {
		t.Fatal("buffered dash started at the tail of the recovery frame")
	}
	start := g.player.pos
	g.Update(1.0 / 60)
	if g.player.dashMomentum != 2 || g.player.dashTimer <= 0 {
		t.Fatalf("buffered chain did not start on the next frame: chain %d timer %.3f",
			g.player.dashMomentum, g.player.dashTimer)
	}
	if g.player.invuln <= 0 || vdist(start, g.player.pos) <= 0 {
		t.Fatal("buffered dash movement and invulnerability did not begin together")
	}
}

func TestDashRegenSpikesOnlyAtCriticalHealth(t *testing.T) {
	p := newPlayer()
	base := p.dashRegenPerSecond()
	p.hp = p.maxHP * 0.25
	atQuarter := p.dashRegenPerSecond()
	p.hp = p.maxHP * 0.10
	atCritical := p.dashRegenPerSecond()
	if atQuarter > base*1.02 {
		t.Fatalf("25%% HP regen rose too early: %.2f -> %.2f", base, atQuarter)
	}
	if atCritical < base*1.4 {
		t.Fatalf("10%% HP regen did not spike enough: %.2f -> %.2f", base, atCritical)
	}
}

func TestDashRecoveryIsVulnerable(t *testing.T) {
	g := arena(t, 63)
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{10, 0})
	g.startDash()
	g.Update(dashDuration)
	if g.player.dashRecovery <= 0 || g.player.invuln > 0 {
		t.Fatalf("dash ended without a vulnerable recovery: recovery %.2f invuln %.2f",
			g.player.dashRecovery, g.player.invuln)
	}
	before := g.player.hp
	g.startDash()
	if g.player.dashTimer > 0 {
		t.Fatal("a second dash skipped the recovery gap")
	}
	g.damagePlayer(5, Vec{1, 0})
	if g.player.hp >= before {
		t.Fatal("the recovery gap still absorbed damage")
	}
}

func TestDashStrikeRepeatCooldown(t *testing.T) {
	g := arena(t, 64)
	g.addEnemy(enemyDefs[5], g.player.pos)
	e := &g.enemies[0]
	g.player.dashDir = Vec{1, 0}
	g.elapsed = 1
	g.dashStrike()
	first := e.hp
	g.player.dashHitIDs = g.player.dashHitIDs[:0]
	g.elapsed += dashRepeatHitCD - 0.01
	g.dashStrike()
	if e.hp != first {
		t.Fatal("the same enemy was hit twice inside the dash strike cooldown")
	}
	g.player.dashHitIDs = g.player.dashHitIDs[:0]
	g.elapsed += 0.02
	g.dashStrike()
	if e.hp >= first {
		t.Fatal("dash strike stayed locked after its repeat cooldown")
	}
}
