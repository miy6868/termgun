package main

import "math"

// Elites are ordinary enemies wearing one extra rule. A run is more memorable
// for "the Brute that kept healing" than for a Brute with a bigger number, and
// one prefix ability is far cheaper to build and to read than a whole new enemy.
type EliteKind int

const (
	EliteNone EliteKind = iota
	EliteRegen
	EliteSplit
	EliteBurst
	EliteAura
	EliteShield
)

type eliteDef struct {
	Name  string // shown in front of the base name
	Color int16
	Short string // one line for the help overlay
}

var eliteDefs = map[EliteKind]eliteDef{
	EliteRegen:  {"재생", 84, "피해를 입지 않으면 체력을 회복한다 - 화력을 몰아야 한다"},
	EliteSplit:  {"분열", 220, "죽으면 두 마리로 갈라진다 - 좁은 곳에서 잡지 말 것"},
	EliteBurst:  {"폭발", 203, "맞을 때마다 전방위로 탄을 뿌린다 - 쏘고 나서 움직여라"},
	EliteAura:   {"지휘", 129, "주변의 적을 강화한다 - 먼저 끊는 편이 낫다"},
	EliteShield: {"보호막", 39, "피해를 흡수하는 보호막이 있다 - 깨고 나면 잠시 그대로"},
}

var eliteKinds = []EliteKind{EliteRegen, EliteSplit, EliteBurst, EliteAura, EliteShield}

const (
	auraRadius   = 9.0
	auraSpeedMul = 1.30
	auraDmgMul   = 1.25
	regenPerSec  = 0.05 // fraction of max HP
	regenDelay   = 1.6  // seconds without damage before healing starts

	shieldFrac       = 0.35 // shield pool as a share of max HP
	shieldRegenDelay = 4.5  // seconds without damage before the shield refills
	shieldRegenRate  = 0.15 // share of the pool restored per second
)

// eliteChance grows with depth, so the first floors stay readable while later
// ones get their difficulty from behaviour rather than from bigger numbers.
func eliteChance(depth int) float64 {
	if depth < 2 {
		return 0
	}
	return math.Min(0.06+float64(depth)*0.025, 0.22)
}

// makeElite upgrades a definition in place. The glyph goes uppercase and the
// colour changes so the player can tell one apart at a glance — that is the
// whole point of the mechanic.
func makeElite(def EnemyDef, kind EliteKind) EnemyDef {
	ed, ok := eliteDefs[kind]
	if !ok {
		return def
	}
	def.Name = ed.Name + " " + def.Name
	def.Color = ed.Color
	if def.Glyph >= 'a' && def.Glyph <= 'z' {
		def.Glyph = def.Glyph - 'a' + 'A'
	}
	def.HP *= 2.2
	def.Damage *= 1.15
	def.XP = int(float64(def.XP)*2.5) + 1
	def.Score *= 3
	return def
}

// updateElite runs the per-frame half of an elite's rule.
func (g *Game) updateElite(e *Enemy, dt float64) {
	switch e.elite {
	case EliteRegen:
		// Only out of combat, so the counter-play is sustained pressure rather
		// than a damage check.
		if e.sinceHurt > regenDelay && e.hp < e.maxHP {
			e.hp = math.Min(e.maxHP, e.hp+e.maxHP*regenPerSec*dt)
		}
	case EliteAura:
		for i := range g.enemies {
			o := &g.enemies[i]
			if o.id == e.id || o.elite == EliteAura {
				continue
			}
			if vdist(o.pos, e.pos) < auraRadius {
				o.buffed = 0.25
			}
		}
	case EliteShield:
		// The pool refills out of combat like the regen elite heals, so chip
		// damage has to be followed up instead of traded one shot at a time.
		full := e.maxHP * shieldFrac
		if e.sinceHurt > shieldRegenDelay && e.shield < full {
			e.shield = math.Min(full, e.shield+e.maxHP*shieldRegenRate*dt)
		}
	}
}

// eliteOnHurt is the reactive half: the burst elite answers every hit.
func (g *Game) eliteOnHurt(e *Enemy) {
	if e.elite != EliteBurst || e.hp <= 0 {
		return
	}
	if e.burstCD > 0 {
		return
	}
	e.burstCD = 0.55
	for i := 0; i < 8; i++ {
		g.enemyShot(e, Vec{1, 0}.rotate(float64(i)*math.Pi/4), 0)
	}
}

// eliteOnDeath is the other reactive half. Splitting deliberately produces
// weaker, non-elite children: the point is to change where you stand, not to
// double the fight.
func (g *Game) eliteOnDeath(e *Enemy) {
	if e.elite != EliteSplit {
		return
	}
	base := *e.def
	base.Name = "조각"
	base.HP = math.Max(6, e.maxHP*0.22)
	base.Score /= 4
	base.XP = maxInt(1, base.XP/3)
	base.Color = 221
	if base.Glyph >= 'A' && base.Glyph <= 'Z' {
		base.Glyph = base.Glyph - 'A' + 'a'
	}
	for i := 0; i < 2; i++ {
		off := Vec{1, 0}.rotate(float64(i)*math.Pi + g.rng.Float64()).unvisual().Scale(0.9)
		g.addEnemy(base, e.pos.Add(off))
	}
}
