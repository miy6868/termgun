package main

// The juice layer: everything here reshapes how the simulation is *shown* or
// how forgiving the controls feel, without touching simulation outcomes.
// Effects are spawned only from Update paths so that a frame rate change (or
// the terminal redraw cadence) can never perturb the seeded RNG stream.

const (
	floaterLife = 0.7 // seconds a damage number stays on screen
	floaterRise = 1.8 // tiles a damage number climbs per second
	floaterCap  = 48  // beyond this, the oldest numbers are dropped

	decalLife = 14.0 // seconds a corpse mark takes to fade out
	decalCap  = 96   // oldest marks are recycled beyond this

	killSlowmo      = 0.05 // seconds of slow motion granted per kill
	bossSlowmo      = 0.16 // a boss death earns a longer beat
	killSlowmoScale = 0.25 // simulation speed while slow motion lasts

	dashBufferTime       = 0.25 // covers one base dash plus its vulnerable recovery
	fireBufferTime       = 0.15 // same for a trigger tap landed during cooldown
	quickMeleeBufferTime = 0.20 // lets the emergency swing follow a nearly-ready shot

	// Dash energy: one press is a short committed step. Repeated presses spend
	// the shared pool, making distance an explicit player choice.
	dashEnergyBase      = 100.0
	dashEnergyCost      = 30.0
	dashRegenBase       = 18.0
	dashRegenDelay      = 0.35
	dashDuration        = 0.10
	dashDistance        = 3.5
	dashDistanceStep    = 0.25
	dashRecovery        = 0.08
	dashCoastDuration   = 0.14
	dashCoastStartMul   = 1.50
	dashChainWindow     = 0.60
	dashRepeatHitCD     = 0.30
	dashMomentumStep    = 0.25
	dashMomentumFullGap = 0.14
	dashMomentumFadeGap = 0.40
	dashStraightBonus2  = 0.35
	dashStraightBonus3  = 0.50

	// Perfect dodge: a hit absorbed during a young dash pays out. The window is
	// deliberately wider than the short step, but the invulnerability itself
	// ends with the step, leaving recovery vulnerable.
	perfectDodgeWindow = 0.30 // seconds after the dash starts
	perfectDodgeSlowmo = 0.32 // seconds of bullet time granted
	perfectDodgeEnergy = 20.0 // energy returned once per dash chain

	stairsShimmerChance = 0.10 // per-frame chance of a mote rising off the stairs

	lowHPVignette = 0.30 // HP fraction below which the screen border pulses

	// Loot magnetism: pickups this close drift towards the player, so ending a
	// fight on top of a drop never turns into pixel hunting for the last medkit.
	pickupMagnetRange = 2.6
	pickupMagnetSpeed = 7.0

	hitDirTime = 0.55 // seconds the "damage came from here" marker lasts

	// Dash strike: the blink is also a shoulder charge. Damage is deliberately
	// melee-tier, so dashing through a Swarmling pack clears it but charging a
	// Brute head-on is still a mistake.
	dashStrikeDamage = 16.0
	dashStrikeKnock  = 9.0

	acidBubbleChance = 0.10 // per-tick chance per visible pool tile

	bossBarMaxW = 40 // widest the boss health gauge may get

	// Lighting falloff bands as squared distances from the player, measured in
	// visual space. Squared comparisons keep a sqrt out of the hottest loop in
	// the renderer, which runs once per visible tile per frame.
	lightNear2 = 42.0
	lightMid2  = 150.0
)

// addFloater queues one damage number, recycling the oldest past the cap so a
// shotgun blast into a crowd cannot grow the slice without bound.
func (g *Game) addFloater(pos Vec, text string, color int16) {
	if len(g.floaters) >= floaterCap {
		g.floaters = g.floaters[1:]
	}
	g.floaters = append(g.floaters, FloatText{
		pos: pos, life: floaterLife, max: floaterLife, text: text, color: color,
	})
}

// addDecal drops a fading corpse mark at a death site.
func (g *Game) addDecal(pos Vec) {
	if len(g.decals) >= decalCap {
		g.decals = g.decals[1:]
	}
	g.decals = append(g.decals, Decal{pos: pos, life: decalLife, max: decalLife})
}

// updateFX ages the presentation-only actors. It runs inside the slowed dt, so
// during kill slow motion the numbers and sparks hang in the air with it.
func (g *Game) updateFX(dt float64) {
	out := g.floaters[:0]
	for _, f := range g.floaters {
		f.life -= dt
		if f.life <= 0 {
			continue
		}
		f.pos.Y -= floaterRise * dt
		out = append(out, f)
	}
	g.floaters = out

	dout := g.decals[:0]
	for _, d := range g.decals {
		d.life -= dt
		if d.life > 0 {
			dout = append(dout, d)
		}
	}
	g.decals = dout
}

// lightTier grades how brightly a visible tile reads by distance from the
// player: 0 is full brightness, 2 is the dimmest lit band.
func lightTier(dx, dy float64) int {
	d2 := dx*dx + dy*dy*aspect*aspect
	switch {
	case d2 < lightNear2:
		return 0
	case d2 < lightMid2:
		return 1
	}
	return 2
}

// lightTierAt grades one tile of the level for the lighting pass.
func (g *Game) lightTierAt(wx, wy int) int {
	return lightTier(float64(wx)+0.5-g.player.pos.X, float64(wy)+0.5-g.player.pos.Y)
}

// stairsShimmer spawns the occasional bright mote drifting up from the stairs,
// so the exit draws the eye even in a busy room. Called from Update only, for
// RNG determinism.
func (g *Game) stairsShimmer() {
	if !g.level.Visible(int(g.stairsPos.X), int(g.stairsPos.Y)) {
		return
	}
	if g.fxRNG.Float64() >= stairsShimmerChance {
		return
	}
	g.parts = append(g.parts, Particle{
		pos:  g.stairsPos.Add(Vec{g.fxRNG.Float64()*0.8 - 0.4, g.fxRNG.Float64()*0.4 - 0.2}),
		vel:  Vec{(g.fxRNG.Float64() - 0.5) * 0.6, -1.2},
		life: 0.55, max: 0.55,
		glyph: '.', color: colStairs,
	})
}
