package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
)

type State int

const (
	StatePlaying State = iota
	StateLevelUp
	StateWeaponCore
	StateHelp
	StateSettings
	StatePaused
	StateDead
	StateVictory
	StateQuit
)

type Message struct {
	text  string
	color int16
	age   float64
}

type Perk struct {
	Name  string
	Desc  string
	Apply func(p *Player)
}

// InputMode records where key state comes from, which decides how movement is
// resolved and what the help screen tells the player.
type InputMode int

const (
	// InputCompat: terminal keystrokes only, no releases — held keys have to be
	// inferred from auto-repeat.
	InputCompat InputMode = iota
	// InputKitty: the terminal reports press/repeat/release itself.
	InputKitty
	// InputDevice: key state read from /dev/input, gated on window focus.
	InputDevice
	// InputWinConsole: the Windows console reports press and release itself.
	InputWinConsole
)

// trueState reports whether this mode knows when a key is actually released.
func (m InputMode) trueState() bool { return m != InputCompat }

func (m InputMode) String() string {
	switch m {
	case InputKitty:
		return "정밀 (kitty 프로토콜)"
	case InputDevice:
		return "정밀 (/dev/input 직접 읽기)"
	case InputWinConsole:
		return "정밀 (Windows 콘솔)"
	}
	return "호환 (터미널 자동반복)"
}

type Game struct {
	rng   *rand.Rand // simulation: generation, combat, loot and AI
	fxRNG *rand.Rand // presentation only; draw cadence must not change a run
	state State
	seed  int64
	hud   hudScratch

	level   *Level
	depth   int
	player  Player
	enemies []Enemy
	bullets []Bullet
	parts   []Particle
	pickups []Pickup

	nextID   int
	flowTick float64
	// scratch buffers for the per-frame compaction of bullets and enemies, so
	// that spawning during a sweep is safe
	bulletBuf []Bullet
	enemyBuf  []Enemy

	// input
	inputMode     InputMode
	focused       bool      // terminal has focus; device input is ignored when not
	move          *movement // keyboard direction state
	aim           Vec       // world position the mouse points at
	firing        bool
	mouseSet      bool
	quickMeleeBuf float64

	// presentation
	camX, camY float64
	// Previous rendered player cells let a camera already tracking one axis
	// absorb a newly added diagonal component without mixing world scroll and
	// player drift on the other axis.
	camPlayerX, camPlayerY, camPlayerZoom int
	camPlayerTilesW, camPlayerTilesH      int
	camLockX, camLockY                    int
	camPlayerSet                          bool
	// viewport rectangle on screen, in terminal cells (set each frame by Draw)
	viewX, viewY, viewW, viewH int
	// zoom is the requested cells-per-tile; zoomEff is what the current terminal
	// size actually allows, and tilesW/tilesH is the resulting view in tiles.
	// All three are recomputed each frame by Draw.
	zoom, zoomEff  int
	tilesW, tilesH int
	shake          float64
	// whole-cell screen shake offset, applied on top of the camera
	shakeX, shakeY int
	msgs           []Message
	flash          float64
	// juice layer state: damage numbers, corpse marks, kill slow motion and a
	// buffered trigger tap (see fx.go)
	floaters    []FloatText
	decals      []Decal
	hitStop     float64
	fireBuf     float64
	hitFrom     Vec     // direction the last hit came from
	hitFromAge  float64 // seconds since that hit
	showMap     bool
	helpPage    int
	menuReturn  State
	autoPause   bool
	showSeed    bool
	autoWeapon  bool
	screenShake bool
	debugPerf   bool
	perf        perfStats
	settingRow  int

	perkChoices []Perk
	coreChoices []WeaponCore
	coreWeapon  int

	score     int
	kills     int
	elapsed   float64
	stairsPos Vec

	// special rooms
	ambushRoom  int
	ambushOn    bool
	ambushDone  bool
	ambushWave  int
	ambushDoors [][2]int
	shrine      *Shrine

	// descent pressure: how long this floor has been occupied
	floorTime  float64
	reinfTimer float64
	reinfWaves int
	// Objective hints advance only while the floor is open. Sealed ambush
	// rooms suspend the search clock so their mandatory fight does not reveal
	// the exit for free.
	objectiveHintTime float64
	objectiveHintTick float64
	objectiveHint     int

	// interactive terrain
	barrels  []Barrel
	acidTick float64
}

func NewGame(seed int64) *Game {
	return newGameWithInput(seed, InputCompat)
}

// newGameWithInput starts a run using the given keyboard input mode.
func newGameWithInput(seed int64, mode InputMode) *Game {
	g := &Game{
		rng:         rand.New(rand.NewSource(seed)),
		fxRNG:       rand.New(rand.NewSource(seed ^ -7046029254386353131)),
		seed:        seed,
		player:      newPlayer(),
		inputMode:   mode,
		focused:     true,
		autoPause:   true,
		showSeed:    true,
		autoWeapon:  true,
		screenShake: true,
		zoom:        defaultZoom,
		zoomEff:     defaultZoom,
		move:        newMovement(mode.trueState()),
	}
	g.enterDepth(1)
	g.log("WASD 이동 / 마우스 조준 / 좌클릭 사격 / Space 대시 - [?] 도움말", 231)
	return g
}

// ---- floors -----------------------------------------------------------------

func (g *Game) enterDepth(depth int) {
	g.depth = depth
	g.level = GenerateLevel(depth, g.rng)
	g.enemies = g.enemies[:0]
	g.bullets = g.bullets[:0]
	g.parts = g.parts[:0]
	g.pickups = g.pickups[:0]
	g.floaters, g.decals = g.floaters[:0], g.decals[:0]
	g.hitStop, g.fireBuf, g.quickMeleeBuf = 0, 0, 0

	start := g.level.rooms[0]
	sx, sy := start.center()
	g.player.pos = Vec{float64(sx) + 0.5, float64(sy) + 0.5}
	g.player.vel = Vec{}

	ex, ey := g.level.PlaceStairs(sx, sy, g.rng)
	g.stairsPos = Vec{float64(ex) + 0.5, float64(ey) + 0.5}

	boss := depth%5 == 0
	count := 8 + depth*3
	if boss {
		count = 6 + depth
	}
	for i := 0; i < count; i++ {
		def, ok := g.rollEnemy(depth)
		if !ok {
			continue
		}
		x, y := g.level.FreeSpot(g.rng, sx, sy, 22)
		g.addEliteEnemy(scaleForDepth(def, depth), Vec{x, y}, g.rollElite(depth))
	}
	if boss {
		def := bossDef
		// The existing bosses close the first two acts. B15 gets its own owner,
		// so the campaign ends on a new question instead of another rotation.
		if depth == campaignDepth {
			def = coreBossDef
		} else if (depth/5)%2 == 0 {
			def = queenDef
		}
		def.HP *= 1 + float64(depth)*0.35
		def.Damage *= 1 + float64(depth)*0.10
		x, y := g.level.FreeSpot(g.rng, sx, sy, 30)
		g.addEnemy(def, Vec{x, y})
		g.log(fmt.Sprintf("!! %s 의 기척이 느껴진다 !!", def.Name), 196)
		// The floor announces itself in the body of the screen too, not just in
		// the log: a red beat and a rumble say "boss" before it is read.
		g.flash = 0.35
		g.shake = math.Max(g.shake, 0.30)
	}

	// Loot: a couple of medkits, sometimes a new weapon.
	for i := 0; i < 2+depth/3; i++ {
		x, y := g.level.FreeSpot(g.rng, sx, sy, 8)
		g.addPickup(Pickup{pos: Vec{x, y}, kind: PickHealth})
	}
	// Ammunition is guaranteed on every floor: running a whole level on melee
	// because the dice were unkind is not an interesting kind of difficulty.
	for i := 0; i < 3+depth/2; i++ {
		x, y := g.level.FreeSpot(g.rng, sx, sy, 6)
		g.addPickup(Pickup{pos: Vec{x, y}, kind: PickAmmo, weapon: g.rollAmmoFor()})
	}
	if depth >= 2 && g.rng.Float64() < 0.75 {
		x, y := g.level.FreeSpot(g.rng, sx, sy, 10)
		g.addPickup(Pickup{pos: Vec{x, y}, kind: PickWeapon, weapon: g.rollWeapon()})
	}
	if g.rng.Float64() < 0.3 {
		x, y := g.level.FreeSpot(g.rng, sx, sy, 12)
		g.addPickup(Pickup{pos: Vec{x, y}, kind: PickHeart})
	}

	// Special rooms go in last: they need the entrance and the exit already
	// placed so neither of them can be the room that seals you in.
	g.ambushRoom, g.ambushOn, g.ambushDone, g.ambushWave = -1, false, false, 0
	g.ambushDoors, g.shrine = nil, nil
	g.floorTime, g.reinfTimer, g.reinfWaves = 0, reinforceEvery, 0
	g.objectiveHintTime, g.objectiveHintTick, g.objectiveHint = 0, 0, 0
	g.camPlayerSet = false
	g.camLockX, g.camLockY = 0, 0
	g.acidTick = 0
	g.placeBarrels(sx, sy)
	g.assignRooms(0, g.level.RoomAt(g.stairsPos))

	g.level.ComputeFOV(int(g.player.pos.X), int(g.player.pos.Y), 15)
	g.level.BuildFlow(int(g.player.pos.X), int(g.player.pos.Y))
	g.log(fmt.Sprintf("B%d %s 진입", depth, actName(depth)), 117)
}

