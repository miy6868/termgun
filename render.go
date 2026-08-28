package main

import (
	"fmt"
	"math"
	"strings"
)

// Palette
const (
	colWallLit   int16 = 145
	colWallDim   int16 = 236
	colFloorLit  int16 = 240
	colFloorDim  int16 = 234
	colPlayer    int16 = 231
	colStairs    int16 = 226
	colHUDFrame  int16 = 240
	colHUDText   int16 = 250
	colHUDAccent int16 = 117
)

const (
	hudTop    = 2
	hudBottom = 2

	// Smallest useful HP readout: "HP " + a 6-cell gauge + " 100/100".
	minHPWidth = 3 + 6 + 10
	// "| DASH " + a 6-cell gauge, and "| Lv12 " + an 8-cell gauge.
	dashWidth  = 7 + 6
	levelWidth = 7 + 8
	// "| 층 x1.00 " (11 columns — 층 is double-width) plus a 6-cell gauge.
	pressWidth = 11 + 6
)

// camCell is the camera in whole cells.
//
// Everything drawn has to derive from this one integer, or the terrain grid and
// the things standing on it disagree. Mapping each separately from a fractional
// camera puts a tile's block at floor((tile-cam)*zoom) while putting a body
// inside that tile at floor((pos-cam)*zoom) — and those differ as soon as the
// camera is not on a whole tile, which it almost never is, because the dead-zone
// camera tracks the player's fractional position. The body then draws one cell
// past its own tile: standing next to a wall, it is drawn *in* the wall, and it
// walks around in there. Nothing is wrong with the simulation when that happens.
//
// Quantising the camera costs nothing, because a terminal can only scroll by a
// whole cell in the first place.
func (g *Game) camCell() (int, int) {
	z := float64(g.zoomEff)
	return int(math.Floor(g.camX * z)), int(math.Floor(g.camY * z))
}

// worldToScreen keeps sub-tile precision: at any zoom above 1 a bullet crossing
// a tile moves through several cells instead of jumping the whole tile at once.
// A point in tile t always lands inside t's own block, since floor(p*z) stays in
// [t*z, t*z+z) for every p in [t, t+1).
func (g *Game) worldToScreen(p Vec) (int, int) {
	z := float64(g.zoomEff)
	cx, cy := g.camCell()
	return g.viewX + g.shakeX + int(math.Floor(p.X*z)) - cx,
		g.viewY + g.shakeY + int(math.Floor(p.Y*z)) - cy
}

// screenToWorld maps a terminal cell back to a world coordinate. It is the exact
// inverse of worldToScreen, so what the mouse points at is what is drawn there.
func (g *Game) screenToWorld(mx, my int) Vec {
	z := float64(g.zoomEff)
	cx, cy := g.camCell()
	return Vec{
		X: (float64(mx-g.viewX-g.shakeX+cx) + 0.5) / z,
		Y: (float64(my-g.viewY-g.shakeY+cy) + 0.5) / z,
	}
}

func (g *Game) Draw(s *Screen) {
	s.Clear()
	g.viewX, g.viewY = 0, hudTop
	g.viewW, g.viewH = s.W, s.H-hudTop-hudBottom
	if g.viewH < 1 {
		g.viewH = 1
	}
	g.zoomEff = fitZoom(g.zoom, g.viewW, g.viewH)
	g.tilesW, g.tilesH = g.viewW/g.zoomEff, g.viewH/g.zoomEff

	g.updateCamera()
	g.drawLevel(s)
	g.drawEntities(s)
	g.drawHUD(s)
	if g.showMap {
		g.drawMinimap(s)
	}
	// Over the map so the minimap frame cannot crowd it off the top row.
	g.drawBossBar(s)

	switch g.state {
	case StateLevelUp:
		g.drawPerkMenu(s)
	case StateHelp:
		g.drawHelp(s)
	case StateSettings:
		g.drawSettings(s)
	case StatePaused:
		g.drawCenterBox(s, "일시 정지",
			[]string{"ESC / P 로 계속", "[?] 조작 안내", "[O] 설정", "[Q] 종료"}, colHUDAccent)
	case StateDead:
		g.drawGameOver(s)
	}
}

// deadzone is the fraction of the view, measured from the centre, that the
// player can move around in before the camera follows at all.
const deadzone = 0.18

// updateCamera follows the player with a dead zone and no easing.
//
// Both of those are about how scrolling *reads* in a terminal. The screen can
// only ever scroll a whole cell at a time, so what the eye picks up is not the
// size of each jump but whether the jumps arrive at a steady rhythm. Easing
// makes the camera accelerate, which spreads the cell crossings unevenly (one
// after two frames, the next after three) and reads as stutter. Tracking the
// player exactly means the camera moves at the player's constant speed, so the
// crossings are evenly spaced. The dead zone then removes most of the scrolling
// altogether: walking around a room moves nothing at all.
func (g *Game) updateCamera() {
	p := g.player.pos
	// Follow on each axis only once the player leaves the dead zone, then keep
	// exactly level with them.
	follow := func(cam, pos, view float64) float64 {
		half := view / 2
		dz := view * deadzone
		if lo := pos - (half - dz); cam > lo {
			cam = lo
		}
		if hi := pos - (half + dz); cam < hi {
			cam = hi
		}
		return cam
	}
	g.camX = follow(g.camX, p.X, float64(g.tilesW))
	g.camY = follow(g.camY, p.Y, float64(g.tilesH))

	g.camX = clampF(g.camX, 0, math.Max(0, float64(g.level.W-g.tilesW)))
	g.camY = clampF(g.camY, 0, math.Max(0, float64(g.level.H-g.tilesH)))

	// Shake is a whole-cell offset applied on top of the camera rather than a
	// nudge to it: it stays the same size on screen at any zoom, and it cannot
	// drag the camera out of its dead zone or fight the level-bounds clamp.
	g.shakeX, g.shakeY = 0, 0
	if g.screenShake && g.shake > 0 {
		g.shakeX = int(math.Round((g.rng.Float64()*2 - 1) * g.shake * 6))
		g.shakeY = int(math.Round((g.rng.Float64()*2 - 1) * g.shake * 3))
	}
}

