package main

import "math"

type Behavior int

const (
	BeChaser  Behavior = iota // runs straight at you and melees
	BeShooter                 // keeps its distance and fires
	BeBomber                  // fast, detonates on contact
	BeTurret                  // immobile, fires spreads through corridors
	BeSwarm                   // fast, fragile, arrives in packs
	BeBoss                    // heavy: spreads, charges, spawns swarm
	BeHealer                  // flees you, heals its wounded allies
)

// Which attack script a boss runs. The field defaults to the OVERSEER so older
// definitions need no change.
const (
	bossOverseer int = iota
	bossQueen
	bossCore
)

type EnemyDef struct {
	Name     string
	Glyph    rune
	Color    int16
	HP       float64
	Speed    float64 // visual units / sec
	Damage   float64
	Radius   float64
	Behavior Behavior
	FireRate float64
	BulletSp float64
	Sight    float64
	XP       int
	Score    int
	MinDepth int
	Weight   int

	// Windup is how long the enemy visibly prepares before its committed
	// attack. Without it, dodging is reaction time; with it, dodging is a
	// decision, and the shotgun's knockback, the dash's invulnerability and
	// each weapon's range all become answers to something instead of just
	// different damage numbers.
	Windup float64
	// Charge turns a chaser into a rush: it locks a direction during the
	// windup, sprints along it, and is stunned if it hits a wall.
	Charge bool
	// KnockMul scales how far this enemy is pushed when hit. Light bodies can
	// be shoved off their attack; heavy ones cannot.
	KnockMul float64

	// BossKind picks the attack script for BeBoss enemies.
	BossKind int

	// Dodge marks a light body that reads an incoming dash and sidesteps it.
	// Deliberately rare: if everything dodges, the dash strike stops being an
	// answer to anything.
	Dodge bool
}

// knockMul defaults to 1 so definitions only mention it when it matters.
func (d *EnemyDef) knockMul() float64 {
	if d.KnockMul == 0 {
		return 1
	}
	return d.KnockMul
}

var enemyDefs = []EnemyDef{
	{Name: "Grunt", Glyph: 'g', Color: 77, HP: 26, Speed: 7.5, Damage: 9, Radius: 0.45,
		Behavior: BeChaser, Sight: 22, XP: 3, Score: 10, MinDepth: 1, Weight: 30},
	{Name: "Swarmling", Glyph: 'w', Color: 148, HP: 10, Speed: 12.0, Damage: 5, Radius: 0.34,
		Behavior: BeSwarm, Sight: 26, XP: 1, Score: 5, MinDepth: 1, Weight: 22, Dodge: true},
	{Name: "Spitter", Glyph: 's', Color: 141, HP: 22, Speed: 6.0, Damage: 8, Radius: 0.45,
		Behavior: BeShooter, FireRate: 1.5, BulletSp: 22, Sight: 26, XP: 4, Score: 15, MinDepth: 2, Weight: 24,
		Windup: 0.30},
	{Name: "Bomber", Glyph: 'b', Color: 208, HP: 18, Speed: 13.0, Damage: 26, Radius: 0.45,
		Behavior: BeBomber, Sight: 24, XP: 4, Score: 18, MinDepth: 3, Weight: 14,
		KnockMul: 4.5, Dodge: true},
	{Name: "Turret", Glyph: 'T', Color: 244, HP: 46, Speed: 0, Damage: 10, Radius: 0.48,
		Behavior: BeTurret, FireRate: 1.1, BulletSp: 26, Sight: 30, XP: 5, Score: 20, MinDepth: 3, Weight: 12,
		Windup: 0.50},
	{Name: "Brute", Glyph: 'B', Color: 160, HP: 90, Speed: 6.5, Damage: 18, Radius: 0.6,
		Behavior: BeChaser, Sight: 26, XP: 9, Score: 40, MinDepth: 4, Weight: 16,
		Windup: 0.55, Charge: true, KnockMul: 0.35},
	{Name: "Sniper", Glyph: 'S', Color: 45, HP: 30, Speed: 7.0, Damage: 16, Radius: 0.45,
		Behavior: BeShooter, FireRate: 2.2, BulletSp: 46, Sight: 40, XP: 7, Score: 30, MinDepth: 5, Weight: 14,
		Windup: 0.60},
	// The Medic is a priority target wearing a weak body. It never attacks:
	// the pressure it makes comes entirely from undoing the player's damage,
	// so killing it first is the puzzle.
	{Name: "Medic", Glyph: 'm', Color: 48, HP: 34, Speed: 8.2, Damage: 7, Radius: 0.45,
		Behavior: BeHealer, Sight: 24, XP: 6, Score: 25, MinDepth: 4, Weight: 12,
		KnockMul: 1.4, Dodge: true},
}