func (g *Game) rollEnemy(depth int) (EnemyDef, bool) {
	total := 0
	for _, d := range enemyDefs {
		if d.MinDepth <= depth {
			total += d.Weight
		}
	}
	if total == 0 {
		return EnemyDef{}, false
	}
	r := g.rng.Intn(total)
	for _, d := range enemyDefs {
		if d.MinDepth > depth {
			continue
		}
		r -= d.Weight
		if r < 0 {
			return d, true
		}
	}
	return enemyDefs[0], true
}

// giveAmmo tops up a weapon's pool and reports how many rounds fitted.
func (g *Game) giveAmmo(w, n int) int {
	p := &g.player
	space := weapons[w].MaxAmmo - p.ammo[w]
	if n > space {
		n = space
	}
	if n < 0 {
		n = 0
	}
	p.ammo[w] += n
	return n
}

// rollAmmoFor picks which weapon a dropped ammo box belongs to. Only weapons
// the player owns are considered, weighted so that the powerful ones resupply
// far more slowly.
func (g *Game) rollAmmoFor() int {
	total := 0
	for i := range weapons {
		if g.player.owned[i] && weapons[i].needsAmmo() {
			total += weapons[i].AmmoWeight
		}
	}
	if total == 0 {
		return wpPistol
	}
	r := g.rng.Intn(total)
	for i := range weapons {
		if !g.player.owned[i] || !weapons[i].needsAmmo() {
			continue
		}
		r -= weapons[i].AmmoWeight
		if r < 0 {
			return i
		}
	}
	return wpPistol
}