func (g *Game) drawLevel(s *Screen) {
	ox, oy := int(math.Floor(g.camX)), int(math.Floor(g.camY))
	cx, cy := g.camCell()
	z := g.zoomEff
	screenLeft := g.viewX + g.shakeX - cx
	screenTop := g.viewY + g.shakeY - cy
	// Overdraw the edges: the camera sits on fractional coordinates so the last
	// tile is usually half off-screen, and a screen shake slides the whole
	// playfield far enough to expose tiles that were just outside it.
	mx := 1 + absInt(g.shakeX)/g.zoomEff
	my := 1 + absInt(g.shakeY)/g.zoomEff
	// Clip the tile span to the level once instead of bounds-testing every tile,
	// and index the grids directly: this loop runs over the whole screen every
	// frame, and At/Visible/Explored each repeat the same bounds test and the
	// same y*W+x on the way to adjacent bytes.
	lv := g.level
	for ty := -my; ty <= g.tilesH+my; ty++ {
		wy := oy + ty
		if wy < 0 || wy >= lv.H {
			continue
		}
		row := wy * lv.W
		y0 := screenTop + wy*z
		for tx := maxInt(-mx, -ox); tx <= g.tilesW+mx; tx++ {
			wx := ox + tx
			if wx >= lv.W {
				break
			}
			i := row + wx
			vis := lv.visible[i]
			if !vis && !lv.explored[i] {
				continue
			}
			var r rune
			var fg int16
			// Whether the tile is painted edge to edge. Walls read as mass and
			// hazards need an unmistakable boundary, so both fill; ordinary
			// floor keeps one glyph per tile, or a zoomed-in room turns into a
			// field of dots with nothing legible moving in it.
			fill := false
			// Lighting falloff: visible tiles dim with distance from the player,
			// so a lit room pools light around you instead of glowing flat.
			tier := 0
			if vis {
				tier = g.lightTierAt(wx, wy)
			} else {
				tier = 2
			}
			switch lv.tiles[i] {
			case TileWall:
				r, fg, fill = '#', colWallDim, true
				if vis {
					switch tier {
					case 0:
						fg = colWallLit
					case 1:
						fg = 244
					}
				}
			case TileStairs:
				r, fg = '>', colStairs
				if !vis {
					fg = 58
				}
			case TileDoor:
				// Not '+': that is the medkit glyph, and a ring of doors around
				// a room full of pickups has to read as a barrier at a glance.
				r, fg, fill = 'X', 130, true
				if vis {
					fg = 214
				}
			case TileCracked:
				// Distinct from '#': a wall you can open has to look different
				// from one you cannot, or the launcher's second use is invisible.
				r, fg, fill = '%', 94, true
				if vis {
					fg = 180
				}
			case TileAcid:
				r, fg, fill = '~', 22, true
				if vis {
					fg = 47
				}
			case TileRubble:
				r = ','
				if vis {
					switch tier {
					case 0:
						fg = colFloorLit
					case 1:
						fg = 238
					default:
						fg = colFloorDim
					}
				} else {
					fg = colFloorDim
				}
			default:
				r = '.'
				if vis {
					switch tier {
					case 0:
						fg = colFloorLit
					case 1:
						fg = 238
					default:
						fg = colFloorDim
					}
				} else {
					fg = colFloorDim
				}
			}
			// Terrain always starts on whole tiles. Reuse the frame's integer
			// transform instead of rebuilding it for every visible tile; moving
			// entities still use worldToScreen to preserve sub-tile precision.
			x0 := screenLeft + wx*z
			if !fill {
				// Centre the glyph in its block.
				g.put(s, x0+z/2, y0+z/2, r, fg)
				continue
			}
			// One clipped rectangle rather than zoom² separate writes, each of
			// which would re-run the same bounds tests.
			g.putRect(s, x0, y0, z, z, r, fg)
		}
	}
}

