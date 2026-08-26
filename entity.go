package main

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
	dashCD    float64
	dashTimer float64
	dashDir   Vec
	// dashBuffer holds a Space press that landed mid-cooldown; it fires the
	// moment the dash is ready instead of eating the input.
	dashBuffer float64
	// lastDashStart is when the live dash began, so a hit landing just inside
	// it can be graded as a perfect dodge rather than plain invulnerability.
	lastDashStart float64
	// dodgedThisDash keeps the perfect-dodge reward to once per dash.
	dodgedThisDash bool
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
	dashMax    float64
	// dashPower stretches the blink itself; magnetMul widens loot pull;
	// meleeMul drives both the melee arc and the dash impact.
	dashPower float64
	magnetMul float64
	meleeMul  float64
	// damageTaken scales incoming damage; shrines can trade it away
	damageTaken float64

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
	return Player{
		hp: 100, maxHP: 100, radius: 0.42,
		weapon:    wpPistol,
		owned:     owned,
		ammo:      ammo,
		damageMul: 1, fireMul: 1, speedMul: 1, damageTaken: 1,
		dashMax: 1.6, dashPower: 1, magnetMul: 1, meleeMul: 1,
		level: 1, xpNext: 12,
	}
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
