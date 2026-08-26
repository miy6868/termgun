package main

import "fmt"

// Special rooms exist so that a floor is a map rather than a set of boxes. Each
// one asks a question the player answers by walking somewhere: is the guarded
// loot worth the fight, is it worth being locked in, is the trade worth taking.

const (
	ambushWaves    = 3
	treasureGuards = 2
)

// shrineOffer is a permanent trade. Every perk in the game is an upgrade, so
// there is never a choice with a cost attached — this is that choice.
type shrineOffer struct {
	Desc  string
	Apply func(p *Player)
}

var shrineOffers = []shrineOffer{
	{"공격력 +30% / 최대 체력 -20%", func(p *Player) {
		p.damageMul *= 1.30
		p.maxHP *= 0.80
		p.hp = minF(p.hp, p.maxHP)
	}},
	{"이동 속도 +25% / 최대 체력 -15%", func(p *Player) {
		p.speedMul *= 1.25
		p.maxHP *= 0.85
		p.hp = minF(p.hp, p.maxHP)
	}},
	{"연사 속도 +35% / 받는 피해 +20%", func(p *Player) {
		p.fireMul *= 1.35
		p.damageTaken *= 1.20
	}},
	{"최대 체력 +40% / 이동 속도 -15%", func(p *Player) {
		p.maxHP *= 1.40
		p.hp = minF(p.hp+p.maxHP*0.20, p.maxHP)
		p.speedMul *= 0.85
	}},
}

// assignRooms picks which rooms get a role. The entrance and the stairs room
// stay ordinary so a run always has somewhere safe to start and a clean exit.
func (g *Game) assignRooms(startRoom, stairsRoom int) {
	l := g.level
	var free []int
	for i := range l.rooms {
		if i != startRoom && i != stairsRoom {
			free = append(free, i)
		}
	}
	g.rng.Shuffle(len(free), func(a, b int) { free[a], free[b] = free[b], free[a] })

	take := func() int {
		if len(free) == 0 {
			return -1
		}
		i := free[0]
		free = free[1:]
		return i
	}

	if i := take(); i >= 0 {
		l.roomKind[i] = RoomTreasure
		g.fillTreasureRoom(i)
	}
	if g.depth >= 2 {
		if i := take(); i >= 0 {
			l.roomKind[i] = RoomAmbush
			g.ambushRoom = i
		}
	}
	if g.depth >= 2 {
		if i := take(); i >= 0 {
			l.roomKind[i] = RoomShrine
			cx, cy := l.rooms[i].center()
			g.shrine = &Shrine{
				pos:   Vec{float64(cx) + 0.5, float64(cy) + 0.5},
				offer: shrineOffers[g.rng.Intn(len(shrineOffers))],
			}
		}
	}
}

// fillTreasureRoom puts real loot behind a real guard. The reward has to be
// worth the fight or the room is just scenery.
func (g *Game) fillTreasureRoom(idx int) {
	r := g.level.rooms[idx]
	cx, cy := r.center()
	at := func(dx, dy float64) Vec {
		return Vec{float64(cx) + 0.5 + dx, float64(cy) + 0.5 + dy}
	}
	g.addPickup(Pickup{pos: at(0, 0), kind: PickWeapon, weapon: g.rollWeapon()})
	g.addPickup(Pickup{pos: at(-1.5, 0), kind: PickHeart})
	g.addPickup(Pickup{pos: at(1.5, 0), kind: PickAmmo, weapon: g.rollAmmoFor()})
	for i := 0; i < treasureGuards; i++ {
		def, ok := g.rollEnemy(g.depth + 2)
		if !ok {
			continue
		}
		kind := eliteKinds[g.rng.Intn(len(eliteKinds))]
		g.addEliteEnemy(scaleForDepth(def, g.depth), at(float64(i)*2-1, 1.5), kind)
	}
}

// Shrine is a one-shot permanent trade standing in a room.
type Shrine struct {
	pos   Vec
	offer shrineOffer
	used  bool
}

// nearShrine reports whether the player is close enough to use it.
func (g *Game) nearShrine() bool {
	return g.shrine != nil && !g.shrine.used && vdist(g.player.pos, g.shrine.pos) < 3.0
}