func (g *Game) drawEntities(s *Screen) {
	// Corpse marks first: they belong to the floor, everything else walks on
	// top of them. putIfEmpty keeps one from stamping over stairs or loot.
	for i := range g.decals {
		d := &g.decals[i]
		if !g.level.Visible(int(d.pos.X), int(d.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(d.pos)
		c := int16(52)
		if d.life/d.max < 0.5 {
			c = 234 // half-faded: sink back towards bare floor
		}
		g.putIfEmpty(s, x+g.zoomEff/2-1, y+g.zoomEff/2, ',', c)
	}

	// particles first so entities draw over them
	for _, p := range g.parts {
		if !g.level.Visible(int(p.pos.X), int(p.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(p.pos)
		c := p.color
		if p.life/p.max < 0.4 && !p.hold {
			c = 240
		}
		g.put(s, x, y, p.glyph, c)
	}

	for _, pk := range g.pickups {
		if !g.level.Visible(int(pk.pos.X), int(pk.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(pk.pos)
		c := pk.color()
		if math.Mod(pk.bob, 1.2) < 0.6 {
			c = 231
		}
		g.put(s, x, y, pk.glyph(), c)
	}

	// Barrels sit under everything else: they are terrain, not actors.
	for i := range g.barrels {
		b := &g.barrels[i]
		if b.spent || !g.level.Visible(int(b.pos.X), int(b.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(b.pos)
		c := int16(166)
		if b.lit {
			// Blinks fast once lit, so a chain about to reach you is obvious.
			if int(g.elapsed*20)%2 == 0 {
				c = 231
			} else {
				c = 196
			}
		}
		g.put(s, x, y, '0', c)
	}

	// The shrine gets its own glyph and pulses, because a permanent trade the
	// player walks past without noticing may as well not be in the game.
	if sh := g.shrine; sh != nil && g.level.Visible(int(sh.pos.X), int(sh.pos.Y)) {
		x, y := g.worldToScreen(sh.pos)
		c, gl := int16(213), '$'
		if sh.used {
			c, gl = 240, '_'
		} else if int(g.elapsed*3)%2 == 0 {
			c = 231
		}
		g.put(s, x, y, gl, c)
	}

	if !g.level.Visible(int(g.stairsPos.X), int(g.stairsPos.Y)) && g.level.Explored(int(g.stairsPos.X), int(g.stairsPos.Y)) {
		x, y := g.worldToScreen(g.stairsPos)
		g.put(s, x, y, '>', 58)
	}

	// Telegraph lanes go under the enemies so a body never hides its own tell.
	// A commander's aura ring rides along: without it the buff radius is a
	// number in the code rather than something a player can act on.
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.telegraphing() {
			g.drawTelegraph(s, e)
		}
		if e.elite == EliteAura && e.hp > 0 && g.level.Visible(int(e.pos.X), int(e.pos.Y)) {
			g.drawAuraRing(s, e)
		}
	}

	for i := range g.enemies {
		e := &g.enemies[i]
		if !g.level.Visible(int(e.pos.X), int(e.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(e.pos)
		c, gl := e.def.Color, e.def.Glyph
		switch {
		case e.hurt > 0:
			c = 231
		case e.stun > 0:
			// Wide open: say so, or the reward for dodging goes unnoticed.
			c, gl = 245, 'x'
		case e.telegraphing():
			// Flashing at a fixed rate reads as "about to happen" even when the
			// lane itself is off screen.
			if int(e.phase*12)%2 == 0 {
				c = 231
			} else {
				c = 196
			}
		case e.fuse > 0:
			// The fuse blinks faster the closer it is to going off.
			rate := 6 + 18*(e.fuse/bomberFuse)
			if int(e.phase*rate)%2 == 0 {
				c, gl = 231, '*'
			} else {
				c = 196
			}
		case e.charge > 0:
			c = 231
		case e.buffed > 0:
			// Empowered by a nearby commander. The extra speed and damage have
			// to show, or fighting into an aura feels like unfair dice.
			c = 177
		case e.shield > 0 && int(e.phase*6)%3 == 0:
			// The shield pool flickers cyan so "my shots are being eaten"
			// has a visible cause on the glyph itself.
			c = 39
		}
		g.put(s, x, y, gl, c)
		// A damage pip above wounded elites.
		if e.maxHP > 60 && e.hp < e.maxHP {
			g.putBar(s, x+g.zoomEff/2-2, y-1, 5, e.hp/e.maxHP)
		}
	}

	for _, b := range g.bullets {
		if !g.level.Visible(int(b.pos.X), int(b.pos.Y)) {
			continue
		}
		x, y := g.worldToScreen(b.pos)
		gl := b.glyph
		if gl == '|' {
			gl = bulletTrailGlyph(b.vel)
		}
		g.put(s, x, y, gl, b.color)
	}

	// player
	px, py := g.worldToScreen(g.player.pos)
	pc := colPlayer
	if g.player.invuln > 0 && int(g.elapsed*20)%2 == 0 {
		pc = 196
	}
	g.put(s, px, py, '@', pc)

	// Damage direction marker: a pair of stars on the side the last hit came
	// from, so "which way do I dodge" survives the flash.
	if g.hitFrom.len() > 0.01 && g.hitFromAge < hitDirTime {
		c := int16(196)
		if g.hitFromAge > hitDirTime*0.5 {
			c = 52
		}
		for _, d := range [2]float64{1.3, 1.9} {
			p := g.player.pos.Add(g.hitFrom.unvisual().Scale(d))
			x, y := g.worldToScreen(p)
			g.put(s, x, y, '*', c)
		}
	}

	// aim reticle + a dotted line towards it
	if g.mouseSet {
		dir := aimDir(g.player.pos, g.aim)
		dist := vdist(g.player.pos, g.aim)
		// Spacing is in world units, so divide by the zoom to keep the dots the
		// same distance apart on screen.
		z := float64(g.zoomEff)
		for d := 1.5 / z; d < dist-0.5; d += 1.6 / z {
			p := g.player.pos.Add(dir.unvisual().Scale(d))
			if g.level.SolidAtPoint(p) {
				break
			}
			x, y := g.worldToScreen(p)
			if x == px && y == py {
				continue
			}
			g.putIfEmpty(s, x, y, ':', 238)
		}
		ax, ay := g.worldToScreen(g.aim)
		// The reticle turns into a red X over a target, so "in the crosshairs"
		// is answered by the screen and not by hope.
		gl, ac := '+', int16(46)
		for i := range g.enemies {
			if vdist(g.enemies[i].pos, g.aim) < g.enemies[i].def.Radius+0.45 {
				gl, ac = 'X', 196
				break
			}
		}
		g.put(s, ax, ay, gl, ac)
	}

	// Damage numbers last: they are feedback about a hit, not part of the
	// scene, so nothing may bury them.
	for i := range g.floaters {
		f := &g.floaters[i]
		x, y := g.worldToScreen(f.pos)
		c := f.color
		if f.life/f.max < 0.4 {
			c = 240
		}
		x -= len(f.text) / 2 // digits are ASCII, one column each
		for j := 0; j < len(f.text); j++ {
			g.put(s, x+j, y, rune(f.text[j]), c)
		}
	}
}

// drawTelegraph paints the lane an enemy has committed to during its windup.
// This is the whole point of the windup: the player has to be able to see the
// line in order to step out of it.
func (g *Game) drawTelegraph(s *Screen, e *Enemy) {
	if e.lockDir.len() < 0.01 {
		return
	}
	reach := e.def.Sight
	if e.def.Charge {
		reach = e.speed() * chargeMul * chargeTime
	}
	// Fills in as the windup runs down, so the lane doubles as the timer.
	frac := 1.0
	if e.def.Windup > 0 {
		frac = 1 - e.windup/e.def.Windup
	}
	col := int16(196)
	if frac > 0.7 {
		col = 231 // about to fire
	}
	z := float64(g.zoomEff)
	for d := 1.0; d < reach; d += 1.0 / z {
		p := e.pos.Add(e.lockDir.unvisual().Scale(d))
		if g.level.SolidAtPoint(p) {
			break
		}
		if d > reach*frac {
			break
		}
		// Drawn per-tile rather than gated on the shooter being visible: a
		// sniper outranges the player's sight, so the lane has to be able to
		// reach into view out of the dark. Otherwise the one attack that most
		// needs warning is the one that never gets any.
		if !g.level.Visible(int(p.X), int(p.Y)) {
			continue
		}
		x, y := g.worldToScreen(p)
		g.putIfEmpty(s, x, y, ':', col)
	}
}

// drawAuraRing sketches the reach of a commander's elite aura with a sparse
// dotted circle, pulsing slowly. It is deliberately under the enemies: the ring
// is information, not the threat itself.
func (g *Game) drawAuraRing(s *Screen, e *Enemy) {
	const points = 18
	col := int16(129)
	if int(e.phase*2)%2 == 0 {
		col = 240
	}
	for i := 0; i < points; i++ {
		a := float64(i) / points * math.Pi * 2
		// The radius lives in visual space like every other range in the game.
		p := e.pos.Add(Vec{1, 0}.rotate(a).Scale(auraRadius).unvisual())
		if g.level.SolidAtPoint(p) {
			continue
		}
		x, y := g.worldToScreen(p)
		g.putIfEmpty(s, x, y, ':', col)
	}
}

func bulletTrailGlyph(v Vec) rune {
	vv := v.visual()
	a := math.Abs(vv.Y / (math.Abs(vv.X) + 1e-6))
	switch {
	case a < 0.5:
		return '-'
	case a > 2:
		return '|'
	case vv.X*vv.Y > 0:
		return '\\'
	default:
		return '/'
	}
}

func (g *Game) put(s *Screen, x, y int, r rune, fg int16) {
	if y < g.viewY || y >= g.viewY+g.viewH || x < g.viewX || x >= g.viewX+g.viewW {
		return
	}
	s.Set(x, y, r, fg, colorDefault)
}

// putRect fills a block of cells clipped to the playfield, for terrain that is
// painted edge to edge.
func (g *Game) putRect(s *Screen, x, y, w, h int, r rune, fg int16) {
	x0, y0 := maxInt(x, g.viewX), maxInt(y, g.viewY)
	x1, y1 := minInt(x+w, g.viewX+g.viewW), minInt(y+h, g.viewY+g.viewH)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	s.Fill(x0, y0, x1-x0, y1-y0, r, fg, colorDefault)
}

func (g *Game) putIfEmpty(s *Screen, x, y int, r rune, fg int16) {
	if y < g.viewY || y >= g.viewY+g.viewH || x < g.viewX || x >= g.viewX+g.viewW {
		return
	}
	if c := s.cur[y*s.W+x]; c.R == '.' || c.R == ' ' || c.R == ',' {
		s.Set(x, y, r, fg, colorDefault)
	}
}

func (g *Game) putBar(s *Screen, x, y, w int, frac float64) {
	filled := int(frac*float64(w) + 0.5)
	for i := 0; i < w; i++ {
		r, c := '-', int16(238)
		if i < filled {
			r, c = '=', 203
		}
		g.put(s, x+i, y, r, c)
	}
}

// drawGauge paints a progress bar out of blank cells with coloured
// backgrounds — no block-drawing glyphs, whose width terminals disagree on.
func drawGauge(s *Screen, x, y, w int, frac float64, fill, empty int16) {
	n := int(clampF(frac, 0, 1)*float64(w) + 0.5)
	for i := 0; i < w; i++ {
		bg := empty
		if i < n {
			bg = fill
		}
		s.Set(x+i, y, ' ', colHUDText, bg)
	}
}

// appendSlotLabel writes the "<key><weapon> <ammo>" chip for one weapon slot
// into b. Melee has no ammunition, so it shows none.
//
// This is built by hand rather than with Sprintf because it is the whole HUD's
// allocation budget: six chips are formatted every frame, and each one measured
// again for padding, which was most of the garbage a drawn frame produced.
func appendSlotLabel(b []byte, p *Player, i int, abbrev bool) []byte {
	b = append(b, byte('1'+i))
	if abbrev {
		b = append(b, weapons[i].Short...)
	} else {
		b = append(b, ' ')
		b = append(b, weapons[i].Name...)
	}
	if weapons[i].needsAmmo() {
		b = append(b, ' ')
		b = appendUint(b, p.ammo[i])
	}
	return b
}

// slotLabelLen is the reserved width of each chip, indexed by abbrev. The
// weapon table and fullMag are both fixed, so these never change once measured.
var slotLabelLen [2][]int

func reservedSlotLen(i int, abbrev bool) int {
	a := 0
	if abbrev {
		a = 1
	}
	if slotLabelLen[a] == nil {
		lens := make([]int, len(weapons))
		var buf []byte
		for k := range weapons {
			lens[k] = len(appendSlotLabel(buf[:0], fullMag, k, abbrev))
		}
		slotLabelLen[a] = lens
	}
	return slotLabelLen[a][i]
}

// slotBarWidth measures the bar at its widest, using each weapon's full
// magazine, so the layout does not reflow every time a round is spent.
func slotBarWidth(abbrev, label bool) int {
	n := len(weapons) - 1 // gaps between chips
	if label {
		n += 5 // the "무기" prefix
	}
	for i := range weapons {
		n += reservedSlotLen(i, abbrev)
	}
	return n
}

// fullMag is a stand-in player with every magazine full, used only to reserve
// the widest each slot chip can become.
var fullMag = &Player{}

// slotBuf is scratch for the per-frame chip text. Drawing is single-threaded,
// so one shared buffer is enough to keep the HUD allocation-free.
var slotBuf []byte

// drawSlots renders the weapon list. The equipped weapon is shown inverted,
// weapons you own in their own colour, and ones you have not found yet greyed
// out — so which number selects what is readable before you own them.
func drawSlots(s *Screen, x, y int, p *Player, abbrev, label bool) int {
	if label {
		s.Str(x, y, "무기", colHUDText, 234)
		x += 5
	}
	for i := range weapons {
		slotBuf = appendSlotLabel(slotBuf[:0], p, i, abbrev)
		empty := weapons[i].needsAmmo() && p.ammo[i] == 0
		fg, bg := weapons[i].Color, int16(234)
		switch {
		case i == p.weapon:
			fg, bg = 232, weapons[i].Color // equipped: inverted
		case !p.owned[i]:
			fg = 240 // not picked up yet
		case empty:
			fg = 238 // owned but dry
		}
		s.StrBytes(x, y, slotBuf, fg, bg)
		// Pad to the widest this chip can get, so spending rounds does not
		// shuffle the whole bar sideways.
		x += maxInt(len(slotBuf), reservedSlotLen(i, abbrev)) + 1
	}
	return x
}

// ---- HUD --------------------------------------------------------------------

func (g *Game) drawHUD(s *Screen) {
	p := &g.player

	// The status area gets two dedicated rows. Cramming it onto one line meant
	// dropping whatever did not fit, and a mechanic the player cannot see is a
	// mechanic they will never use — so vitals go on the first row and the
	// weapon slots get the second to themselves.
	s.Fill(0, 0, s.W, 1, ' ', colHUDText, 235)
	s.Fill(0, 1, s.W, 1, ' ', colHUDText, 234)

	// --- row 0: vitals, dash, level, progress -------------------------------
	right := fmt.Sprintf("B%d층 | 처치 %d | 점수 %d | %s", g.depth, g.kills, g.score, fmtTime(g.elapsed))
	// Drop the clock before dropping any ability readout: the elapsed time is
	// flavour, whereas the dash and level gauges teach mechanics.
	if strWidth(right)+minHPWidth+dashWidth+levelWidth+2 > s.W {
		right = fmt.Sprintf("B%d층 | %d | %d", g.depth, g.kills, g.score)
	}
	rx := maxInt(s.W-strWidth(right)-1, 1)
	s.Str(rx, 0, right, colHUDAccent, 235)

	x := 1
	fits := func(n int) bool { return x+n < rx }

	hpFrac := p.hp / p.maxHP
	hpCol := int16(41)
	switch {
	case hpFrac <= 0.3:
		hpCol = 196
	case hpFrac <= 0.6:
		hpCol = 214
	}
	barW := clamp(rx/5, 8, 18)
	s.Str(x, 0, "HP", colHUDText, 235)
	x += 3
	// Gauges are painted with background colour only, so they stay exactly one
	// column per cell in every terminal.
	drawGauge(s, x, 0, barW, hpFrac, hpCol, 238)
	x += barW + 1
	// The numeric HP readout is the first thing sacrificed on a narrow line —
	// the gauge already shows health, while nothing else hints that a dash, a
	// levelling system or a decaying floor bonus exists.
	if x+9+dashWidth+levelWidth+pressWidth < rx {
		s.Str(x, 0, fmt.Sprintf("%3.0f/%3.0f", p.hp, p.maxHP), colHUDText, 235)
		x += 9
	}

	// Dash shows as a refilling gauge, which reads at a glance and also tells a
	// new player the ability exists in the first place.
	dashFrac, dashCol := 1.0, int16(51)
	if p.dashCD > 0 {
		dashFrac, dashCol = 1-p.dashCD/p.dashMax, 240
	}
	if fits(dashWidth) {
		s.Str(x, 0, "| DASH ", dashCol, 235)
		x += 7
		drawGauge(s, x, 0, 6, dashFrac, dashCol, 238)
		x += 7
	}

	if lv := fmt.Sprintf("| Lv%d ", p.level); fits(len(lv) + 8) {
		s.Str(x, 0, lv, 213, 235)
		x += len(lv)
		drawGauge(s, x, 0, 8, float64(p.xp)/float64(p.xpNext), 213, 238)
		x += 9
	}

	// Descent pressure. The bonus multiplier is the number the player is
	// actually trading away by staying, and the gauge fills towards the point
	// where reinforcements start — both have to be visible for "go now or clear
	// the floor" to be a decision rather than a surprise.
	prLabel := fmt.Sprintf("| 층 x%.2f ", g.descentMul())
	if pf := g.pressureFrac(); fits(strWidth(prLabel) + 6) {
		col := int16(84)
		switch {
		case pf >= 1:
			col = 196
		case pf > 0.7:
			col = 214
		}
		s.Str(x, 0, prLabel, col, 235)
		x += strWidth(prLabel)
		drawGauge(s, x, 0, 6, pf, col, 238)
		x += 7
	}

	// --- row 1: weapon slots and the help hint ------------------------------
	// Everything on this row earns its place, so instead of dropping items the
	// layout gets progressively more terse until it fits.
	// The zoom rides along on the hint: it is the one setting whose current
	// value the player cannot read off the playfield itself.
	zoom := fmt.Sprintf("[O] 배율x%d", g.zoomEff)
	layouts := []struct {
		abbrev, label bool
		hint          string
	}{
		{false, true, zoom + "  [?] 도움말"},
		{true, true, zoom + "  [?] 도움말"},
		{true, true, zoom + "  [?]"},
		{true, true, "[?]"},
		{true, false, "[?]"},
	}
	chosen := layouts[len(layouts)-1]
	for _, l := range layouts {
		if slotBarWidth(l.abbrev, l.label)+strWidth(l.hint)+3 <= s.W {
			chosen = l
			break
		}
	}
	drawSlots(s, 1, 1, p, chosen.abbrev, chosen.label)
	if hintX := s.W - strWidth(chosen.hint) - 1; hintX > 1 {
		s.Str(hintX, 1, chosen.hint, colHUDAccent, 234)
	}

	g.drawPrompt(s)

	// Bottom: two most recent messages
	base := s.H - hudBottom
	s.Fill(0, base, s.W, hudBottom, ' ', colHUDText, colorDefault)
	shown := 0
	for i := len(g.msgs) - 1; i >= 0 && shown < hudBottom; i-- {
		m := g.msgs[i]
		c := m.color
		if m.age > 4 {
			c = 240
		}
		s.Str(1, base+hudBottom-1-shown, truncate(m.text, s.W-2), c, colorDefault)
		shown++
	}

	if g.flash > 0 {
		// Tint the frame red when hit.
		for x := 0; x < s.W; x++ {
			s.SetBG(x, hudTop, 52)
			s.SetBG(x, s.H-hudBottom-1, 52)
		}
	}

	// Low-health vignette: below the threshold the whole border pulses red, so
	// "you are one mistake from dying" is ambient rather than a number to read.
	if frac := p.hp / p.maxHP; g.state == StatePlaying && frac > 0 && frac <= lowHPVignette {
		col := int16(88)
		if int(g.elapsed*5)%2 == 0 {
			col = 52
		}
		for x := 0; x < s.W; x++ {
			s.SetBG(x, hudTop, col)
			s.SetBG(x, s.H-hudBottom-1, col)
		}
		for y := hudTop; y < s.H-hudBottom; y++ {
			s.SetBG(0, y, col)
			s.SetBG(s.W-1, y, col)
		}
	}

	if vdist(g.player.pos, g.stairsPos) < 2.0 {
		if g.stairsBlocked() {
			hint := " 보스를 처치해야 계단이 열린다 "
			s.Str((s.W-strWidth(hint))/2, s.H-hudBottom-1, hint, 231, 196)
		} else {
			hint := " [E] 다음 층으로 "
			s.Str((s.W-strWidth(hint))/2, s.H-hudBottom-1, hint, 232, 226)
		}
	}
}

func fmtTime(t float64) string {
	return fmt.Sprintf("%d:%02d", int(t)/60, int(t)%60)
}

// bossEnemy returns the floor's living boss, if any.
func (g *Game) bossEnemy() *Enemy {
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.def.Behavior == BeBoss && e.hp > 0 {
			return e
		}
	}
	return nil
}

// drawBossBar pins the boss's name and health to the top of the playfield once
// it has woken up. A duel against a wall of HP needs a visible answer to "is
// this working", and hunting the boss glyph through a crowd is not one.
func (g *Game) drawBossBar(s *Screen) {
	b := g.bossEnemy()
	if b == nil || !b.alert {
		return
	}
	name := b.def.Name
	w := minInt(bossBarMaxW, g.viewW-12)
	if w < 6 {
		return
	}
	x := maxInt((s.W-(strWidth(name)+1+w))/2, 0)
	y := hudTop
	s.Str(x, y, name, 231, 234)
	drawGauge(s, x+strWidth(name)+1, y, w, clampF(b.hp/b.maxHP, 0, 1), 196, 52)
}

func (g *Game) drawMinimap(s *Screen) {
	// Scale the level down 2x horizontally so it keeps its shape on screen.
	mw := minInt(g.level.W/2, s.W/3)
	mh := minInt(g.level.H/2, g.viewH-2)
	sx := clamp(int(g.player.pos.X)/2-mw/2, 0, g.level.W/2-mw)
	sy := clamp(int(g.player.pos.Y)/2-mh/2, 0, g.level.H/2-mh)
	ox := s.W - mw - 2
	oy := hudTop + 1
	s.Fill(ox-1, oy-1, mw+2, mh+2, ' ', colHUDText, 233)
	for y := 0; y < mh; y++ {
		for x := 0; x < mw; x++ {
			wx, wy := (sx+x)*2, (sy+y)*2
			if !g.level.Explored(wx, wy) {
				continue
			}
			r, c := ' ', int16(238)
			if !g.level.Solid(wx, wy) {
				r, c = '.', 244
			}
			if g.level.At(wx, wy) == TileStairs {
				r, c = '>', 226
			}
			s.Set(ox+x, oy+y, r, c, 233)
		}
	}
	// Landmarks under actors: a shrine you found but never used is worth
	// remembering the way the stairs are, and visible loot is where you are
	// heading next.
	if sh := g.shrine; sh != nil && g.level.Explored(int(sh.pos.X), int(sh.pos.Y)) {
		ex, ey := int(sh.pos.X)/2-sx, int(sh.pos.Y)/2-sy
		if ex >= 0 && ex < mw && ey >= 0 && ey < mh {
			s.Set(ox+ex, oy+ey, '$', 213, 233)
		}
	}
	for _, pk := range g.pickups {
		if !g.level.Visible(int(pk.pos.X), int(pk.pos.Y)) {
			continue
		}
		ex, ey := int(pk.pos.X)/2-sx, int(pk.pos.Y)/2-sy
		if ex >= 0 && ex < mw && ey >= 0 && ey < mh {
			s.Set(ox+ex, oy+ey, pk.glyph(), pk.color(), 233)
		}
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		ex, ey := int(e.pos.X)/2-sx, int(e.pos.Y)/2-sy
		if ex >= 0 && ex < mw && ey >= 0 && ey < mh &&
			g.level.Visible(int(e.pos.X), int(e.pos.Y)) {
			s.Set(ox+ex, oy+ey, 'x', 196, 233)
		}
	}
	s.Set(ox+int(g.player.pos.X)/2-sx, oy+int(g.player.pos.Y)/2-sy, '@', 231, 233)
}

// ---- overlays ---------------------------------------------------------------

const helpPageCount = 3

func (g *Game) drawCenterBox(s *Screen, title string, lines []string, color int16) {
	// Widths are measured in columns, not runes: Hangul takes two columns each,
	// and a frame sized in runes would leave the right border adrift.
	w := strWidth(title) + 4
	for _, l := range lines {
		if n := strWidth(l) + 4; n > w {
			w = n
		}
	}
	w = minInt(w, s.W)
	h := minInt(len(lines)+4, s.H)
	x := maxInt((s.W-w)/2, 0)
	y := maxInt((s.H-h)/2, 0)
	s.Fill(x, y, w, h, ' ', colHUDText, 234)
	for i := 0; i < w; i++ {
		s.Set(x+i, y, '-', color, 234)
		s.Set(x+i, y+h-1, '-', color, 234)
	}
	for j := 0; j < h; j++ {
		s.Set(x, y+j, '|', color, 234)
		s.Set(x+w-1, y+j, '|', color, 234)
	}
	s.Str(x+(w-strWidth(title))/2, y+1, truncate(title, w-2), color, 234)
	for i, l := range lines {
		if y+3+i >= y+h-1 {
			break
		}
		s.Str(x+2, y+3+i, truncate(l, w-4), colHUDText, 234)
	}
}

// padRight pads s out to the given number of columns, counting wide runes as
// two, so mixed Hangul/ASCII columns still line up.
func padRight(s string, cols int) string {
	if n := strWidth(s); n < cols {
		return s + strings.Repeat(" ", cols-n)
	}
	return s
}

func (g *Game) drawPerkMenu(s *Screen) {
	lines := []string{}
	for i, p := range g.perkChoices {
		lines = append(lines, fmt.Sprintf("[%d] %s %s", i+1, padRight(p.Name, 14), p.Desc))
	}
	lines = append(lines, "", "    숫자 키로 하나를 선택하세요")
	g.drawCenterBox(s, fmt.Sprintf("레벨 %d - 특성 선택", g.player.level), lines, 213)
}

// drawPrompt puts a one-line banner at the foot of the playfield for whatever
// the player is currently standing next to. A shrine's trade in particular has
// to be readable *before* committing to it — otherwise the choice is a coin
// flip, not a decision.
func (g *Game) drawPrompt(s *Screen) {
	var text string
	var col int16
	switch {
	case g.nearShrine():
		text, col = "[E] 제단: "+g.shrine.offer.Desc, 213
	case g.ambushOn:
		text, col = fmt.Sprintf("봉쇄됨 - %d/%d 파상공세", g.ambushWave, ambushWaves), 203
	case vdist(g.player.pos, g.stairsPos) < 2.5:
		if g.stairsBlocked() {
			text, col = fmt.Sprintf("%s를 처치해야 계단이 열린다", g.bossEnemy().def.Name), 203
		} else {
			text, col = fmt.Sprintf("[E] 다음 층으로 (보너스 배율 x%.2f)", g.descentMul()), colStairs
		}
	default:
		return
	}
	y := g.viewY + g.viewH - 1
	x := maxInt((s.W-strWidth(text))/2, 0)
	s.Fill(maxInt(x-1, 0), y, minInt(strWidth(text)+2, s.W), 1, ' ', colHUDText, 235)
	s.Str(x, y, truncate(text, s.W), col, 235)
}

// drawHelp lists everything the game can do. A terminal roguelike has no
// tooltips and no menus to stumble into, so without this a new player has no
// way to discover dashing, the minimap, or the stairs.
func (g *Game) drawHelp(s *Screen) {
	mode := g.inputMode.String()
	modeNote := "두 방향을 함께 누르면 대각선으로 이동합니다."
	if g.inputMode == InputCompat {
		modeNote = "이 터미널은 키를 뗀 것을 알려주지 않아 대각선이 불안정합니다."
	}
	controls := []string{
		"[이동]   W A S D 또는 방향키 (두 개를 함께 누르면 대각선)",
		"[조준]   마우스",
		"[사격]   마우스 좌클릭 (누르고 있으면 연사)",
		"[대시]   Space 또는 우클릭 - 짧은 무적. 적을 몸으로 부수면 피해를 주고,",
		"         공격을 스치면 [완벽 회피]. 눈치 빠른 적(w b m)은 흩어져 비켜선다",
		"[무기]   1~5 (획득한 것만) - 상단 목록에서 회색은 미획득, 어두운 숫자는 탄약 0",
		"[근접]   6 - 탄약이 필요 없고 언제나 사용 가능. 짧은 사거리 부채꼴 공격",
		"[계단]   > 위에서 E - 다음 층으로. 제단($) 앞에서도 E",
		"[지도]   Tab 또는 M      " + padRight("[일시정지]", 12) + "P / ESC",
		fmt.Sprintf("[배율]   설정에서 <-/->  (현재 x%d, %d~%d) - 실행 시 -zoom 으로도 지정",
			g.zoomEff, minZoom, maxZoom),
		"[설정]   O - 편의 기능과 화면 배율",
		"키보드: " + mode,
		"        " + modeNote,
	}
	if g.inputMode == InputCompat {
		controls = append(controls, "        -input device 로 실행하면 정밀 모드가 됩니다.")
	}
	controls = append(controls, "", "[A/D 또는 <-/->] 페이지    그 외 키: 돌아가기")

	combat := []string{
		"바닥 아이템:  " + padRight("+ 의료 키트", 14) + padRight("} 무기", 10) +
			padRight("= 탄약", 10) + "& 최대 체력",
		"강한 무기일수록 탄약이 적게 나옵니다. 탄이 떨어지면 1번 권총으로",
		"자동 전환되고, 권총도 비면 근접 공격으로 넘어갑니다.",
		"적:  g 돌격  w 군집  s 사격  b 자폭  m 치유사",
		"     T 포탑  B 중장갑  S 저격  O 보스",
		"",
		"큰 공격 전에는 반드시 예고가 있습니다. 붉은 점선(:)이 그려지면 그 선에서",
		"벗어나세요. 돌진을 피하면 벽에 부딪혀 잠시 무방비(x)가 됩니다. 자폭병(b)의",
		"심지는 한 번 붙으면 꺼지지 않으니, 샷건 넉백으로 밀어내는 것도 방법입니다.",
		"",
		"정예(대문자·다른 색): " + padRight("재생", 8) + eliteDefs[EliteRegen].Short,
		"                      " + padRight("분열", 8) + eliteDefs[EliteSplit].Short,
		"                      " + padRight("폭발", 8) + eliteDefs[EliteBurst].Short,
		"                      " + padRight("지휘", 8) + eliteDefs[EliteAura].Short,
		"                      " + padRight("보호막", 8) + eliteDefs[EliteShield].Short,
		"[A/D 또는 <-/->] 페이지    그 외 키: 돌아가기",
	}

	dungeon := []string{
		"방:  보물방은 정예가 지키고, 매복방은 들어가면 문(X)이 닫힙니다.",
		"     제단($)은 이득과 대가를 함께 주는 영구 교환입니다.",
		"",
		"지형:  " + padRight("0 폭발통", 14) + "쏘면 연쇄 폭발. 적을 유인해서 터뜨리세요",
		"       " + padRight("% 균열 벽", 14) + "폭발로만 부술 수 있습니다. 지름길이 열립니다",
		"       " + padRight("~ 산성 웅덩이", 18) + "밟으면 서서히 녹고 느려집니다. 적도 마찬가지",
		"",
		"오래 머물수록 층 보너스가 줄고 증원이 도착합니다. 상단 [층 x..] 게이지가",
		"차오르면 슬슬 내려갈 때입니다.",
		"",
		"적을 처치하면 경험치가 쌓이고, 레벨업 때 특성 3개 중 하나를 고릅니다.",
		"5층마다 보스가 나오고, 보스를 처치해야 계단이 열립니다. 보스는 두 종류가",
		"번갈아 나옵니다 - 오버시어(O)는 탄막과 소환, 매트리아크(Q)는 브루드와 돌진.",
		"깊은 층의 치유사(m)는 아군 체력을 되돌리니 화력을 아끼지 말 것.",
		"죽으면 처음부터 - 퍼머데스입니다.",
		"[A/D 또는 <-/->] 페이지    그 외 키: 돌아가기",
	}
	pages := [][]string{controls, combat, dungeon}
	g.drawCenterBox(s, fmt.Sprintf("조 작 법 %d/%d", g.helpPage+1, helpPageCount),
		pages[g.helpPage], colHUDAccent)
}

func settingStatus(on bool) string {
	if on {
		return "켜짐"
	}
	return "꺼짐"
}

func (g *Game) drawSettings(s *Screen) {
	row := func(i int, text string) string {
		if i == g.settingRow {
			return "> " + text
		}
		return "  " + text
	}
	lines := []string{
		row(0, fmt.Sprintf("포커스를 잃으면 자동 일시 정지    %s", settingStatus(g.autoPause))),
		row(1, fmt.Sprintf("게임 오버 화면에 현재 시드 표시   %s", settingStatus(g.showSeed))),
		row(2, fmt.Sprintf("자동 무기 변경: 탄약 소진 시 1번     %s", settingStatus(g.autoWeapon))),
		row(3, fmt.Sprintf("화면 흔들림                         %s", settingStatus(g.screenShake))),
		row(4, fmt.Sprintf("화면 배율                         x%d", g.zoom)),
		"",
		"↑/↓ 항목 선택 / ←/→ 조정 / O 또는 ESC로 돌아가기",
	}
	g.drawCenterBox(s, "설 정", lines, colHUDAccent)
}

func (g *Game) drawGameOver(s *Screen) {
	lines := []string{
		fmt.Sprintf("도달 층      B%d층", g.depth),
		fmt.Sprintf("처치         %d", g.kills),
		fmt.Sprintf("점수         %d", g.score),
		fmt.Sprintf("생존 시간    %s", fmtTime(g.elapsed)),
		fmt.Sprintf("레벨         %d", g.player.level),
	}
	if g.showSeed {
		lines = append(lines, fmt.Sprintf("시드         %d", g.seed))
	}
	lines = append(lines, "", "[R] 새로 시작    [Q] 종료")
	g.drawCenterBox(s, "G A M E   O V E R", lines, 196)
}

func init() {
	fullMag.ammo = make([]int, len(weapons))
	for i := range weapons {
		fullMag.ammo[i] = weapons[i].MaxAmmo
	}
}