// rollWeapon prefers a weapon the player has not found yet, so floors keep
// handing out something new until the arsenal is complete.
func (g *Game) rollWeapon() int {
	// Only real firearms drop: the pistol is the starting weapon and melee is
	// never picked up.
	var pool, missing []int
	for i := range weapons {
		if i == wpPistol || !weapons[i].needsAmmo() {
			continue
		}
		pool = append(pool, i)
		if !g.player.owned[i] {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		return missing[g.rng.Intn(len(missing))]
	}
	return pool[g.rng.Intn(len(pool))]
}

// addEliteEnemy spawns an enemy carrying one elite rule.
func (g *Game) addEliteEnemy(def EnemyDef, pos Vec, kind EliteKind) {
	if kind == EliteNone {
		g.addEnemy(def, pos)
		return
	}
	g.addEnemy(makeElite(def, kind), pos)
	e := &g.enemies[len(g.enemies)-1]
	e.elite = kind
	if kind == EliteShield {
		e.shield = e.maxHP * shieldFrac
	}
}

// rollElite decides whether a spawn is upgraded, and to what.
func (g *Game) rollElite(depth int) EliteKind {
	if g.rng.Float64() >= eliteChance(depth) {
		return EliteNone
	}
	return eliteKinds[g.rng.Intn(len(eliteKinds))]
}

func (g *Game) addEnemy(def EnemyDef, pos Vec) {
	g.nextID++
	d := def
	// Every caller that computes a spawn point by arithmetic rather than by
	// searching for floor can put one in a wall, and a body that starts inside
	// a wall never gets out. Correct it here so no call site has to remember.
	pos = g.level.NearestFree(pos)
	// The flank side comes from the id, not the RNG: a pack splits into two
	// even wings for free, and no draw shifts the seeded random stream.
	flank := 1.0
	if g.nextID%2 == 0 {
		flank = -1
	}
	g.enemies = append(g.enemies, Enemy{
		id: g.nextID, def: &d, pos: pos, hp: def.HP, maxHP: def.HP,
		phase: g.rng.Float64() * 10, flank: flank, lastHitWeapon: -1, lastDashHit: -999,
	})
}

// ---- input ------------------------------------------------------------------

func (g *Game) HandleEvent(ev Event) {
	switch ev.Kind {
	case EvFocus:
		g.focused = ev.Focus
		if !ev.Focus {
			// Kernel key events keep arriving while another window is focused;
			// dropping the held keys stops the player walking off unattended.
			g.move.releaseAll()
			g.firing = false
			if g.autoPause && g.state == StatePlaying {
				g.state = StatePaused
			}
		}
	case EvKey:
		directionMenu := g.state == StateHelp || g.state == StateSettings
		if ev.Src == SrcDevice {
			if g.state != StatePlaying {
				// Device directions must not affect a modal screen, but their
				// releases still have to clear keys held before it opened. Dropping
				// the release makes the player bolt away when play resumes.
				if ev.KeyAct == KeyRelease {
					g.move.release(ev.Dir)
				}
				return
			}
			g.handleDeviceKey(ev)
			return
		}
		// With /dev/input driving movement, the duplicate keystrokes the
		// terminal also delivers would fight the real key state. Directional
		// menus still need the terminal copy because device events only carry a
		// movement direction, not the original key.
		if g.inputMode == InputDevice && !directionMenu && dirFor(ev.Rune, ev.Key) >= 0 {
			return
		}
		g.handleKey(ev)
	case EvMouse:
		g.handleMouse(ev)
	}
}

func (g *Game) handleDeviceKey(ev Event) {
	if !g.focused || g.inputMode != InputDevice {
		return
	}
	if ev.KeyAct == KeyRelease {
		g.move.release(ev.Dir)
		return
	}
	g.move.press(ev.Dir, g.elapsed, ev.KeyAct == KeyRepeat)
}

func (g *Game) handleKey(ev Event) {
	// Key releases only matter for movement: letting go of a direction stops
	// it immediately instead of waiting for a hold timer to lapse.
	if ev.KeyAct == KeyRelease {
		g.move.release(dirFor(ev.Rune, ev.Key))
		return
	}
	// True-state inputs report auto-repeat explicitly. Repeats keep movement and
	// directional menus responsive, but action keys must remain edge-triggered:
	// a repeated ESC used to resume and immediately pause again.
	if ev.KeyAct == KeyRepeat && dirFor(ev.Rune, ev.Key) < 0 {
		return
	}
	if ev.Key == KeyCtrlC {
		g.state = StateQuit
		return
	}
	r := ev.Rune
	if r >= 'A' && r <= 'Z' {
		r += 32
	}

	switch g.state {
	case StateLevelUp:
		if r >= '1' && r <= '9' {
			i := int(r - '1')
			if i < len(g.perkChoices) {
				g.perkChoices[i].Apply(&g.player)
				g.log("특성 획득: "+g.perkChoices[i].Name, 213)
				g.perkChoices = nil
				g.state = StatePlaying
			}
		}
		return
	case StateWeaponCore:
		if r >= '1' && r <= '9' {
			i := int(r - '1')
			if i < len(g.coreChoices) {
				core := g.coreChoices[i]
				g.player.cores[g.coreWeapon] |= core
				g.log(weapons[g.coreWeapon].Name+" 진화: "+core.Name(), 51)
				g.coreChoices = nil
				if len(g.perkChoices) > 0 {
					g.state = StateLevelUp
				} else {
					g.state = StatePlaying
				}
			}
		}
		return
	case StateHelp:
		switch {
		case ev.Key == KeyLeft || r == 'a':
			g.helpPage = (g.helpPage + helpPageCount - 1) % helpPageCount
		case ev.Key == KeyRight || r == 'd' || ev.Key == KeyTab:
			g.helpPage = (g.helpPage + 1) % helpPageCount
		case r == 'o':
			g.state = StateSettings
		default:
			g.state = g.menuReturn
		}
		return
	case StateSettings:
		switch {
		case ev.Key == KeyUp:
			g.settingRow = (g.settingRow + settingsCount - 1) % settingsCount
		case ev.Key == KeyDown:
			g.settingRow = (g.settingRow + 1) % settingsCount
		case ev.Key == KeyLeft:
			g.adjustSetting(-1)
		case ev.Key == KeyRight:
			g.adjustSetting(1)
		case r == 'o' || ev.Key == KeyEscape:
			g.state = g.menuReturn
		}
		return
	case StateDead, StateVictory:
		if r == 'r' {
			zoom := g.zoom // a restart should not undo the player's view setting
			autoPause, showSeed := g.autoPause, g.showSeed
			autoWeapon, screenShake := g.autoWeapon, g.screenShake
			debugPerf := g.debugPerf
			*g = *newGameWithInput(g.rng.Int63(), g.inputMode)
			g.zoom = zoom
			g.autoPause, g.showSeed = autoPause, showSeed
			g.autoWeapon, g.screenShake = autoWeapon, screenShake
			g.debugPerf = debugPerf
		} else if r == 'q' || ev.Key == KeyEscape {
			g.state = StateQuit
		}
		return
	}

	switch {
	case r == 'q':
		g.state = StateQuit
	case ev.Key == KeyEscape || r == 'p':
		if g.state == StatePaused {
			g.state = StatePlaying
		} else {
			g.state = StatePaused
		}
	case r == '?' || r == '/' || r == 'h':
		g.menuReturn = g.state
		g.helpPage = 0
		g.state = StateHelp
	case r == 'o':
		g.menuReturn = g.state
		g.state = StateSettings
	case ev.Key == KeyTab || r == 'm':
		g.showMap = !g.showMap
	case ev.Key == KeySpace:
		if g.state == StatePlaying {
			g.startDash()
		}
	case r == 'f':
		if g.state == StatePlaying {
			g.quickMeleeBuf = quickMeleeBufferTime
		}
	case r == '[':
		if g.state == StatePlaying {
			g.cycleWeapon(-1)
		}
	case r == ']':
		if g.state == StatePlaying {
			g.cycleWeapon(1)
		}
	case r == 'e':
		if g.state == StatePlaying {
			g.tryStairs()
		}
	case r >= '1' && r <= '6':
		if g.state == StatePlaying {
			g.selectWeapon(int(r - '1'))
		}
	default:
		if g.state == StatePlaying {
			g.move.press(dirFor(r, ev.Key), g.elapsed, ev.KeyAct == KeyRepeat)
		}
	}
}

const settingsCount = 6

func (g *Game) adjustSetting(direction int) {
	on := direction > 0
	switch g.settingRow {
	case 0:
		g.autoPause = on
	case 1:
		g.showSeed = on
	case 2:
		g.autoWeapon = on
	case 3:
		g.screenShake = on
	case 4:
		g.setZoom(g.zoom + direction)
	case 5:
		g.debugPerf = on
		g.perf = perfStats{}
	}
}

// setZoom changes how large the dungeon is drawn. The message reports the level
// the terminal can actually show, which is not always the one asked for.
func (g *Game) setZoom(z int) {
	z = clamp(z, minZoom, maxZoom)
	if z == g.zoom {
		return
	}
	g.zoom = z
	eff := fitZoom(z, g.viewW, g.viewH)
	if eff != z {
		g.log(fmt.Sprintf("배율 x%d - 터미널이 좁아 x%d 로 표시합니다.", z, eff), 214)
		return
	}
	g.log(fmt.Sprintf("배율 x%d", z), colHUDAccent)
}

// selectWeapon handles the number row. Melee is always selectable; anything
// else has to have been picked up and have rounds left.
func (g *Game) selectWeapon(i int) {
	p := &g.player
	switch {
	case i < 0 || i >= len(weapons):
	case !p.owned[i]:
		g.log(fmt.Sprintf("%d번 무기(%s)는 아직 획득하지 않았다.", i+1, weapons[i].Name), 244)
	case weapons[i].needsAmmo() && p.ammo[i] <= 0:
		g.log(fmt.Sprintf("%s 탄약이 없다.", weapons[i].Name), 203)
	default:
		p.weapon = i
		g.log(fmt.Sprintf("무기: %s ([%d])", weapons[i].Name, i+1), weapons[i].Color)
	}
}

// cycleWeapon walks only through weapons that can fire right now. A wheel
// tick should never strand the player on an empty or undiscovered slot.
func (g *Game) cycleWeapon(direction int) {
	if direction == 0 {
		return
	}
	p := &g.player
	step := 1
	if direction < 0 {
		step = -1
	}
	for n, i := 0, p.weapon; n < len(weapons); n++ {
		i = (i + step + len(weapons)) % len(weapons)
		if !p.owned[i] || (weapons[i].needsAmmo() && p.ammo[i] <= 0) {
			continue
		}
		p.weapon = i
		g.log(fmt.Sprintf("무기: %s ([%d])", weapons[i].Name, i+1), weapons[i].Color)
		return
	}
}

func (g *Game) pressMove(r rune, k Key) {
	g.move.press(dirFor(r, k), g.elapsed, false)
}

func (g *Game) handleMouse(ev Event) {
	g.mouseSet = true
	g.aim = g.screenToWorld(ev.MX, ev.MY)
	if g.state != StatePlaying {
		// A click made while an overlay is open must not become a shot after
		// returning to play. Releases still clear a button held beforehand.
		if ev.Action == MouseRelease || (ev.Action == MouseMove && ev.Button == BtnNone) {
			g.firing = false
		}
		return
	}
	switch ev.Action {
	case MousePress:
		if ev.Button == BtnLeft {
			g.firing = true
			g.fireBuf = fireBufferTime
		} else if ev.Button == BtnRight {
			g.startDash()
		} else if ev.Button == BtnMiddle {
			g.quickMeleeBuf = quickMeleeBufferTime
		} else if ev.Button == BtnWheelUp {
			g.cycleWeapon(-1)
		} else if ev.Button == BtnWheelDown {
			g.cycleWeapon(1)
		}
	case MouseRelease:
		if ev.Button == BtnLeft {
			g.firing = false
		}
	case MouseMove:
		// Button bits are still reported while dragging; keep firing.
		if ev.Button == BtnNone {
			g.firing = false
		}
	}
}

func (g *Game) startDash() {
	p := &g.player
	if g.state != StatePlaying {
		return
	}
	if p.dashTimer > 0 || p.dashRecovery > 0 || p.dashEnergy+1e-9 < dashEnergyCost {
		// Too early: queue it instead of dropping it on the floor.
		p.dashBuffer = dashBufferTime
		return
	}
	dir := g.moveDir()
	if dir.len() < 0.01 {
		dir = aimDir(p.pos, g.aim)
	}
	if dir.len() < 0.01 {
		return
	}
	nextDir := dir.norm()
	gap := g.elapsed - p.lastDashEnd
	continuity := 0.0
	if p.dashMomentum > 0 && gap < dashMomentumFadeGap {
		p.dashMomentum = min(p.dashMomentum+1, 3)
		continuity = 1 - clampF((gap-dashMomentumFullGap)/
			(dashMomentumFadeGap-dashMomentumFullGap), 0, 1)
	} else {
		p.dashMomentum = 1
	}
	speedMul := 1 + dashMomentumStep*float64(p.dashMomentum-1)*continuity
	distance := dashDistance + dashDistanceStep*float64(p.dashMomentum-1)*continuity
	straightBonus := 0.0
	if p.dashMomentum == 2 {
		straightBonus = dashStraightBonus2
	} else if p.dashMomentum >= 3 {
		straightBonus = dashStraightBonus3
	}
	distance += straightBonus * dashStraightness(p.dashDir, nextDir) * continuity
	p.dashSpeed = (dashDistance / dashDuration) * speedMul
	duration := distance / p.dashSpeed
	p.dashDir = nextDir
	p.dashTimer = duration
	p.dashStepDistance = distance
	p.dashEnergy -= dashEnergyCost
	p.dashRegenWait = dashRegenDelay
	p.invuln = math.Max(p.invuln, duration)
	// The blink is also a shoulder charge: reset its hit list and the
	// perfect-dodge chain so repeated taps cannot print rewards in bulk.
	p.lastDashStart = g.elapsed
	if p.dashChainTimer <= 0 {
		p.dodgedThisChain = false
	}
	p.dashChainTimer = dashChainWindow
	p.dashHitIDs = p.dashHitIDs[:0]
	g.spawnParticles(p.pos, 6, 0.25, 51, '.')
}

func dashStraightness(previous, next Vec) float64 {
	dot := clampF(previous.X*next.X+previous.Y*next.Y, 0, 1)
	return dot * dot
}

func (g *Game) moveDir() Vec { return g.move.dir(g.elapsed) }

// dashStrike hurts every enemy the live blink passes through, once each. It
// runs after the movement step so the hit lands where the body actually is.
func (g *Game) dashStrike() {
	p := &g.player
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.hp <= 0 {
			continue
		}
		// A body mid-sidestep is airborne: it slips past the shoulder. This
		// is not free — the hop lasts exactly as long as a dash and is then
		// locked out for over a second, which is the punish window.
		if e.dodgeT > 0 {
			continue
		}
		if vdist(e.pos, p.pos) > e.def.Radius+p.radius+0.25 {
			continue
		}
		already := false
		for _, id := range p.dashHitIDs {
			if id == e.id {
				already = true
				break
			}
		}
		if already {
			continue
		}
		if g.elapsed-e.lastDashHit < dashRepeatHitCD {
			continue
		}
		p.dashHitIDs = append(p.dashHitIDs, e.id)
		e.lastDashHit = g.elapsed
		e.lastHitWeapon = wpMelee
		dmg := dashStrikeDamage * p.damageMul * p.meleeMul
		g.hurtEnemyFrom(e, dmg, p.dashDir.visual().Scale(dashStrikeKnock), damageSecondary)
		g.spawnParticles(e.pos, 6, 0.25, 51, '/')
		g.shake = math.Max(g.shake, 0.10)
	}
}