const (
	// How long a bomber's fuse burns once lit. It never goes out, so the answer
	// is to get away from it or shove it back with knockback.
	bomberFuse = 1.10
	// A charge that ends against a wall leaves the enemy open for this long.
	chargeStun = 1.30
	chargeTime = 0.85
	chargeMul  = 2.8
)

// Medic tuning: the healer has to be faster than what it heals but slower than
// a Swarmling, and its heal must stay a trickle — a medic that out-heals a
// focused player turns every fight into a stalemate.
const (
	medicPanicDist = 6.5  // flee the player inside this visual range
	medicHealRange = 3.2  // heal allies within this visual range
	medicSeekRange = 30.0 // how far it will travel to reach a patient
	medicHealFrac  = 0.09 // share of the patient's max HP restored per second
)

// The MATRIARCH trades raw durability for bodies: she spawns her own escorts,
// sweeps a rotating fan across the room, and charges like a Brute when you try
// to hold mid-range. She visits the deeper boss floors (B10, B20, ...) while
// the OVERSEER keeps B5, B15, ...
var queenDef = EnemyDef{
	Name: "MATRIARCH", Glyph: 'Q', Color: 207, HP: 330, Speed: 5.2, Damage: 17, Radius: 0.9,
	Behavior: BeBoss, FireRate: 1.35, BulletSp: 23, Sight: 60, XP: 70, Score: 650,
	Windup: 0.60, Charge: true, KnockMul: 0.30, BossKind: bossQueen,
}

// queenChargeCD keeps the rush from chaining into itself; without it one bad
// spacing lets her pin the player against a wall indefinitely.
const queenChargeCD = 5.0

// Dash reads and pincer pursuit: both exist so a pack of bodies is an
// arrangement to out-think rather than a queue to mow down. The dodge window
// has to beat the blink's travel time from the edge of the trigger range, or
// the reaction is theatre; the burst multiplier pays for that.
const (
	dashDodgeRange    = 3.6  // react to a dash aimed at the body inside this range
	dashDodgeAlign    = 0.72 // how head-on the dash must be to be worth dodging
	dashDodgeTime     = 0.18
	dashDodgeCooldown = 1.2 // then it is committed and can be punished
	dashDodgeMul      = 1.9 // speed burst while hopping aside

	flankAngle      = 0.9  // radians a distant chaser curves off the beeline
	flankBlendRange = 10.0 // distance over which the curve flattens to zero
)

// flankCurve is how far off the straight line a chaser should hunt at `dist`.
// It decays to zero at melee range, so the pack converges instead of orbiting.
func flankCurve(flank, dist float64) float64 {
	return flank * flankAngle * clampF((dist-2.5)/flankBlendRange, 0, 1)
}

// The boss is tuned a touch under what its stats imply: it already wins by
// outlasting, and a wall of HP that also hits hardest reads as unfair rather
// than as a duel.
var bossDef = EnemyDef{
	Name: "OVERSEER", Glyph: 'O', Color: 196, HP: 400, Speed: 5.5, Damage: 20, Radius: 0.9,
	Behavior: BeBoss, FireRate: 1.1, BulletSp: 24, Sight: 60, XP: 60, Score: 500,
}

var coreBossDef = EnemyDef{
	Name: "CORE WARDEN", Glyph: 'C', Color: 51, HP: 520, Speed: 5.0, Damage: 22, Radius: 1.0,
	Behavior: BeBoss, FireRate: 1.0, BulletSp: 27, Sight: 60, XP: 100, Score: 1200,
	KnockMul: 0.18, BossKind: bossCore,
}

