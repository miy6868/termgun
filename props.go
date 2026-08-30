package main

import "math"

// The dungeon itself joins the fight here. Barrels, cracked walls and acid all
// exist so that where you stand and what you shoot matter as much as which gun
// you are holding.

const (
	barrelHP     = 12
	barrelFuse   = 0.28 // short delay, so a chain reads as a chain
	barrelRadius = 5.2
	barrelDamage = 42

	acidTickTime = 0.45
	acidDamage   = 5.0
	// Acid slows whatever is standing in it, which is what makes stepping
	// around a pool a real choice rather than a small tax.
	acidSlow = 0.62
)

// Barrel is a stationary explosive. It is not an enemy: it never acts, and
// shooting it is meant to feel like using the room rather than killing a thing.
type Barrel struct {
	pos   Vec
	hp    float64
	fuse  float64 // >0 once lit; explodes at zero
	lit   bool
	spent bool // already detonated, awaiting removal
}

// placeBarrels scatters barrels, biased towards rooms with enemies in them so
// there is usually something worth luring.
func (g *Game) placeBarrels(sx, sy int) {
	g.barrels = g.barrels[:0]
	n := 4 + g.depth
	for i := 0; i < n; i++ {
		x, y := g.level.FreeSpot(g.rng, sx, sy, 10)
		p := Vec{x, y}
		if g.level.At(int(x), int(y)) == TileAcid {
			continue
		}
		tooClose := false
		for _, b := range g.barrels {
			if vdist(b.pos, p) < 2.5 {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		g.barrels = append(g.barrels, Barrel{pos: p, hp: barrelHP})
	}
}

// hurtBarrel lights the fuse. Damage does not have to finish the barrel off —
// anything that hits it hard enough starts the countdown.
func (g *Game) hurtBarrel(b *Barrel, dmg float64) {
	if b.lit || b.spent {
		return
	}
	b.hp -= dmg
	if b.hp <= 0 {
		b.lit, b.fuse = true, barrelFuse
		g.spawnParticles(b.pos, 4, 0.2, 214, '!')
	}
}

// updateBarrels burns fuses and detonates. The delay is what turns a row of
// barrels into a visible chain instead of one instant blast.
func (g *Game) updateBarrels(dt float64) {
	blown := false
	for i := range g.barrels {
		b := &g.barrels[i]
		if !b.lit {
			continue
		}
		b.fuse -= dt
		if b.fuse <= 0 {
			// Mark it spent before the blast so it cannot re-trigger itself.
			// Removal is by this flag and not by hp: a barrel lit by a nearby
			// blast can be driven well below zero health, and culling on that
			// would delete it before its own fuse ever ran out — the chain
			// would stop dead after the second link.
			b.spent = true
			g.explode(b.pos, barrelRadius, barrelDamage, false)
			blown = true
		}
	}
	if !blown {
		return
	}
	out := g.barrels[:0]
	for _, b := range g.barrels {
		if !b.spent {
			out = append(out, b)
		}
	}
	g.barrels = out
}

// barrelAt finds a barrel a point has run into.
func (g *Game) barrelAt(p Vec, radius float64) *Barrel {
	for i := range g.barrels {
		b := &g.barrels[i]
		if b.spent {
			continue
		}
		if vdist(p, b.pos) < radius+0.45 {
			return b
		}
	}
	return nil
}

// ---- hazards ----------------------------------------------------------------

// inAcid reports whether a world point stands in a pool.
func (g *Game) inAcid(p Vec) bool {
	return g.level.At(int(p.X), int(p.Y)) == TileAcid
}

// updateHazards burns everything standing in acid, on both sides. Enemies path
// straight through it, so a pool is a weapon the player can fight around.
func (g *Game) updateHazards(dt float64) {
	g.acidTick -= dt
	if g.acidTick > 0 {
		return
	}
	g.acidTick = acidTickTime

	if g.inAcid(g.player.pos) {
		// Deliberately not damagePlayer: that grants invulnerability frames,
		// and standing in acid to become briefly immune to everything else
		// would turn a hazard into a defensive move.
		g.burnPlayer(acidDamage)
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if g.inAcid(e.pos) {
			e.hp -= acidDamage
			e.sinceHurt = 0 // also stops a regenerating elite healing in a pool
			g.spawnParticles(e.pos, 2, 0.2, 47, '`')
		}
	}
	for i := range g.barrels {
		if b := &g.barrels[i]; !b.lit && g.inAcid(b.pos) {
			g.hurtBarrel(b, acidDamage)
		}
	}
	g.acidBubbles()
}

// acidBubbles lets visible pool tiles pop the occasional bubble, so acid reads
// as liquid rather than a flat green carpet. Runs once per hazard tick, so the
// scan is cheap and the RNG spend stays deterministic.
func (g *Game) acidBubbles() {
	lv := g.level
	px, py := int(g.player.pos.X), int(g.player.pos.Y)
	n := 0
	for y := maxInt(py-18, 0); y <= minInt(py+18, lv.H-1) && n < 3; y++ {
		for x := maxInt(px-24, 0); x <= minInt(px+24, lv.W-1) && n < 3; x++ {
			i := y*lv.W + x
			if lv.tiles[i] != TileAcid || !lv.visible[i] || g.fxRNG.Float64() >= acidBubbleChance {
				continue
			}
			n++
			g.parts = append(g.parts, Particle{
				pos:  Vec{float64(x) + g.fxRNG.Float64(), float64(y) + g.fxRNG.Float64()},
				vel:  Vec{0, -0.8},
				life: 0.6, max: 0.6,
				glyph: '`', color: 47,
			})
		}
	}
}

// burnPlayer applies damage that ignores, and does not grant, invulnerability.
func (g *Game) burnPlayer(dmg float64) {
	p := &g.player
	if g.state != StatePlaying {
		return
	}
	p.hp -= dmg * p.damageTaken
	g.flash = math.Max(g.flash, 0.10)
	g.spawnParticles(p.pos, 4, 0.25, 47, '`')
	if p.hp <= 0 {
		p.hp = 0
		g.log("산에 녹아내렸다...", 47)
	}
}