// stairsBlocked reports whether the stairs are sealed. A living boss holds
// them shut: the floor's owner decides when you leave, not your nerve.
func (g *Game) stairsBlocked() bool { return g.bossEnemy() != nil }

// tryStairs handles the [E] key, which is also how a shrine is used. Both are
// "the thing you are standing next to", so one key covers them.
func (g *Game) tryStairs() {
	if g.nearShrine() {
		g.useShrine()
		return
	}
	if g.ambushOn {
		g.log("문이 닫혀 있다. 적을 모두 처치해야 한다.", 203)
		return
	}
	if vdist(g.player.pos, g.stairsPos) < 2.0 && g.stairsBlocked() {
		g.log(fmt.Sprintf("%s를 처치해야 계단이 열린다.", g.bossEnemy().def.Name), 196)
		return
	}
	if vdist(g.player.pos, g.stairsPos) < 2.0 {
		mul := g.descentMul()
		bonus := int(float64(50*g.depth) * mul)
		g.score += bonus
		if g.depth >= campaignDepth {
			g.state = StateVictory
			g.log("심층 코어를 돌파했다. 지상으로 귀환한다.", 226)
			g.flash = 0.35
			return
		}
		g.enterDepth(g.depth + 1)
		g.log(fmt.Sprintf("B%d층으로 내려간다. (보너스 %d, 배율 x%.2f)", g.depth, bonus, mul), 117)
		g.flash = 0.25
	}
}

// ---- simulation -------------------------------------------------------------