type Enemy struct {
	id     int
	def    *EnemyDef
	pos    Vec
	vel    Vec
	hp     float64
	maxHP  float64
	cool   float64
	hurt   float64 // flash timer
	alert  bool
	fuse   float64 // bombers
	wander Vec
	phase  float64
	// lastHitWeapon attributes a kill to a weapon core. Environmental and
	// hostile damage leave it at -1 so swapping guns cannot steal the reward.
	lastHitWeapon int

	// telegraph state
	windup  float64 // counts down while the attack is being shown
	lockDir Vec     // direction committed when the windup started
	charge  float64 // remaining charge time
	stun    float64 // helpless after a charge into a wall

	// elite state
	elite     EliteKind
	sinceHurt float64
	burstCD   float64
	buffed    float64 // seconds of remaining aura buff
	// shield is the EliteShield pool: it soaks damage first and refills only
	// after the enemy has been left alone for a while.
	shield float64

	// healer bookkeeping: a particle cadence for the heal beam, and the
	// queen's separate cooldown for her charge rush.
	healTick float64
	chargeCD float64

	// dash-read state: a light body hopping aside from a blink, and the side
	// this body prefers when a pack fans out around the player. The flank is
	// derived from the id so packs split evenly without touching the RNG.
	dodgeT      float64
	dodgeCD     float64
	dodgeVec    Vec
	flank       float64
	lastDashHit float64
}

// telegraphing reports whether the enemy is visibly preparing an attack, which
// is both what the renderer draws and what the player is meant to react to.
func (e *Enemy) telegraphing() bool { return e.windup > 0 }

// damage is the enemy's outgoing damage including any aura buff.
func (e *Enemy) damage() float64 {
	if e.buffed > 0 {
		return e.def.Damage * auraDmgMul
	}
	return e.def.Damage
}

func (e *Enemy) speed() float64 {
	if e.buffed > 0 {
		return e.def.Speed * auraSpeedMul
	}
	return e.def.Speed
}

// scaleForDepth toughens enemies as the run goes deeper.
func scaleForDepth(def EnemyDef, depth int) EnemyDef {
	f := 1 + float64(depth-1)*0.16
	def.HP *= f
	def.Damage *= 1 + float64(depth-1)*0.09
	def.Speed *= 1 + float64(depth-1)*0.015
	return def
}