func (g *Game) useShrine() {
	if !g.nearShrine() {
		return
	}
	g.shrine.used = true
	g.shrine.offer.Apply(&g.player)
	g.log("제단: "+g.shrine.offer.Desc, 213)
	g.flash = 0.2
	g.spawnParticles(g.shrine.pos, 16, 0.5, 213, '*')
}

// ---- ambush -----------------------------------------------------------------

// updateAmbush runs the sealed-room fight. Walking in shuts the doors and the
// room empties in waves; clearing it opens them again and pays out.
func (g *Game) updateAmbush(dt float64) {
	if g.ambushRoom < 0 || g.ambushRoom >= len(g.level.rooms) {
		return
	}
	room := g.level.rooms[g.ambushRoom]

	if !g.ambushOn {
		if g.ambushDone || !room.contains(g.player.pos) {
			return
		}
		g.startAmbush(room)
		return
	}

	// Sealed: count what is left inside, and send the next wave when it clears.
	alive := 0
	for i := range g.enemies {
		if room.contains(g.enemies[i].pos) {
			alive++
		}
	}
	if alive > 0 {
		return
	}
	if g.ambushWave < ambushWaves {
		g.ambushWave++
		g.spawnAmbushWave(room)
		g.log(fmt.Sprintf("%d/%d 파상공세", g.ambushWave, ambushWaves), 208)
		return
	}
	g.endAmbush()
}

func (g *Game) startAmbush(room Rect) {
	g.ambushOn, g.ambushDone = true, false
	g.ambushWave = 1
	g.ambushDoors = g.level.Openings(room)
	g.level.SetTiles(g.ambushDoors, TileDoor)
	// The doors close over tiles somebody may be standing on. Two things go
	// wrong there: a body whose centre is on the doorway is sealed inside solid
	// rock and can never move again, and a body that only just crossed the
	// threshold keeps a shoulder in the door and gets wedged. Push everyone
	// clear, towards the middle of the room.
	cx, cy := room.center()
	middle := Vec{float64(cx) + 0.5, float64(cy) + 0.5}
	for i := range g.enemies {
		e := &g.enemies[i]
		e.pos = g.level.NearestFree(e.pos)
		g.clearOfWalls(&e.pos, e.def.Radius, middle)
	}
	g.player.pos = g.level.NearestFree(g.player.pos)
	g.clearOfWalls(&g.player.pos, g.player.radius, middle)
	g.spawnAmbushWave(room)
	g.log("문이 닫혔다! 모두 처치해야 열린다.", 196)
	g.shake = maxF(g.shake, 0.2)
}

func (g *Game) endAmbush() {
	g.ambushOn, g.ambushDone = false, true
	g.level.SetTiles(g.ambushDoors, TileFloor)
	g.ambushDoors = nil

	room := g.level.rooms[g.ambushRoom]
	cx, cy := room.center()
	at := Vec{float64(cx) + 0.5, float64(cy) + 0.5}
	g.addPickup(Pickup{pos: at, kind: PickHeart})
	g.addPickup(Pickup{pos: at.Add(Vec{1.5, 0}), kind: PickAmmo, weapon: g.rollAmmoFor()})
	g.addPickup(Pickup{pos: at.Add(Vec{-1.5, 0}), kind: PickHealth})
	g.score += 120 * g.depth
	g.log("매복 격퇴! 문이 열렸다.", 226)
}

func (g *Game) spawnAmbushWave(room Rect) {
	n := 3 + g.depth/2 + g.ambushWave
	for i := 0; i < n; i++ {
		def, ok := g.rollEnemy(g.depth + 1)
		if !ok {
			continue
		}
		// Spawn along the room edge so nothing appears on top of the player.
		x := float64(room.X) + 0.5 + g.rng.Float64()*float64(room.W-1)
		y := float64(room.Y) + 0.5 + g.rng.Float64()*float64(room.H-1)
		p := Vec{x, y}
		if vdist(p, g.player.pos) < 5 {
			continue
		}
		kind := EliteNone
		if g.ambushWave == ambushWaves && i == 0 {
			kind = eliteKinds[g.rng.Intn(len(eliteKinds))] // a leader on the last wave
		}
		g.addEliteEnemy(scaleForDepth(def, g.depth), p, kind)
	}
}
