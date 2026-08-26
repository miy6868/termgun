package main

import "fmt"

// Descent pressure exists because "kill everything, then take the stairs" was
// always the right move, and a roguelike whose central decision has one correct
// answer is not really asking anything. Loitering has to cost something.
const (
	// The floor bonus decays to its floor over this many seconds.
	bonusDecayTime = 100.0
	minBonusMul    = 0.25
	// Reinforcements start arriving after this long and repeat on this period.
	reinforceAfter  = 70.0
	reinforceEvery  = 20.0
	reinforceWarn   = 8.0
	maxReinforceMsg = 3
)

// descentMul is the share of the floor bonus still on the table.
func (g *Game) descentMul() float64 {
	return clampF(1-g.floorTime/bonusDecayTime, minBonusMul, 1)
}

// pressureFrac is 0 before reinforcements begin and 1 once they are due, so the
// HUD can show the wait filling up rather than springing a surprise.
func (g *Game) pressureFrac() float64 {
	if g.floorTime < reinforceAfter {
		return clampF(g.floorTime/reinforceAfter, 0, 1)
	}
	return 1
}

// updatePressure ages the floor and calls in reinforcements.
func (g *Game) updatePressure(dt float64) {
	g.floorTime += dt
	if g.floorTime < reinforceAfter {
		// One warning shortly before the first wave, so the mechanic announces
		// itself instead of just making the floor mysteriously harder.
		if g.floorTime+reinforceWarn >= reinforceAfter && g.reinfWaves == 0 {
			g.reinfWaves = -1 // marker: warned but nothing sent yet
			g.log("아래에서 무언가 올라오고 있다. 오래 머무를수록 불리하다.", 208)
		}
		return
	}
	g.reinfTimer -= dt
	if g.reinfTimer > 0 {
		return
	}
	g.reinfTimer = reinforceEvery
	if g.reinfWaves < 0 {
		g.reinfWaves = 0
	}
	g.reinfWaves++
	g.sendReinforcements()
}

// sendReinforcements spawns a wave away from the player, growing each time.
func (g *Game) sendReinforcements() {
	n := 2 + g.reinfWaves
	px, py := int(g.player.pos.X), int(g.player.pos.Y)
	spawned := 0
	for i := 0; i < n; i++ {
		def, ok := g.rollEnemy(g.depth + g.reinfWaves)
		if !ok {
			continue
		}
		// Well out of sight: arriving on top of the player would be a gotcha
		// rather than pressure.
		x, y := g.level.FreeSpot(g.rng, px, py, 26)
		g.addEliteEnemy(scaleForDepth(def, g.depth), Vec{x, y}, g.rollElite(g.depth+2))
		spawned++
	}
	if spawned > 0 && g.reinfWaves <= maxReinforceMsg {
		g.log(fmt.Sprintf("증원 %d체 도착 - 계단으로 (%d/%d)",
			spawned, g.reinfWaves, maxReinforceMsg), 203)
	}
}
