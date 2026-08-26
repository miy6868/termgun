package main

import "math"

type Weapon struct {
	Name     string
	Short    string // abbreviation for a cramped status bar
	Glyph    rune
	Color    int16
	Damage   float64
	Cooldown float64 // seconds between shots
	Pellets  int
	Spread   float64 // radians, half-angle
	Speed    float64 // cells per second (visual space)
	Range    float64
	Knock    float64
	Pierce   int
	Auto     bool

	// Ammunition. The stronger the weapon, the smaller the magazine and the
	// rarer the resupply, so nothing stays equipped for a whole floor and
	// switching weapons becomes part of the fight rather than a preference.
	MaxAmmo    int // 0 means the weapon needs no ammunition
	PickupAmmo int // rounds in one ammo box
	AmmoWeight int // relative chance that a dropped box is for this weapon

	// Melee is the fallback attack: an arc swing with no ammunition at all.
	Melee bool
	Arc   float64 // radians, full width of the swing
}

var weapons = []Weapon{
	{Name: "Pistol", Short: "PIST", Glyph: '.', Color: 227, Damage: 12, Cooldown: 0.22, Pellets: 1, Spread: 0.02, Speed: 46, Range: 30, Knock: 1.5, Auto: true,
		MaxAmmo: 150, PickupAmmo: 40, AmmoWeight: 40},
	{Name: "SMG", Short: "SMG", Glyph: '.', Color: 123, Damage: 7, Cooldown: 0.075, Pellets: 1, Spread: 0.11, Speed: 52, Range: 26, Knock: 0.8, Auto: true,
		MaxAmmo: 300, PickupAmmo: 80, AmmoWeight: 30},
	{Name: "Shotgun", Short: "SHOT", Glyph: '*', Color: 215, Damage: 9, Cooldown: 0.62, Pellets: 8, Spread: 0.30, Speed: 40, Range: 16, Knock: 5, Auto: true,
		MaxAmmo: 48, PickupAmmo: 12, AmmoWeight: 20},
	{Name: "Railgun", Short: "RAIL", Glyph: '|', Color: 51, Damage: 46, Cooldown: 0.85, Pellets: 1, Spread: 0, Speed: 90, Range: 60, Knock: 3, Pierce: 4, Auto: true,
		MaxAmmo: 24, PickupAmmo: 6, AmmoWeight: 10},
	{Name: "Launcher", Short: "LNCH", Glyph: 'O', Color: 202, Damage: 30, Cooldown: 0.95, Pellets: 1, Spread: 0.03, Speed: 28, Range: 30, Knock: 2, Auto: true,
		MaxAmmo: 15, PickupAmmo: 4, AmmoWeight: 6},
	{Name: "Melee", Short: "MELE", Glyph: '/', Color: 250, Damage: 26, Cooldown: 0.42, Range: 2.4, Knock: 7, Auto: true,
		Melee: true, Arc: 2.1},
}

const (
	wpPistol = iota
	wpSMG
	wpShotgun
	wpRailgun
	wpLauncher
	wpMelee
)

// startingAmmo is what the pistol begins the run with.
const startingAmmo = 80

// muzzleSparks is how many directional sparks one trigger pull kicks out.
const muzzleSparks = 4

// needsAmmo reports whether this weapon draws from an ammo pool at all.
func (w *Weapon) needsAmmo() bool { return w.MaxAmmo > 0 }

// shoot performs one trigger pull with the equipped weapon, spending a round
// and falling back to the pistol when the magazine runs dry.
func (g *Game) shoot(origin, dir Vec) {
	p := &g.player
	w := &weapons[p.weapon]

	if w.Melee {
		g.meleeSwing(origin, dir)
		return
	}
	if p.ammo[p.weapon] <= 0 {
		g.autoSwitch()
		return
	}
	p.ammo[p.weapon]--
	g.fire(w, origin, dir)
	if p.ammo[p.weapon] == 0 {
		g.autoSwitch()
	}
}

// autoSwitch returns to the pistol, or melee when the pistol is empty.
func (g *Game) autoSwitch() {
	if !g.autoWeapon {
		return
	}
	p := &g.player
	best := wpPistol
	if !p.owned[wpPistol] || p.ammo[wpPistol] <= 0 {
		best = wpMelee
	}
	if best == p.weapon {
		return
	}
	from := weapons[p.weapon].Name
	p.weapon = best
	p.cooldown = math.Max(p.cooldown, 0.25) // a beat of delay while swapping
	if best == wpMelee {
		g.log("탄약 소진! 근접 공격으로 전환 ([6])", 203)
	} else {
		g.log(from+" 탄약 소진 - "+weapons[best].Name+"으로 전환", 214)
	}
}

// meleeSwing hits everything inside a short arc in front of the player.
func (g *Game) meleeSwing(origin, dir Vec) {
	p := &g.player
	w := &weapons[wpMelee]
	minCos := math.Cos(w.Arc / 2)

	for i := range g.enemies {
		e := &g.enemies[i]
		d := e.pos.Sub(origin).visual()
		dist := d.len()
		if dist > w.Range+e.def.Radius {
			continue
		}
		// Anything overlapping the player is hit regardless of facing.
		if dist > 0.4 {
			n := d.norm()
			if n.X*dir.X+n.Y*dir.Y < minCos {
				continue
			}
		}
		g.hurtEnemy(e, w.Damage*p.damageMul*p.meleeMul, dir.Scale(w.Knock))
	}

	// Sweep of particles so the swing reads on screen.
	for a := -w.Arc / 2; a <= w.Arc/2; a += 0.3 {
		at := origin.Add(dir.rotate(a).unvisual().Scale(w.Range * 0.75))
		g.parts = append(g.parts, Particle{
			pos: at, life: 0.12, max: 0.12, glyph: '/', color: 254,
		})
	}
	g.shake = math.Max(g.shake, 0.08)
}

// fire spawns the bullets for one trigger pull.
func (g *Game) fire(w *Weapon, origin Vec, dir Vec) {
	p := &g.player
	pellets := w.Pellets + p.extraShots
	for i := 0; i < pellets; i++ {
		spread := w.Spread
		if pellets > 1 {
			spread = math.Max(spread, 0.05)
		}
		a := (g.rng.Float64()*2 - 1) * spread
		d := dir.rotate(a)
		b := Bullet{
			pos:      origin,
			vel:      d.unvisual().Scale(w.Speed),
			dmg:      w.Damage * p.damageMul,
			life:     w.Range / w.Speed,
			glyph:    w.Glyph,
			color:    w.Color,
			pierce:   w.Pierce + p.pierce,
			knock:    w.Knock,
			explode:  w.Name == "Launcher",
			friendly: true,
		}
		g.bullets = append(g.bullets, b)
	}
	// Muzzle flash: sparks thrown forward in a narrow cone, so the gun clearly
	// fires down its own barrel rather than puffing in place.
	for i := 0; i < muzzleSparks; i++ {
		a := (g.rng.Float64()*2 - 1) * 0.35
		vd := dir.rotate(a)
		g.parts = append(g.parts, Particle{
			pos:  origin.Add(dir.unvisual().Scale(0.6)),
			vel:  vd.unvisual().Scale(9 + g.rng.Float64()*10),
			life: 0.09, max: 0.09,
			glyph: '*', color: 226,
		})
	}
	g.shake = math.Max(g.shake, 0.06+w.Cooldown*0.08)
}