func (g *Game) Update(dt float64) {
	if g.state != StatePlaying {
		// Messages keep ageing so the log does not freeze mid-overlay.
		g.ageMessages(dt)
		return
	}
	// Kill slow motion: right after a kill the world briefly runs at a fraction
	// of speed, which reads as impact. The clock that pays for it is real time,
	// so a slow-motion beat always lasts the same wall-clock length.
	raw := dt
	if g.hitStop > 0 {
		g.hitStop -= raw
		dt *= killSlowmoScale
	}
	g.elapsed += dt
	p := &g.player
	p.lifestealBudget = math.Min(p.lifestealCapPerSecond(),
		p.lifestealBudget+p.lifestealCapPerSecond()*dt)

	// --- player movement: intent-driven steering ----------------------------
	// A buffered chain starts at a frame boundary, before movement and
	// invulnerability are aged. Starting it at the tail of the previous frame
	// made the protection expire before the body actually moved.
	if p.dashBuffer > 0 && p.dashTimer <= 0 && p.dashRecovery <= 0 &&
		p.dashEnergy+1e-9 >= dashEnergyCost {
		p.dashBuffer = 0
		g.startDash()
	}
	dir := g.moveDir()
	dashing := p.dashTimer > 0
	coasting := !dashing && p.dashCoast > 0
	if coasting && dir.len() > 0.01 {
		in := dir.norm()
		if p.dashDir.X*in.X+p.dashDir.Y*in.Y < -0.2 {
			// An explicit reversal means the player wanted one precise dash, not
			// a slide that keeps carrying them past their intended stopping point.
			p.dashCoast = 0
			p.vel = Vec{}
			coasting = false
		}
	}
	coastMoveDT := math.Min(dt, p.dashCoast)
	coastPhase0 := clampF(p.dashCoast/dashCoastDuration, 0, 1)
	coastPhase1 := clampF((p.dashCoast-coastMoveDT)/dashCoastDuration, 0, 1)
	moveDT := dt
	if p.dashRecovery > 0 {
		p.dashRecovery = math.Max(0, p.dashRecovery-dt)
	}
	if p.dashCoast > 0 {
		p.dashCoast = math.Max(0, p.dashCoast-dt)
	}
	if p.dashChainTimer > 0 {
		p.dashChainTimer -= dt
		if p.dashChainTimer <= 0 {
			p.dodgedThisChain = false
		}
	}
	if p.dashRegenWait > 0 {
		p.dashRegenWait -= dt
	} else {
		p.dashEnergy = math.Min(p.dashEnergyMax, p.dashEnergy+p.dashRegenPerSecond()*dt)
	}
	if dashing {
		moveDT = math.Min(dt, p.dashTimer)
		p.dashTimer -= dt
		if p.dashTimer <= 0 {
			p.dashTimer = 0
			p.dashRecovery = math.Max(p.dashRecovery, math.Max(0, dashRecovery-(dt-moveDT)))
			p.dashCoast = dashCoastDuration
			p.lastDashEnd = g.elapsed - (dt - moveDT)
		}
		p.vel = p.dashDir.Scale(p.dashSpeed)
		// Afterimage: a fading ghost of the player along the dash line, so the
		// blink reads as movement rather than teleporting.
		g.parts = append(g.parts, Particle{
			pos: p.pos, life: 0.22, max: 0.22, glyph: '@', color: 45, hold: true,
		})
	} else if coasting {
		// Integrate the quadratic falloff over this frame. Using the interval
		// average keeps the coast distance stable through a 0.1s frame stall.
		curve := (coastPhase0*coastPhase0 + coastPhase0*coastPhase1 +
			coastPhase1*coastPhase1) / 3
		coastSpeed := p.baseSpeed() * (1 + (dashCoastStartMul-1)*curve)
		coastDir := p.dashDir
		if dir.len() > 0.01 {
			steer := 1 - (coastPhase0+coastPhase1)/2
			coastDir = p.dashDir.Scale(1 - steer).Add(dir.norm().Scale(steer)).norm()
		}
		if g.inAcid(p.pos) {
			coastSpeed *= acidSlow
		}
		moveDT = coastMoveDT
		p.vel = coastDir.Scale(coastSpeed)
		g.parts = append(g.parts, Particle{
			pos: p.pos, life: 0.12, max: 0.12, glyph: '.', color: 37, hold: true,
		})
	} else {
		top := p.baseSpeed()
		if g.inAcid(p.pos) {
			top *= acidSlow // wading, not walking
		}
		target := dir.Scale(top)
		p.vel = steerMovementVelocity(p.vel, target, dt)
	}
	if p.dashBuffer > 0 {
		p.dashBuffer -= dt
		if p.dashBuffer <= 0 {
			p.dashBuffer = 0
		}
	}
	if p.invuln > 0 {
		p.invuln -= dt
	}
	g.moveWithCollision(&p.pos, p.vel.unvisual().Scale(moveDT), p.radius)
	if dashing {
		g.dashStrike()
		if p.dashTimer <= 0 {
			p.vel = p.dashDir.Scale(p.baseSpeed() * dashCoastStartMul)
		}
	} else if coasting && p.dashCoast <= 0 {
		p.vel = p.dashDir.Scale(p.baseSpeed())
	}

	// --- shooting -----------------------------------------------------------
	if p.cooldown > 0 {
		p.cooldown -= dt
	}
	// Quick melee borrows the shared attack cooldown but never changes the
	// equipped weapon. It is a defensive cancel, not another inventory chore.
	if g.quickMeleeBuf > 0 && p.cooldown <= 0 {
		g.quickMeleeBuf = 0
		dir := g.moveDir()
		if g.mouseSet {
			dir = aimDir(p.pos, g.aim)
		}
		if dir.len() < 0.01 {
			dir = Vec{X: 1}
		}
		g.meleeSwing(p.pos, dir)
		p.cooldown = weapons[wpMelee].Cooldown / p.fireMul
	}
	// A trigger tap that lands during the cooldown is buffered the same way as
	// dash, so a deliberate single click is never swallowed by a bad frame.
	trigger := g.firing || (g.fireBuf > 0 && g.mouseSet)
	if trigger && g.mouseSet && p.cooldown <= 0 {
		g.fireBuf = 0
		w := &weapons[p.weapon]
		g.shoot(p.pos, aimDir(p.pos, g.aim))
		p.cooldown = w.Cooldown / p.fireMul
	}
	if g.fireBuf > 0 {
		g.fireBuf -= dt
	}
	if g.quickMeleeBuf > 0 {
		g.quickMeleeBuf -= dt
		if g.quickMeleeBuf < 0 {
			g.quickMeleeBuf = 0
		}
	}

	// --- visibility & pathing ----------------------------------------------
	g.level.ComputeFOV(int(p.pos.X), int(p.pos.Y), 15)
	g.flowTick -= dt
	if g.flowTick <= 0 {
		g.level.BuildFlow(int(p.pos.X), int(p.pos.Y))
		g.flowTick = 0.2
	}

	g.updateBullets(dt)

	for i := range g.enemies {
		e := &g.enemies[i]
		g.updateEnemy(e, dt)
		// Boss attacks can append summons and move the slice to a new backing
		// array. Preserve the boss updates made through the old pointer; later
		// iterations already index the current slice and need no correction.
		if e != &g.enemies[i] {
			g.enemies[i] = *e
		}
	}
	g.separateEnemies()
	g.reapEnemies()

	g.updatePickups(dt)
	g.updateParticles(dt)
	g.updateFX(dt)
	g.stairsShimmer()
	g.updateBarrels(dt)
	g.updateHazards(dt)
	g.updateAmbush(dt)
	g.updatePressure(dt)
	g.updateObjectiveHint(dt)

	if g.shake > 0 {
		g.shake -= dt
	}
	if g.flash > 0 {
		g.flash -= dt
	}
	if g.hitFromAge < hitDirTime {
		g.hitFromAge += dt
	}
	g.ageMessages(dt)

	if p.hp <= 0 {
		g.state = StateDead
	}
}

func (g *Game) ageMessages(dt float64) {
	out := g.msgs[:0]
	for _, m := range g.msgs {
		m.age += dt
		if m.age < 6 {
			out = append(out, m)
		}
	}
	g.msgs = out
}

// moveWithCollision slides an entity along walls by resolving each axis
// separately. When an axis is blocked on one side only — walking into a
// doorway slightly off-centre, or hugging a corridor wall — the body is nudged
// sideways so it slips past instead of stopping dead. Without that assist a
// one-tile gap is nearly impassable, since the player would have to be aligned
// to within a fraction of a cell.
func (g *Game) moveWithCollision(pos *Vec, delta Vec, radius float64) {
	rx := math.Min(radius, maxCollisionRadius)
	ry := rx / aspect

	// A single frame can ask for a move several tiles long: a dash, a charging
	// Brute, or any frame that stalled up to the application loop's 0.1s cap. Resolving
	// that in one shot only ever tests the destination, so a body steps clean
	// over anything in between — at dash speed a 0.1s frame is 5.1 tiles, which
	// crosses a two-tile-thick wall and lands on the far side. Advance in
	// pieces no longer than the body, the way bullets already do.
	steps := maxInt(1, maxInt(int(math.Abs(delta.X)/rx), int(math.Abs(delta.Y)/ry)))
	if steps > 1 {
		steps = minInt(steps, 64)
		part := delta.Scale(1 / float64(steps))
		for i := 0; i < steps; i++ {
			g.moveStep(pos, part, rx, ry)
		}
		return
	}
	g.moveStep(pos, delta, rx, ry)
}

func (g *Game) moveStep(pos *Vec, delta Vec, rx, ry float64) {
	if delta.X != 0 {
		nx := pos.X + delta.X
		if !g.blocked(nx, pos.Y, rx, ry) {
			pos.X = nx
		} else if dy, ok := g.slip(nx, pos.Y, rx, ry, math.Abs(delta.X)/aspect*slipRate, false); ok {
			if !g.blocked(pos.X, pos.Y+dy, rx, ry) {
				pos.Y += dy
			}
			if !g.blocked(nx, pos.Y, rx, ry) {
				pos.X = nx
			}
		}
	}
	if delta.Y != 0 {
		ny := pos.Y + delta.Y
		if !g.blocked(pos.X, ny, rx, ry) {
			pos.Y = ny
		} else if dx, ok := g.slip(pos.X, ny, rx, ry, math.Abs(delta.Y)*aspect*slipRate, true); ok {
			if !g.blocked(pos.X+dx, pos.Y, rx, ry) {
				pos.X += dx
			}
			if !g.blocked(pos.X, ny, rx, ry) {
				pos.Y = ny
			}
		}
	}
}