func (g *Game) updateEnemy(e *Enemy, dt float64) {
	p := &g.player
	toPlayer := p.pos.Sub(e.pos)
	dist := toPlayer.visual().len()
	dir := toPlayer.visual().norm()
	e.phase += dt
	e.sinceHurt += dt
	for _, t := range []*float64{&e.hurt, &e.cool, &e.stun, &e.burstCD, &e.buffed, &e.chargeCD, &e.dodgeCD} {
		if *t > 0 {
			*t -= dt
		}
	}
	g.updateElite(e, dt)

	los := dist < e.def.Sight && g.level.LineOfSight(e.pos, p.pos)
	if los {
		e.alert = true
	}

	// A stunned enemy does nothing at all. That window is the reward for
	// dodging a charge, so it has to be long enough to actually use.
	if e.stun > 0 {
		return
	}
	// A charge is committed: it runs along the direction locked at the windup,
	// which is what makes stepping aside work.
	if e.charge > 0 {
		g.updateCharge(e, dt, dist, dir)
		return
	}

	// Dash read: a light body sees the blink coming straight at it and hops
	// aside. Geometric side pick (no RNG), so the same inputs always scatter
	// the same way. Heavies and telegraphing bodies do not bother — eating a
	// dash strike has to stay possible, or the move loses its answer value.
	if e.def.Dodge && !e.telegraphing() && e.dodgeCD <= 0 && p.dashTimer > 0 {
		rel := e.pos.Sub(p.pos).visual()
		if d := rel.len(); d < dashDodgeRange && d > 0.01 {
			if n := rel.norm(); n.X*p.dashDir.X+n.Y*p.dashDir.Y > dashDodgeAlign {
				perp := Vec{-p.dashDir.Y, p.dashDir.X}
				if p.dashDir.X*n.Y-p.dashDir.Y*n.X < 0 {
					perp = perp.Scale(-1) // flee along the side it is already on
				}
				e.dodgeVec = perp.norm()
				e.dodgeT = dashDodgeTime
				e.dodgeCD = dashDodgeCooldown + dashDodgeTime
				g.spawnParticles(e.pos, 4, 0.2, 250, '.')
			}
		}
	}

	var move Vec
	switch e.def.Behavior {
	case BeChaser, BeSwarm:
		move = g.pursue(e, dir, los)
		if los {
			// Fan out while far away so a pack surrounds from both wings
			// instead of queueing behind its frontman. The curve decays with
			// distance, so everyone still converges by melee range.
			move = move.rotate(flankCurve(e.flank, dist))
		}
		if e.def.Behavior == BeSwarm {
			// Swarmlings weave, which makes packs harder to line up.
			move = move.rotate(math.Sin(e.phase*7+float64(e.id)) * 0.5)
		}
		switch {
		case e.telegraphing():
			// Planting its feet is the tell. Standing still also means the
			// player can read the direction it committed to.
			move = Vec{}
			if e.windup -= dt; e.windup <= 0 {
				e.charge = chargeTime
			}
		case e.def.Charge && los && dist > 3.5 && dist < 16 && e.cool <= 0:
			e.windup, e.lockDir = e.def.Windup, dir
			e.cool = 3.2
		case dist < e.def.Radius+p.radius+0.35 && e.cool <= 0:
			g.damagePlayer(e.damage(), dir)
			e.cool = 0.7
		}

	case BeBomber:
		move = g.pursue(e, dir, los)
		// Once lit the fuse never goes out. That turns a bomber from something
		// you outrun into something you can also shove back with knockback.
		if e.fuse > 0 || dist < 2.6 {
			e.fuse += dt
			move = move.Scale(0.4) // slow while fizzing, so a shove really moves it
			if e.fuse > bomberFuse {
				g.explode(e.pos, 4.2, e.damage(), false)
				e.hp = 0
			}
		}

	case BeShooter:
		if e.alert && los {
			switch {
			case dist > 16:
				move = dir
			case dist < 9:
				move = dir.Scale(-1)
			default:
				move = dir.rotate(math.Pi / 2).Scale(0.7) // strafe
			}
			if e.telegraphing() {
				move = Vec{} // holds still to aim
				if e.windup -= dt; e.windup <= 0 {
					// Fires along the locked line, not at the player's current
					// position: leaving the line is what dodges it.
					g.enemyShot(e, e.lockDir, 0.03)
					e.cool = e.def.FireRate
				}
			} else if e.cool <= 0 {
				e.windup, e.lockDir = e.def.Windup, dir
			}
		} else if e.alert {
			move = g.pursue(e, dir, false)
		}

	case BeTurret:
		if e.telegraphing() {
			if e.windup -= dt; e.windup <= 0 {
				for i := -1; i <= 1; i++ {
					g.enemyShot(e, e.lockDir.rotate(float64(i)*0.22), 0)
				}
				e.cool = e.def.FireRate
			}
		} else if los && dist < e.def.Sight && e.cool <= 0 {
			e.windup, e.lockDir = e.def.Windup, dir
		}

	case BeBoss:
		if !los {
			move = g.pursue(e, dir, false)
			break
		}
		if dist > 12 {
			move = dir
		} else if dist < 7 {
			move = dir.Scale(-1)
		} else {
			move = dir.rotate(math.Pi / 2)
		}
		if e.def.BossKind == bossQueen {
			switch {
			case e.telegraphing():
				// Same tell as a Brute: she plants her feet so the charge
				// line can be read and stepped out of.
				move = Vec{}
				if e.windup -= dt; e.windup <= 0 {
					e.charge = chargeTime
				}
			default:
				if e.cool <= 0 {
					g.bossAttack(e, dir)
					e.cool = e.def.FireRate
				}
				// The rush answers mid-range turtling: standing at the edge of
				// her ring is safe from nothing.
				if e.chargeCD <= 0 && dist > 4 && dist < 13 {
					e.windup, e.lockDir = e.def.Windup, dir
					e.chargeCD = queenChargeCD
				}
			}
		} else if e.cool <= 0 {
			g.bossAttack(e, dir)
			e.cool = e.def.FireRate
		}

	case BeHealer:
		move = g.medicAI(e, dt, dir, dist, los)
	}

	// A live sidestep overrides whatever the behavior wanted: the hop is the
	// whole point, and it is short enough that no patrol stalls on it.
	dodging := e.dodgeT > 0
	if dodging {
		e.dodgeT -= dt
		move = e.dodgeVec
	}

	if e.def.Speed > 0 && move.len() > 0.01 {
		sp := e.speed()
		if g.inAcid(e.pos) {
			sp *= acidSlow // slowed the same way the player is
		}
		if dodging {
			sp *= dashDodgeMul // the burst that makes the reaction real
		}
		step := move.norm().unvisual().Scale(sp * dt)
		g.moveWithCollision(&e.pos, step, e.def.Radius)
	}
}

