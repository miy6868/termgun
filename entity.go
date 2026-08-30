package main

import "math"

type Player struct {
	pos      Vec
	vel      Vec
	hp       float64
	maxHP    float64
	radius   float64
	weapon   int
	owned    []bool // weapons picked up so far; you start with the pistol only
	ammo     []int  // rounds remaining per weapon
	cooldown float64

	// dash
	dashEnergy       float64
	dashEnergyMax    float64
	dashRegenMul     float64
	dashRegenWait    float64
	dashRecovery     float64
	dashTimer        float64
	dashDir          Vec
	dashSpeed        float64
	dashStepDistance float64
	dashCoast        float64
	dashMomentum     int
	lastDashEnd      float64
	// dashBuffer holds a Space press that landed during a live dash or its
	// recovery; it fires as soon as the next short step is legal.
	dashBuffer float64
	// lastDashStart is when the live dash began, so a hit landing just inside
	// it can be graded as a perfect dodge rather than plain invulnerability.
	lastDashStart float64
	// A chain lasts briefly across repeated short dashes. Perfect-dodge energy
	// pays once per chain, not once per button press.
	dashChainTimer  float64
	dodgedThisChain bool
	// dashHitIDs lists enemies already struck by the current dash, so one
	// blink deals its impact damage to each body at most once.
	dashHitIDs []int

	invuln float64

	// perk-driven modifiers
	damageMul  float64
	fireMul    float64
	speedMul   float64
	pierce     int
	extraShots int
	lifesteal  float64
	// lifestealBudget is a token bucket measured in HP. Its capacity and
	// refill rate grow with level and maximum health, preventing burst weapons
	// from bypassing the per-second sustain budget.
	lifestealBudget float64
	// magnetMul widens loot pull; meleeMul drives both the melee arc and the
	// dash impact.
	magnetMul float64
	meleeMul  float64
	// damageTaken scales incoming damage; shrines can trade it away
	damageTaken float64
	cores       []WeaponCore // one bitset per weapon
	coreShots   []int        // trigger count for cadence-based cores

	level  int
	xp     int
	xpNext int
}

func newPlayer() Player {
	owned := make([]bool, len(weapons))
	owned[wpPistol] = true
	owned[wpMelee] = true // the fallback attack is always available
	ammo := make([]int, len(weapons))
	ammo[wpPistol] = startingAmmo
	cores := make([]WeaponCore, len(weapons))
	coreShots := make([]int, len(weapons))
	p := Player{
		hp: 100, maxHP: 100, radius: 0.42,
		weapon:    wpPistol,
		owned:     owned,
		ammo:      ammo,
		cores:     cores,
		coreShots: coreShots,
		damageMul: 1, fireMul: 1, speedMul: 1, damageTaken: 1,
		dashEnergy: dashEnergyBase, dashEnergyMax: dashEnergyBase, dashRegenMul: 1,
		magnetMul: 1, meleeMul: 1,
		level: 1, xpNext: 12,
	}
	p.lifestealBudget = p.lifestealCapPerSecond()
	return p
}

func (p *Player) lifestealCapPerSecond() float64 {
	ratio := math.Min(0.025+0.00075*float64(max(0, p.level-1)), 0.05)
	return p.maxHP * ratio
}

func (p *Player) dashRegenPerSecond() float64 {
	hpFrac := clampF(p.hp/p.maxHP, 0, 1)
	crisis := clampF((0.30-hpFrac)/0.30, 0, 1)
	return dashRegenBase * p.dashRegenMul * (1 + 1.5*crisis*crisis*crisis)
}

// totalAmmo is every round the player is carrying, across all weapons.
func (p *Player) totalAmmo() int {
	n := 0
	for i := range p.ammo {
		if weapons[i].needsAmmo() {
			n += p.ammo[i]
		}
	}
	return n
}

// baseSpeed is the player's movement speed in visual units per second.
func (p *Player) baseSpeed() float64 { return 15.0 * p.speedMul }

type Bullet struct {
	pos      Vec
	vel      Vec
	dmg      float64
	life     float64
	glyph    rune
	color    int16
	pierce   int
	knock    float64
	explode  bool
	friendly bool
	weapon   int // player weapon that created it; ignored for hostile bullets
	hitIDs   []int
}

type Particle struct {
	pos   Vec
	vel   Vec
	life  float64
	max   float64
	glyph rune
	color int16
	// hold keeps the particle's own colour until it dies instead of fading to
	// grey — used by the dash afterimage, which should read as "you" for its
	// whole life.
	hold bool
}

// FloatText and Decal live in fx.go with the rest of the juice layer.

// FloatText is a damage number that climbs away from the thing that was hit.
type FloatText struct {
	pos   Vec
	life  float64
	max   float64
	text  string
	color int16
}

// Decal is a floor mark left where something died, fading back to bare floor.
type Decal struct {
	pos  Vec
	life float64
	max  float64
}

type PickupKind int

const (
	PickHealth PickupKind = iota
	PickWeapon
	PickAmmo
	PickHeart // permanent max HP
)

type Pickup struct {
	pos    Vec
	kind   PickupKind
	weapon int
	bob    float64
}

func (p Pickup) glyph() rune {
	switch p.kind {
	case PickHealth:
		return '+'
	case PickWeapon:
		return '}'
	case PickAmmo:
		return '='
	default:
		return '&'
	}
}

func (p Pickup) color() int16 {
	switch p.kind {
	case PickHealth:
		return 46
	case PickWeapon:
		return 220
	case PickAmmo:
		return weapons[p.weapon].Color
	default:
		return 199
	}
}