const (
	// Bodies wider than this could never fit through a corridor, so large
	// enemies keep their big hitbox but collide with a smaller one.
	maxCollisionRadius = 0.40
	// How far the corner assist may push per unit of blocked movement.
	slipRate = 1.5
)

// slip reports how far to shift perpendicular to a blocked move so the body
// clears the wall it is clipping. It only helps when exactly one side is
// obstructed; if both sides are walls there is genuinely no way through.
func (g *Game) slip(x, y, rx, ry, maxShift float64, shiftX bool) (float64, bool) {
	var lowBlocked, highBlocked bool
	if shiftX {
		lowBlocked = g.level.Solid(int(x-rx), int(y-ry)) || g.level.Solid(int(x-rx), int(y+ry))
		highBlocked = g.level.Solid(int(x+rx), int(y-ry)) || g.level.Solid(int(x+rx), int(y+ry))
	} else {
		lowBlocked = g.level.Solid(int(x-rx), int(y-ry)) || g.level.Solid(int(x+rx), int(y-ry))
		highBlocked = g.level.Solid(int(x-rx), int(y+ry)) || g.level.Solid(int(x+rx), int(y+ry))
	}
	if lowBlocked == highBlocked {
		return 0, false
	}

	lo, hi, r := y-ry, y+ry, ry
	if shiftX {
		lo, hi, r = x-rx, x+rx, rx
	}
	var want float64
	if lowBlocked {
		want = math.Floor(lo) + 1 + r + 1e-6 // push towards the free side
		if shiftX {
			want -= x
		} else {
			want -= y
		}
		return math.Min(want, maxShift), true
	}
	want = math.Ceil(hi) - 1 - r - 1e-6
	if shiftX {
		want -= x
	} else {
		want -= y
	}
	return math.Max(want, -maxShift), true
}

// clearOfWalls nudges a body until its hitbox no longer overlaps a solid tile,
// walking it towards `toward` if it has to.
//
// Having your centre on floor is not enough to be able to move: a body is wider
// than a point, and a door that shuts across it leaves it standing just inside
// the room with its shoulder still in the doorway. From there most directions
// are refused and it reads as being caught in the door. Stepping fully into the
// tile it is already standing on is almost always enough.
func (g *Game) clearOfWalls(pos *Vec, radius float64, toward Vec) {
	rx := math.Min(radius, maxCollisionRadius)
	ry := rx / aspect
	if !g.blocked(pos.X, pos.Y, rx, ry) {
		return
	}
	fits := func(p Vec) bool {
		return !g.level.SolidAtPoint(p) && !g.blocked(p.X, p.Y, rx, ry)
	}
	middle := func(p Vec) Vec {
		return Vec{math.Floor(p.X) + 0.5, math.Floor(p.Y) + 0.5}
	}
	if c := middle(*pos); fits(c) {
		*pos = c
		return
	}
	dir := toward.Sub(*pos).visual()
	if dir.len() < 1e-6 {
		return
	}
	step := dir.norm().unvisual()
	p := *pos
	for i := 0; i < 4; i++ {
		p = p.Add(step)
		if c := middle(p); fits(c) {
			*pos = c
			return
		}
	}
}

func (g *Game) blocked(x, y, rx, ry float64) bool {
	for _, o := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		if g.level.Solid(int(x+o[0]*rx), int(y+o[1]*ry)) {
			return true
		}
	}
	return false
}

func (g *Game) updateBullets(dt float64) {
	// Compact into a scratch buffer rather than over g.bullets itself: a hit
	// can spawn more bullets (an elite answering the shot), and appending to
	// the slice being overwritten would drop them on the floor.
	n := len(g.bullets)
	out := g.bulletBuf[:0]
	for i := 0; i < n; i++ {
		b := g.bullets[i]
		b.life -= dt
		if b.life <= 0 {
			continue
		}
		steps := 1 + int(b.vel.visual().len()*dt/0.5)
		hit := false
		for s := 0; s < steps && !hit; s++ {
			b.pos = b.pos.Add(b.vel.Scale(dt / float64(steps)))
			if g.level.SolidAtPoint(b.pos) {
				if b.explode {
					if b.friendly {
						g.markBlastSource(b.pos, 4.5, b.weapon)
					}
					g.explode(b.pos, 4.5, b.dmg, b.friendly)
				} else {
					g.spawnParticles(b.pos, 2, 0.12, 240, '.')
				}
				hit = true
				break
			}
			// Either side's shots set a barrel off, so a stray enemy round can
			// blow up the room too.
			if bar := g.barrelAt(b.pos, 0.25); bar != nil {
				if b.explode {
					if b.friendly {
						g.markBlastSource(b.pos, 4.5, b.weapon)
					}
					g.explode(b.pos, 4.5, b.dmg, b.friendly)
				} else {
					g.hurtBarrel(bar, b.dmg)
				}
				hit = true
				break
			}
			if b.friendly {
				hit = g.bulletVsEnemies(&b)
			} else if g.player.invuln <= 0 &&
				vdist(b.pos, g.player.pos) < g.player.radius+0.25 {
				g.damagePlayer(b.dmg, b.vel.visual().norm())
				hit = true
			}
		}
		if !hit {
			out = append(out, b)
		}
	}
	// Carry over anything spawned during the loop; it lives past the original
	// length and has not been simulated yet.
	out = append(out, g.bullets[n:]...)
	g.bulletBuf, g.bullets = g.bullets, out
}

// bulletVsEnemies returns true when the bullet should be removed.
func (g *Game) bulletVsEnemies(b *Bullet) bool {
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.hp <= 0 || vdist(b.pos, e.pos) > e.def.Radius+0.3 {
			continue
		}
		already := false
		for _, id := range b.hitIDs {
			if id == e.id {
				already = true
				break
			}
		}
		if already {
			continue
		}
		if b.explode {
			g.markBlastSource(b.pos, 4.5, b.weapon)
			g.explode(b.pos, 4.5, b.dmg, true)
			return true
		}
		e.lastHitWeapon = b.weapon
		g.hurtEnemy(e, b.dmg, b.vel.visual().norm().Scale(b.knock))
		b.hitIDs = append(b.hitIDs, e.id)
		if b.pierce > 0 {
			b.pierce--
			b.dmg *= 0.8
			return false
		}
		return true
	}
	return false
}

type enemyDamageSource uint8

const (
	damageSecondary enemyDamageSource = iota
	damageWeapon
)

// hurtEnemy is the direct weapon-hit path. Secondary effects must opt out of
// lifesteal through hurtEnemyFrom so damage chains cannot become healing loops.
func (g *Game) hurtEnemy(e *Enemy, dmg float64, knock Vec) {
	g.hurtEnemyFrom(e, dmg, knock, damageWeapon)
}