// updateCharge runs a committed rush. Hitting the player hurts and throws them
// clear; hitting a wall leaves the charger stunned and open.
func (g *Game) updateCharge(e *Enemy, dt, dist float64, dir Vec) {
	e.charge -= dt
	if dist < e.def.Radius+g.player.radius+0.5 {
		g.damagePlayer(e.damage()*1.3, e.lockDir)
		e.charge, e.cool = 0, 1.4
		return
	}
	before := e.pos
	step := e.lockDir.unvisual().Scale(e.speed() * chargeMul * dt)
	g.moveWithCollision(&e.pos, step, e.def.Radius)
	// Barely moved means it ran into something.
	if e.pos.Sub(before).visual().len() < e.speed()*chargeMul*dt*0.35 {
		e.charge = 0
		e.stun = chargeStun
		g.spawnParticles(e.pos, 8, 0.35, 250, '*')
		g.shake = math.Max(g.shake, 0.10)
	}
	if e.charge <= 0 && e.stun <= 0 {
		e.cool = 1.4
	}
}

// pursue picks a direction: straight at the player when visible, otherwise
// downhill along the flow field, otherwise a slow idle wander.
func (g *Game) pursue(e *Enemy, dir Vec, los bool) Vec {
	if los {
		return dir
	}
	if e.alert {
		if d, ok := g.level.FlowStep(int(e.pos.X), int(e.pos.Y)); ok {
			return d
		}
	}
	if e.wander.len() < 0.01 || g.rng.Float64() < 0.01 {
		e.wander = Vec{g.rng.Float64()*2 - 1, g.rng.Float64()*2 - 1}.norm()
	}
	return e.wander.Scale(0.35)
}

func (g *Game) enemyShot(e *Enemy, dir Vec, spread float64) {
	if spread > 0 {
		dir = dir.rotate((g.rng.Float64()*2 - 1) * spread)
	}
	g.bullets = append(g.bullets, Bullet{
		pos:   e.pos,
		vel:   dir.unvisual().Scale(e.def.BulletSp),
		dmg:   e.damage(),
		life:  2.4,
		glyph: 'o',
		color: 203,
	})
}

func (g *Game) bossAttack(e *Enemy, dir Vec) {
	if e.def.BossKind == bossQueen {
		g.queenAttack(e, dir)
		return
	}
	if e.def.BossKind == bossCore {
		g.coreAttack(e, dir)
		return
	}
	switch int(e.phase/3) % 3 {
	case 0: // fan
		for i := -3; i <= 3; i++ {
			g.enemyShot(e, dir.rotate(float64(i)*0.18), 0)
		}
	case 1: // full ring
		for i := 0; i < 16; i++ {
			g.enemyShot(e, Vec{1, 0}.rotate(float64(i)*math.Pi/8+e.phase), 0)
		}
	case 2: // call in swarmlings
		for i := 0; i < 3; i++ {
			def := scaleForDepth(enemyDefs[1], g.level.Depth)
			a := g.rng.Float64() * math.Pi * 2
			g.addEnemy(def, e.pos.Add(Vec{1, 0}.rotate(a).unvisual().Scale(1.6)))
		}
	}
}

