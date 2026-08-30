package main

const (
	coreEchoEvery     = 3
	coreEchoDamage    = 0.65
	coreEchoAngle     = 0.10
	coreRuptureRadius = 3.4
	coreRuptureDamage = 18.0
)

// WeaponCore is a bitset because a weapon can earn one core at each act boss.
// Cores change attack rhythm rather than merely raising a permanent stat.
type WeaponCore uint8

const (
	CoreEcho WeaponCore = 1 << iota
	CoreRupture
)

var weaponCores = []WeaponCore{CoreEcho, CoreRupture}

func (c WeaponCore) Name() string {
	switch c {
	case CoreEcho:
		return "잔향 코어"
	case CoreRupture:
		return "파열 코어"
	default:
		return "알 수 없는 코어"
	}
}

func (c WeaponCore) Desc() string {
	switch c {
	case CoreEcho:
		return "세 번째 공격마다 65% 위력의 청록색 추가 공격"
	case CoreRupture:
		return "이 무기로 처치하면 주변 적에게 파열 피해"
	default:
		return ""
	}
}

func (p *Player) hasCore(weapon int, core WeaponCore) bool {
	return weapon >= 0 && weapon < len(p.cores) && p.cores[weapon]&core != 0
}

// coreEchoes advances one weapon's trigger rhythm and reports whether this is
// the paid third beat. It is called only after an attack can actually fire.
func (g *Game) coreEchoes(weapon int) bool {
	p := &g.player
	if !p.hasCore(weapon, CoreEcho) || weapon >= len(p.coreShots) {
		return false
	}
	p.coreShots[weapon]++
	return p.coreShots[weapon]%coreEchoEvery == 0
}

func (g *Game) offerWeaponCores(preferred int) {
	if preferred < 0 || preferred >= len(weapons) || !g.player.owned[preferred] {
		preferred = g.player.weapon
	}

	// A weapon that already owns both cores yields the boss reward to the next
	// owned weapon instead of wasting it.
	target := -1
	for offset := 0; offset < len(weapons); offset++ {
		w := (preferred + offset) % len(weapons)
		if !g.player.owned[w] {
			continue
		}
		for _, core := range weaponCores {
			if !g.player.hasCore(w, core) {
				target = w
				break
			}
		}
		if target >= 0 {
			break
		}
	}
	if target < 0 {
		return
	}

	g.coreWeapon = target
	g.coreChoices = g.coreChoices[:0]
	for _, core := range weaponCores {
		if !g.player.hasCore(target, core) {
			g.coreChoices = append(g.coreChoices, core)
		}
	}
	g.state = StateWeaponCore
}

// markBlastSource attributes all bodies touched by a player rocket before the
// common explosion path applies damage to them.
func (g *Game) markBlastSource(at Vec, radius float64, weapon int) {
	if weapon < 0 || weapon >= len(weapons) {
		return
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.hp > 0 && vdist(at, e.pos) < radius {
			e.lastHitWeapon = weapon
		}
	}
}

// triggerRupture is deliberately not a normal explosion: it cannot hurt the
// player, light barrels or break walls. Its only job is to turn a kill into a
// positioning reward without replacing the launcher's terrain identity.
func (g *Game) triggerRupture(weapon int, at Vec) {
	if !g.player.hasCore(weapon, CoreRupture) {
		return
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.hp <= 0 || vdist(at, e.pos) >= coreRuptureRadius {
			continue
		}
		e.lastHitWeapon = weapon
		g.hurtEnemyFrom(e, coreRuptureDamage*g.player.damageMul,
			e.pos.Sub(at).visual().norm().Scale(2), damageSecondary)
	}
	g.spawnParticles(at, 16, 0.35, 51, '*')
	g.addFloater(at.Add(Vec{0, -0.5}), "파열", 51)
}