func (g *Game) hurtEnemyFrom(e *Enemy, dmg float64, knock Vec, source enemyDamageSource) {
	// A live shield soaks the hit first and only the remainder reaches the
	// body. Absorbing in fractions (rather than voiding whole hits) keeps
	// shotguns and the railgun honest against it: big numbers still punch
	// through, small ones are what the shield is for.
	if e.shield > 0 {
		absorbed := minF(dmg, e.shield)
		e.shield -= absorbed
		e.hurt, e.sinceHurt, e.alert = 0.12, 0, true
		g.spawnParticles(e.pos, 3, 0.18, 39, ']')
		g.addFloater(Vec{e.pos.X, e.pos.Y - 0.3}, "막", 39)
		dmg -= absorbed
		if dmg <= 0.01 {
			return
		}
	}
	// Lifesteal pays out on health actually removed, not on the number rolled.
	// Otherwise overkill is free healing: eight shotgun pellets into a 10 HP
	// Swarmling deal 72, and the 62 that hit a corpse used to heal you as much
	// as the 10 that killed it.
	landed := math.Min(dmg, math.Max(0, e.hp))
	e.hp -= dmg
	e.hurt = 0.12
	e.sinceHurt = 0
	e.alert = true
	if e.def.Speed > 0 {
		g.moveWithCollision(&e.pos, knock.unvisual().Scale(0.10*e.def.knockMul()), e.def.Radius)
	}
	g.spawnParticles(e.pos, 3, 0.2, 174, '`')
	// The damage number is the shot's receipt: without it, a fast weapon feels
	// like it is doing nothing until something dies.
	col := int16(231)
	if e.hp <= 0 {
		col = colStairs // the killing blow pays off in the exit colour
	}
	g.addFloater(Vec{
		e.pos.X + g.fxRNG.Float64()*0.8 - 0.4,
		e.pos.Y - 0.3 - g.fxRNG.Float64()*0.4,
	}, strconv.Itoa(int(dmg+0.5)), col)
	if source == damageWeapon && g.player.lifesteal > 0 && landed > 0 {
		requested := landed * g.player.lifesteal
		healed := math.Min(requested, g.player.lifestealBudget)
		healed = math.Min(healed, math.Max(0, g.player.maxHP-g.player.hp))
		if healed > 0 {
			g.player.lifestealBudget -= healed
			g.heal(healed)
		}
	}
	g.eliteOnHurt(e)
}

func (g *Game) reapEnemies() {
	// Survivors first, into a scratch buffer. Death rewards run afterwards
	// because a splitting elite spawns new enemies, and doing that while
	// compacting g.enemies in place would lose them.
	n := len(g.enemies)
	out := g.enemyBuf[:0]
	var dead []Enemy
	for i := 0; i < n; i++ {
		if g.enemies[i].hp > 0 {
			out = append(out, g.enemies[i])
		} else {
			dead = append(dead, g.enemies[i])
		}
	}
	out = append(out, g.enemies[n:]...)
	g.enemyBuf, g.enemies = g.enemies, out

	for _, e := range dead {
		g.kills++
		g.score += e.def.Score
		g.gainXP(e.def.XP)
		g.spawnParticles(e.pos, 10, 0.45, 167, '*')
		g.shake = math.Max(g.shake, 0.08)
		g.addDecal(e.pos)
		// A beat of slow motion sells the kill; bosses get a longer one.
		if e.def.Behavior == BeBoss {
			g.hitStop = math.Max(g.hitStop, bossSlowmo)
		} else {
			g.hitStop = math.Max(g.hitStop, killSlowmo)
		}
		g.eliteOnDeath(&e)
		g.triggerRupture(e.lastHitWeapon, e.pos)
		if e.def.Behavior == BeBoss {
			g.log(fmt.Sprintf("%s 격파! 계단이 열렸다.", e.def.Name), 226)
			g.addPickup(Pickup{pos: e.pos, kind: PickHeart})
			if g.depth%floorsPerAct == 0 && g.depth < campaignDepth {
				g.offerWeaponCores(e.lastHitWeapon)
			}
		} else if e.elite != EliteNone {
			g.log(fmt.Sprintf("%s 처치!", e.def.Name), e.def.Color)
			g.addPickup(Pickup{pos: e.pos, kind: PickAmmo, weapon: g.rollAmmoFor()})
			if g.rng.Float64() < eliteMedkitDrop {
				g.addPickup(Pickup{pos: e.pos.Add(Vec{0.8, 0}), kind: PickHealth})
			}
		} else {
			// Running completely dry is meant to be a scare, not a death
			// spiral: with every magazine empty, kills resupply far more often.
			ammoChance := 0.22
			if g.player.totalAmmo() == 0 {
				ammoChance = 0.6
			}
			if r := g.rng.Float64(); r < normalMedkitDrop {
				g.addPickup(Pickup{pos: e.pos, kind: PickHealth})
			} else if r < normalMedkitDrop+ammoChance {
				g.addPickup(Pickup{pos: e.pos, kind: PickAmmo, weapon: g.rollAmmoFor()})
			}
		}
	}
}

// separateEnemies keeps bodies from stacking into a single glyph.
func (g *Game) separateEnemies() {
	for i := range g.enemies {
		for j := i + 1; j < len(g.enemies); j++ {
			a, b := &g.enemies[i], &g.enemies[j]
			d := b.pos.Sub(a.pos).visual()
			min := a.def.Radius + b.def.Radius
			l := d.len()
			if l > min || l < 1e-6 {
				continue
			}
			push := d.norm().Scale((min - l) * 0.25).unvisual()
			if a.def.Speed > 0 {
				g.moveWithCollision(&a.pos, push.Scale(-1), a.def.Radius)
			}
			if b.def.Speed > 0 {
				g.moveWithCollision(&b.pos, push, b.def.Radius)
			}
		}
	}
}

func (g *Game) explode(at Vec, radius, dmg float64, friendly bool) {
	g.spawnParticles(at, 22, 0.5, 208, '#')
	// Shockwave ring: an expanding circle of sparks is what makes a blast read
	// as a blast instead of a puff of confetti.
	for i := 0; i < 26; i++ {
		a := float64(i) / 26 * math.Pi * 2
		dv := Vec{1, 0}.rotate(a)
		c := int16(214)
		if i%2 == 0 {
			c = 208
		}
		g.parts = append(g.parts, Particle{
			pos:  at.Add(dv.unvisual().Scale(radius * 0.35)),
			vel:  dv.unvisual().Scale(radius * 3),
			life: 0.28, max: 0.28,
			glyph: '*', color: c,
		})
	}
	g.shake = math.Max(g.shake, 0.22)
	for i := range g.enemies {
		e := &g.enemies[i]
		d := vdist(at, e.pos)
		if d < radius {
			f := 1 - d/radius
			source := damageSecondary
			if friendly {
				source = damageWeapon
			}
			g.hurtEnemyFrom(e, dmg*f, e.pos.Sub(at).visual().norm().Scale(4), source)
		}
	}
	// Rockets and bomber blasts both hurt the player.
	if d := vdist(at, g.player.pos); d < radius && g.player.invuln <= 0 {
		f := 1 - d/radius
		mul := 1.0
		if friendly {
			mul = 0.35 // your own splash stings, but less
		}
		g.damagePlayer(dmg*f*mul, g.player.pos.Sub(at).visual().norm())
	}
	// Barrels caught in the blast light their own fuses, which is what makes a
	// row of them chain instead of all going off at the same instant.
	for i := range g.barrels {
		b := &g.barrels[i]
		if b.lit || b.spent {
			continue
		}
		if d := vdist(at, b.pos); d < radius {
			g.hurtBarrel(b, dmg*(1-d/radius))
		}
	}
	g.breakWalls(at, radius)
}

// breakWalls opens any cracked wall inside a blast. Only explosives can do it,
// which is the launcher's second job.
func (g *Game) breakWalls(at Vec, radius float64) {
	r := int(radius) + 1
	cx, cy := int(at.X), int(at.Y)
	opened := 0
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if g.level.At(x, y) != TileCracked {
				continue
			}
			if vdist(at, Vec{float64(x) + 0.5, float64(y) + 0.5}) > radius {
				continue
			}
			if g.level.Break(x, y) {
				opened++
				g.spawnParticles(Vec{float64(x) + 0.5, float64(y) + 0.5}, 8, 0.4, 180, '#')
			}
		}
	}
	if opened > 0 {
		g.log("벽이 무너졌다. 새 길이 열렸다.", 180)
	}
}