// coreAttack alternates a rotating lattice, a tight aimed gate and support
// nodes. The patterns remix skills learned in both earlier acts, but the
// offset double ring belongs only to the final duel.
func (g *Game) coreAttack(e *Enemy, dir Vec) {
	switch int(e.phase/2.6) % 3 {
	case 0: // offset double ring: the second ring closes the first set of gaps
		for i := 0; i < 12; i++ {
			a := float64(i)*math.Pi/6 + e.phase*0.35
			g.enemyShot(e, Vec{1, 0}.rotate(a), 0)
			g.enemyShot(e, Vec{1, 0}.rotate(a+math.Pi/12), 0)
		}
	case 1: // aimed gate with a readable centre lane to cross through
		for i := -4; i <= 4; i++ {
			if i == 0 {
				continue
			}
			g.enemyShot(e, dir.rotate(float64(i)*0.10), 0)
		}
	case 2: // one stationary gun and one healer force a priority decision
		for _, idx := range []int{4, 7} {
			def := scaleForDepth(enemyDefs[idx], g.level.Depth)
			a := e.phase + float64(idx)
			g.addEnemy(def, e.pos.Add(Vec{1, 0}.rotate(a).unvisual().Scale(2.2)))
		}
		g.spawnParticles(e.pos, 12, 0.4, 51, '*')
	}
}

// queenAttack is the MATRIARCH's script: bodies, then bullets. The brood wave
// comes first in the cycle so her escorts are already on the floor by the time
// the rotating fan starts sweeping them forward.
func (g *Game) queenAttack(e *Enemy, dir Vec) {
	switch int(e.phase/2.8) % 3 {
	case 0: // brood: two swarmlings and a bomber ringed around her
		for i := 0; i < 3; i++ {
			base := enemyDefs[1]
			if i == 2 {
				base = enemyDefs[3] // one bomber makes the pack something to route around
			}
			def := scaleForDepth(base, g.level.Depth)
			a := float64(i)*math.Pi*2/3 + e.phase
			g.addEnemy(def, e.pos.Add(Vec{1, 0}.rotate(a).unvisual().Scale(1.8)))
		}
		g.spawnParticles(e.pos, 10, 0.4, 207, '*')
	case 1: // rotating fan: the base angle drifts each volley, so hugging one
		// gap stops working the second time round.
		baseA := e.phase * 1.9
		for i := 0; i < 10; i++ {
			g.enemyShot(e, Vec{1, 0}.rotate(baseA+float64(i)*math.Pi*2/10), 0)
		}
	case 2: // aimed wedge: three tight pairs straight down the player's line
		for i := -1; i <= 1; i++ {
			g.enemyShot(e, dir.rotate(float64(i)*0.13), 0)
			g.enemyShot(e, dir.rotate(float64(i)*0.13+0.05), 0)
		}
	}
}

// medicAI keeps the healer alive first and useful second: flee the player,
// otherwise close on the most wounded ally and trickle health back into it.
func (g *Game) medicAI(e *Enemy, dt float64, dir Vec, dist float64, los bool) Vec {
	if los && dist < medicPanicDist {
		return dir.Scale(-1)
	}

	patient := -1
	bestMissing := 0.0
	for i := range g.enemies {
		o := &g.enemies[i]
		if o.id == e.id || o.hp <= 0 {
			continue
		}
		missing := o.maxHP - o.hp
		// Scratches do not count: chasing a 98%-HP ally would pin the medic in
		// place doing almost nothing.
		if missing < o.maxHP*0.05 {
			continue
		}
		if d := vdist(o.pos, e.pos); d <= medicSeekRange && missing > bestMissing {
			patient, bestMissing = i, missing
		}
	}
	if patient < 0 {
		return g.pursue(e, dir, false).Scale(0.6)
	}

	o := &g.enemies[patient]
	d := vdist(o.pos, e.pos)
	if d > medicHealRange {
		if g.level.LineOfSight(e.pos, o.pos) {
			return o.pos.Sub(e.pos).visual().norm()
		}
		return g.pursue(e, dir, false).Scale(0.6)
	}

	o.hp = minF(o.maxHP, o.hp+o.maxHP*medicHealFrac*dt)
	// The beam has to read as healing, not as stray sparks: a slow pulse of
	// plus-signs travelling from the patient upward does that at a glance.
	e.healTick -= dt
	if e.healTick <= 0 && g.level.Visible(int(o.pos.X), int(o.pos.Y)) {
		e.healTick = 0.15
		g.parts = append(g.parts, Particle{
			pos:  o.pos.Add(Vec{0, -0.4}),
			vel:  Vec{0, -1.4},
			life: 0.35, max: 0.35,
			glyph: '+', color: 84,
		})
	}
	return Vec{} // stand still while treating
}