func (g *Game) damagePlayer(dmg float64, dir Vec) {
	p := &g.player
	if p.invuln > 0 || g.state != StatePlaying {
		// Perfect dodge: a hit that lands inside a young dash was beaten by
		// the player's timing, not by the invulnerability they were owed, so
		// it pays out — bullet time and energy back. Once per chain, or rapidly
		// tapping through an explosion would print the reward in bulk.
		if g.state == StatePlaying &&
			g.elapsed-p.lastDashStart <= perfectDodgeWindow && !p.dodgedThisChain {
			p.dodgedThisChain = true
			g.hitStop = math.Max(g.hitStop, perfectDodgeSlowmo)
			p.dashEnergy = math.Min(p.dashEnergyMax, p.dashEnergy+perfectDodgeEnergy)
			g.addFloater(p.pos.Add(Vec{0, -0.7}), "완벽 회피!", 51)
			g.log("완벽 회피! 대시 게이지가 돌아왔다.", 51)
		}
		return
	}
	p.hp -= dmg * p.damageTaken
	p.invuln = 0.35
	p.vel = p.vel.Add(dir.Scale(6))
	// Remember where the hit came from so the screen can point at it: in a
	// crowd the flash alone does not say which side to dash away from.
	if dir.len() > 0.01 {
		g.hitFrom = dir.Scale(-1).norm()
		g.hitFromAge = 0
	}
	g.flash = 0.18
	g.shake = math.Max(g.shake, 0.18)
	g.spawnParticles(p.pos, 6, 0.3, 196, '`')
	if p.hp <= 0 {
		p.hp = 0
		g.log("당신은 쓰러졌다...", 196)
	}
}

// Health is the resource a run is played against, so every source of it is
// deliberately small enough that a fight cannot pay for itself. A medkit is a
// fifth of a fresh bar, not a third of one.
const (
	medkitHeal       = 20.0
	eliteMedkitDrop  = 0.30
	normalMedkitDrop = 0.08
)

// addPickup drops loot, corrected onto floor the same way spawns are. Several
// drop points are an offset from a corpse or a room centre, and loot inside a
// wall is loot the player can see and never reach.
func (g *Game) addPickup(p Pickup) {
	p.pos = g.level.NearestFree(p.pos)
	g.pickups = append(g.pickups, p)
}

func (g *Game) heal(amount float64) {
	p := &g.player
	p.hp = math.Min(p.maxHP, p.hp+amount)
}

func (g *Game) updatePickups(dt float64) {
	out := g.pickups[:0]
	for i := range g.pickups {
		pk := g.pickups[i]
		pk.bob += dt
		if vdist(pk.pos, g.player.pos) < 1.1 {
			switch pk.kind {
			case PickHealth:
				g.heal(medkitHeal)
				g.log(fmt.Sprintf("의료 키트 +%.0f HP", medkitHeal), 46)
			case PickWeapon:
				w := &weapons[pk.weapon]
				g.player.owned[pk.weapon] = true
				g.giveAmmo(pk.weapon, w.PickupAmmo*2)
				g.log(fmt.Sprintf("무기 습득: %s ([%d]번, 탄약 %d)",
					w.Name, pk.weapon+1, g.player.ammo[pk.weapon]), w.Color)
			case PickAmmo:
				w := &weapons[pk.weapon]
				got := g.giveAmmo(pk.weapon, w.PickupAmmo)
				g.log(fmt.Sprintf("%s 탄약 +%d (%d/%d)",
					w.Name, got, g.player.ammo[pk.weapon], w.MaxAmmo), w.Color)
			case PickHeart:
				g.player.maxHP += 20
				g.heal(20)
				g.log("최대 체력 +20", 199)
			}
			continue
		}
		// Loot drifts to a player who is close but not collecting yet: the
		// reward for winning the fight should walk the last bit on its own.
		if d := vdist(pk.pos, g.player.pos); d < pickupMagnetRange*g.player.magnetMul {
			pull := g.player.pos.Sub(pk.pos).visual().norm()
			nx := pk.pos.Add(pull.unvisual().Scale(pickupMagnetSpeed * dt))
			if !g.level.SolidAtPoint(nx) {
				pk.pos = nx
			}
		}
		out = append(out, pk)
	}
	g.pickups = out
}

func (g *Game) spawnParticles(at Vec, n int, life float64, color int16, glyph rune) {
	for i := 0; i < n; i++ {
		a := g.fxRNG.Float64() * math.Pi * 2
		sp := 2 + g.fxRNG.Float64()*8
		l := life * (0.5 + g.fxRNG.Float64())
		g.parts = append(g.parts, Particle{
			pos:   at,
			vel:   Vec{1, 0}.rotate(a).unvisual().Scale(sp),
			life:  l,
			max:   l,
			glyph: glyph,
			color: color,
		})
	}
}

func (g *Game) updateParticles(dt float64) {
	out := g.parts[:0]
	for i := range g.parts {
		p := g.parts[i]
		p.life -= dt
		if p.life <= 0 {
			continue
		}
		p.pos = p.pos.Add(p.vel.Scale(dt))
		p.vel = p.vel.Scale(math.Max(0, 1-4*dt))
		out = append(out, p)
	}
	g.parts = out
}

// ---- progression ------------------------------------------------------------

func (g *Game) gainXP(n int) {
	p := &g.player
	p.xp += n
	for p.xp >= p.xpNext {
		p.xp -= p.xpNext
		p.level++
		p.xpNext = int(float64(p.xpNext) * 1.35)
		g.offerPerks()
	}
}

var allPerks = []Perk{
	{"화력 증강", "총알 피해량 +25%", func(p *Player) { p.damageMul *= 1.25 }},
	{"속사", "연사 속도 +22%", func(p *Player) { p.fireMul *= 1.22 }},
	{"경량 부츠", "이동 속도 +15%", func(p *Player) { p.speedMul *= 1.15 }},
	{"강화 골격", "최대 체력 +20, 즉시 회복", func(p *Player) { p.maxHP += 20; p.hp += 20 }},
	{"관통탄", "총알이 적 1명을 더 관통", func(p *Player) { p.pierce++ }},
	{"이중 사격", "발사체 +1", func(p *Player) { p.extraShots++ }},
	{"흡혈 회로", "준 피해의 2%를 회복 (회복 예산 적용)", func(p *Player) { p.lifesteal += 0.02 }},
	{"단축 회로", "대시 에너지 충전 +30%", func(p *Player) { p.dashRegenMul *= 1.3 }},
	// Deliberately not a heal. Survivability perks that hand back health make
	// health stop being the resource a run is played against; this one makes
	// the health you have last longer instead of replacing it.
	{"방탄 조끼", "받는 피해 -12%", func(p *Player) { p.damageTaken *= 0.88 }},
	// The dash-themed picks: they deepen the answer to "when do I blink"
	// rather than adding a passive number.
	{"제트 부스트", "대시 에너지 최대치 +25", func(p *Player) {
		p.dashEnergyMax += 25
		p.dashEnergy = math.Min(p.dashEnergyMax, p.dashEnergy+25)
	}},
	{"강화 건틀릿", "근접과 대시 충격 피해 +40%", func(p *Player) { p.meleeMul *= 1.4 }},
	{"자기장", "떨어진 아이템이 더 멀리서 끌려온다", func(p *Player) { p.magnetMul *= 1.6 }},
}

func (g *Game) offerPerks() {
	idx := g.rng.Perm(len(allPerks))
	g.perkChoices = nil
	for i := 0; i < 3 && i < len(idx); i++ {
		g.perkChoices = append(g.perkChoices, allPerks[idx[i]])
	}
	// A boss core is the rarer modal and stays on top. Its selection resumes
	// this pending perk screen, even if another enemy died later in the frame.
	if g.state != StateWeaponCore {
		g.state = StateLevelUp
	}
	g.log(fmt.Sprintf("레벨 %d 달성!", g.player.level), 213)
}

func (g *Game) log(text string, color int16) {
	g.msgs = append(g.msgs, Message{text: text, color: color})
	if len(g.msgs) > 30 {
		g.msgs = g.msgs[len(g.msgs)-30:]
	}
}
