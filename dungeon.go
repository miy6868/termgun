package main

import (
	"math"
	"math/rand"
)

type Tile uint8

const (
	TileWall Tile = iota
	TileFloor
	TileStairs
	TileRubble  // cosmetic floor variant
	TileDoor    // shut during an ambush, ordinary floor otherwise
	TileCracked // a wall that explosives can open into a new route
	TileAcid    // burns anything standing in it, the player included
)

// tileSolid answers solid() with one load instead of a chain of comparisons.
// The flow-field search asks up to 24 times per tile it visits and rebuilds
// most frames, which made this the hottest predicate in the simulation. It is
// sized to the full range of Tile so indexing needs no bounds check; add new
// solid tile kinds here and to plainFloor.
var tileSolid = [256]bool{TileWall: true, TileDoor: true, TileCracked: true}

func (t Tile) solid() bool { return tileSolid[t] }

// walkable floor variants that a hazard or a door may replace.
func (t Tile) plainFloor() bool { return t == TileFloor || t == TileRubble }

// The exit goes in a random room from the floor's outer band: at least this
// share of the longest possible walk away. A fixed farthest corner turns every
// descent into the same beeline, but the band still rules out short cuts —
// finding the stairs stays a search across the whole floor, not a sprint.
const stairsFarMinShare = 0.6

// How many random tiles inside the chosen room the exit may try before
// falling back to the room's center. Off-center stairs are not visible down
// the entry corridor, so spotting the room is not the same as spotting them.
const stairsSpotTries = 8

type Rect struct{ X, Y, W, H int }

func (r Rect) center() (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

// contains reports whether a world point lies inside the room.
func (r Rect) contains(p Vec) bool {
	x, y := int(p.X), int(p.Y)
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// RoomKind gives a room a reason to exist. Without it a floor is a set of
// interchangeable boxes and there is never a reason to pick one over another.
type RoomKind uint8

const (
	RoomNormal RoomKind = iota
	RoomTreasure
	RoomAmbush
	RoomShrine
)

type Level struct {
	W, H     int
	Depth    int
	tiles    []Tile
	visible  []bool
	explored []bool
	rooms    []Rect
	roomKind []RoomKind

	// flow is a Dijkstra distance map towards the player, rebuilt a few times
	// per second and used by enemies to path around corners.
	flow    []int32
	flowSrc [2]int

	// Scratch for BuildFlow. The player changes tile constantly, so the search
	// runs most frames; keeping the queue and the pre-filled reset row here
	// turns two per-frame allocations and a store loop into one memmove.
	flowQueue []int32
	flowReset []int32

	// fovSrc is the tile the current visibility set was cast from, so a frame
	// that did not change tile reuses it. Invalidated wherever terrain moves.
	fovSrc [3]int
}

func (l *Level) idx(x, y int) int { return y*l.W + x }

func (l *Level) inBounds(x, y int) bool { return x >= 0 && y >= 0 && x < l.W && y < l.H }

func (l *Level) At(x, y int) Tile {
	if !l.inBounds(x, y) {
		return TileWall
	}
	return l.tiles[l.idx(x, y)]
}

func (l *Level) Solid(x, y int) bool { return l.At(x, y).solid() }

// SolidAtPoint tests a world-space point against the tile grid.
func (l *Level) SolidAtPoint(p Vec) bool { return l.Solid(int(p.X), int(p.Y)) }

func (l *Level) Visible(x, y int) bool {
	return l.inBounds(x, y) && l.visible[l.idx(x, y)]
}

func (l *Level) Explored(x, y int) bool {
	return l.inBounds(x, y) && l.explored[l.idx(x, y)]
}

// ---- generation -------------------------------------------------------------

// GenerateLevel builds a BSP dungeon: the map is recursively split, one room is
// carved per leaf, and sibling rooms are joined with L-shaped corridors. Deeper
// floors get larger and more subdivided.
func GenerateLevel(depth int, rng *rand.Rand) *Level {
	w := clamp(64+depth*4, 64, 110)
	h := clamp(38+depth*2, 38, 60)

	l := &Level{W: w, H: h, Depth: depth}
	l.tiles = make([]Tile, w*h)
	l.visible = make([]bool, w*h)
	l.explored = make([]bool, w*h)
	l.flow = make([]int32, w*h)
	l.flowQueue = make([]int32, 0, w*h)
	l.flowReset = make([]int32, w*h)
	for i := range l.flowReset {
		l.flowReset[i] = flowUnreachable
	}
	l.invalidateTerrain()

	minLeaf := 11
	leaves := []Rect{{1, 1, w - 2, h - 2}}
	splits := clamp(4+depth/2, 4, 7)
	for i := 0; i < splits; i++ {
		var next []Rect
		for _, r := range leaves {
			a, b, ok := splitRect(r, minLeaf, rng)
			if ok {
				next = append(next, a, b)
			} else {
				next = append(next, r)
			}
		}
		leaves = next
	}

	for _, leaf := range leaves {
		rw := rng.Intn(maxInt(3, leaf.W-4)) + 5
		rh := rng.Intn(maxInt(2, leaf.H-4)) + 4
		rw = minInt(rw, leaf.W-2)
		rh = minInt(rh, leaf.H-2)
		if rw < 4 || rh < 3 {
			continue
		}
		room := Rect{
			X: leaf.X + rng.Intn(leaf.W-rw+1),
			Y: leaf.Y + rng.Intn(leaf.H-rh+1),
			W: rw, H: rh,
		}
		l.carveRoom(room)
		l.rooms = append(l.rooms, room)
	}

	// Connect every room to the previous one, plus a couple of extra loops so
	// the floor is not a pure tree (loops make combat retreats possible).
	for i := 1; i < len(l.rooms); i++ {
		ax, ay := l.rooms[i-1].center()
		bx, by := l.rooms[i].center()
		l.carveCorridor(ax, ay, bx, by, rng)
	}
	for i := 0; i < 2+depth/3 && len(l.rooms) > 2; i++ {
		a := l.rooms[rng.Intn(len(l.rooms))]
		b := l.rooms[rng.Intn(len(l.rooms))]
		ax, ay := a.center()
		bx, by := b.center()
		l.carveCorridor(ax, ay, bx, by, rng)
	}

	// Rotate the room list so the entrance lands in a random room. The BSP
	// otherwise starts every floor in the same corner, which pins the exit
	// diagonally opposite and turns "find the stairs" into one memorized walk.
	if n := len(l.rooms); n > 1 {
		rot := rng.Intn(n)
		rooms := make([]Rect, 0, n)
		rooms = append(rooms, l.rooms[rot:]...)
		rooms = append(rooms, l.rooms[:rot]...)
		l.rooms = rooms
	}

	l.roomKind = make([]RoomKind, len(l.rooms))

	// Scatter a little rubble for visual texture.
	for i := 0; i < w*h/60; i++ {
		x, y := rng.Intn(w), rng.Intn(h)
		if l.At(x, y) == TileFloor {
			l.tiles[l.idx(x, y)] = TileRubble
		}
	}
	l.placeCrackedWalls(depth, rng)
	l.placeAcid(depth, rng)
	return l
}

// placeCrackedWalls picks walls that separate two open areas, so blowing one
// open is a shortcut rather than a hole into solid rock. This is what gives the
// launcher a job other than damage.
func (l *Level) placeCrackedWalls(depth int, rng *rand.Rand) {
	var candidates [][2]int
	for y := 2; y < l.H-2; y++ {
		for x := 2; x < l.W-2; x++ {
			if l.At(x, y) != TileWall {
				continue
			}
			// Open on opposite sides: breaking it joins two places.
			horiz := !l.Solid(x-1, y) && !l.Solid(x+1, y)
			vert := !l.Solid(x, y-1) && !l.Solid(x, y+1)
			if horiz != vert { // exactly one axis, so it is a thin divider
				candidates = append(candidates, [2]int{x, y})
			}
		}
	}
	rng.Shuffle(len(candidates), func(a, b int) {
		candidates[a], candidates[b] = candidates[b], candidates[a]
	})
	n := minInt(3+depth/2, len(candidates))
	for i := 0; i < n; i++ {
		c := candidates[i]
		l.tiles[l.idx(c[0], c[1])] = TileCracked
	}
}

// placeAcid drops a few small pools. They are deliberately not treated as
// obstacles by the pathfinder: enemies walk straight through, so the player can
// fight over one on purpose.
func (l *Level) placeAcid(depth int, rng *rand.Rand) {
	if depth < 2 || len(l.rooms) == 0 {
		return
	}
	// The player always arrives at the centre of the first room, so keep a
	// clear landing area: starting a floor already standing in acid is damage
	// with no decision attached and no warning.
	spawnX, spawnY := l.rooms[0].center()
	const spawnClear = 4

	pools := 1 + depth/3
	for i := 0; i < pools; i++ {
		r := l.rooms[rng.Intn(len(l.rooms))]
		if r.W < 5 || r.H < 4 {
			continue
		}
		cx := r.X + 1 + rng.Intn(r.W-2)
		cy := r.Y + 1 + rng.Intn(r.H-2)
		for dy := -1; dy <= 1; dy++ {
			for dx := -2; dx <= 2; dx++ {
				x, y := cx+dx, cy+dy
				// A ragged blob rather than a rectangle.
				if absInt(dx)+absInt(dy)*2 > 2+rng.Intn(2) {
					continue
				}
				if absInt(x-spawnX) <= spawnClear && absInt(y-spawnY) <= spawnClear {
					continue
				}
				if l.inBounds(x, y) && l.At(x, y).plainFloor() {
					l.tiles[l.idx(x, y)] = TileAcid
				}
			}
		}
	}
}

// Break turns a cracked wall into rubble and reports whether anything changed.
func (l *Level) Break(x, y int) bool {
	if !l.inBounds(x, y) || l.At(x, y) != TileCracked {
		return false
	}
	l.tiles[l.idx(x, y)] = TileRubble
	// The map just changed shape; any cached route or sightline through it is
	// stale.
	l.invalidateTerrain()
	return true
}

func splitRect(r Rect, min int, rng *rand.Rand) (Rect, Rect, bool) {
	horizontal := rng.Intn(2) == 0
	if r.W > r.H*2 {
		horizontal = false
	} else if r.H > r.W*2 {
		horizontal = true
	}
	if horizontal {
		if r.H < min*2 {
			return r, r, false
		}
		cut := min + rng.Intn(r.H-min*2+1)
		return Rect{r.X, r.Y, r.W, cut}, Rect{r.X, r.Y + cut, r.W, r.H - cut}, true
	}
	if r.W < min*2 {
		return r, r, false
	}
	cut := min + rng.Intn(r.W-min*2+1)
	return Rect{r.X, r.Y, cut, r.H}, Rect{r.X + cut, r.Y, r.W - cut, r.H}, true
}

func (l *Level) carveRoom(r Rect) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if l.inBounds(x, y) {
				l.tiles[l.idx(x, y)] = TileFloor
			}
		}
	}
}

func (l *Level) carveCorridor(ax, ay, bx, by int, rng *rand.Rand) {
	// Corridors are two tiles tall so that dodging inside them stays possible.
	if rng.Intn(2) == 0 {
		l.carveH(ax, bx, ay)
		l.carveV(ay, by, bx)
	} else {
		l.carveV(ay, by, ax)
		l.carveH(ax, bx, by)
	}
}

func (l *Level) carveH(x1, x2, y int) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	for x := x1; x <= x2; x++ {
		for dy := 0; dy <= 1; dy++ {
			if l.inBounds(x, y+dy) && y+dy < l.H-1 {
				l.tiles[l.idx(x, y+dy)] = TileFloor
			}
		}
	}
}

func (l *Level) carveV(y1, y2, x int) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	for y := y1; y <= y2; y++ {
		for dx := 0; dx <= 1; dx++ {
			if l.inBounds(x+dx, y) && x+dx < l.W-1 {
				l.tiles[l.idx(x+dx, y)] = TileFloor
			}
		}
	}
}

// PlaceStairs puts the exit in a random room from the outer band of the floor
// — far from the entrance, but not always in the same corner — and inside that
// room on a random floor tile rather than always the center, which would put
// it in the line of sight of the doorway.
func (l *Level) PlaceStairs(fromX, fromY int, rng *rand.Rand) (int, int) {
	dists := make([]int, len(l.rooms))
	maxD := 0
	for i, r := range l.rooms {
		cx, cy := r.center()
		dists[i] = absInt(cx-fromX) + absInt(cy-fromY)
		if dists[i] > maxD {
			maxD = dists[i]
		}
	}
	minD := int(math.Ceil(float64(maxD) * stairsFarMinShare))
	var pool []int
	for i, d := range dists {
		if d >= minD {
			pool = append(pool, i)
		}
	}
	if len(pool) == 0 {
		return fromX, fromY
	}

	r := l.rooms[pool[rng.Intn(len(pool))]]
	sx, sy := r.center()
	for try := 0; try < stairsSpotTries; try++ {
		x := r.X + rng.Intn(r.W)
		y := r.Y + rng.Intn(r.H)
		if l.At(x, y).plainFloor() {
			sx, sy = x, y
			break
		}
	}
	l.tiles[l.idx(sx, sy)] = TileStairs
	return sx, sy
}

// RoomAt returns the index of the room containing a point, or -1.
func (l *Level) RoomAt(p Vec) int {
	for i, r := range l.rooms {
		if r.contains(p) {
			return i
		}
	}
	return -1
}

// Openings lists the floor tiles in the wall ring around a room — the places a
// corridor punches through, and therefore the tiles a door has to fill to seal
// it. Corners are skipped: they are never doorways and sealing them can wall
// off a corridor that merely runs past.
func (l *Level) Openings(r Rect) [][2]int {
	var out [][2]int
	add := func(x, y int) {
		if !l.inBounds(x, y) {
			return
		}
		// Only plain floor becomes a door. Rooms can sit close enough that the
		// ring around one clips the stairs, and sealing those would replace the
		// exit with a door and then restore it as blank floor — a floor with no
		// way down.
		if l.At(x, y).plainFloor() {
			out = append(out, [2]int{x, y})
		}
	}
	for x := r.X; x < r.X+r.W; x++ {
		add(x, r.Y-1)
		add(x, r.Y+r.H)
	}
	for y := r.Y; y < r.Y+r.H; y++ {
		add(r.X-1, y)
		add(r.X+r.W, y)
	}
	return out
}

// SetTiles paints a fixed tile over a list of positions.
func (l *Level) SetTiles(at [][2]int, t Tile) {
	for _, p := range at {
		if l.inBounds(p[0], p[1]) {
			l.tiles[l.idx(p[0], p[1])] = t
			// A door changes what can be walked through and what can be seen
			// past, so both caches are stale the moment it moves.
			l.invalidateTerrain()
		}
	}
}

// NearestFree pulls a point out of solid rock and onto the closest open tile,
// leaving it alone if it is already clear.
//
// Collision resolution only ever moves a body to a position it has checked, so
// nothing can walk into a wall — but plenty of code picks a spot by arithmetic
// rather than by search: a splitting elite dropping fragments beside itself, a
// boss calling in swarmlings on a ring around it, an ambush scattering a wave
// across a room. Any of those can land in a wall, and a body that starts inside
// one is stuck there permanently, because from there every direction it tries
// is blocked too. A shooter parked in the rock still has line of fire.
func (l *Level) NearestFree(p Vec) Vec {
	if !l.SolidAtPoint(p) {
		return p
	}
	cx, cy := int(p.X), int(p.Y)
	for r := 1; r <= 6; r++ {
		best, bestD := [2]int{}, math.Inf(1)
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if absInt(dx) != r && absInt(dy) != r {
					continue // interior already searched by a smaller ring
				}
				x, y := cx+dx, cy+dy
				if !l.inBounds(x, y) || l.Solid(x, y) {
					continue
				}
				c := Vec{float64(x) + 0.5, float64(y) + 0.5}
				if d := vdist(c, p); d < bestD {
					best, bestD = [2]int{x, y}, d
				}
			}
		}
		if !math.IsInf(bestD, 1) {
			return Vec{float64(best[0]) + 0.5, float64(best[1]) + 0.5}
		}
	}
	return p
}

// FreeSpot returns a random floor tile at least minDist away from (ax, ay).
func (l *Level) FreeSpot(rng *rand.Rand, ax, ay int, minDist float64) (float64, float64) {
	for try := 0; try < 400; try++ {
		r := l.rooms[rng.Intn(len(l.rooms))]
		x := r.X + rng.Intn(r.W)
		y := r.Y + rng.Intn(r.H)
		if l.Solid(x, y) {
			continue
		}
		if vdist(Vec{float64(x), float64(y)}, Vec{float64(ax), float64(ay)}) < minDist {
			continue
		}
		return float64(x) + 0.5, float64(y) + 0.5
	}
	// Fallback: any floor tile at all.
	for y := 1; y < l.H-1; y++ {
		for x := 1; x < l.W-1; x++ {
			if !l.Solid(x, y) {
				return float64(x) + 0.5, float64(y) + 0.5
			}
		}
	}
	return 1.5, 1.5
}

// ---- field of view (recursive shadow casting) -------------------------------

var octants = [8][4]int{
	{1, 0, 0, 1}, {0, 1, 1, 0}, {0, -1, 1, 0}, {-1, 0, 0, 1},
	{-1, 0, 0, -1}, {0, -1, -1, 0}, {0, 1, -1, 0}, {1, 0, 0, -1},
}

// invalidateTerrain drops every cache that was derived from the tile grid.
// Both writers of l.tiles call it; forgetting one leaves enemies pathing
// through a wall that is gone, or a doorway that still blocks sight.
func (l *Level) invalidateTerrain() {
	l.flowSrc = [2]int{-1, -1}
	l.fovSrc = [3]int{-1, -1, -1}
}

// ComputeFOV recalculates which tiles the player can see from (cx, cy).
//
// The result depends only on the tile stood in and the terrain, but the caller
// runs every frame while the player crosses a tile a few times a second, so a
// repeat from the same tile reuses the visibility set rather than re-casting
// all eight octants.
func (l *Level) ComputeFOV(cx, cy, radius int) {
	if l.fovSrc == [3]int{cx, cy, radius} {
		return
	}
	l.fovSrc = [3]int{cx, cy, radius}
	for i := range l.visible {
		l.visible[i] = false
	}
	if !l.inBounds(cx, cy) {
		return
	}
	l.setVisible(cx, cy)
	for _, o := range octants {
		l.castLight(cx, cy, 1, 1.0, 0.0, radius, o[0], o[1], o[2], o[3])
	}
}

func (l *Level) setVisible(x, y int) {
	if !l.inBounds(x, y) {
		return
	}
	l.visible[l.idx(x, y)] = true
	l.explored[l.idx(x, y)] = true
}

func (l *Level) castLight(cx, cy, row int, start, end float64, radius, xx, xy, yx, yy int) {
	if start < end {
		return
	}
	blocked := false
	newStart := start
	for dist := row; dist <= radius && !blocked; dist++ {
		dy := -dist
		for dx := -dist; dx <= 0; dx++ {
			// Aspect correction: the light circle is squashed vertically so it
			// looks round on a character grid.
			mx := cx + dx*xx + dy*xy
			my := cy + dx*yx + dy*yy
			lSlope := (float64(dx) - 0.5) / (float64(dy) + 0.5)
			rSlope := (float64(dx) + 0.5) / (float64(dy) - 0.5)
			if rSlope > start {
				continue
			}
			if lSlope < end {
				break
			}
			// The range test has to be in world coordinates. dx and dy are
			// octant-local — dy is depth along whichever axis this octant
			// runs — so applying the aspect correction to dy squashes the
			// horizontal octants horizontally, leaving a view barely half as
			// wide as it should be instead of an aspect-correct circle.
			wx, wy := mx-cx, my-cy
			if float64(wx*wx)+float64(wy*wy)*aspect*aspect <= float64(radius*radius) {
				l.setVisible(mx, my)
			}
			if blocked {
				if l.Solid(mx, my) {
					newStart = rSlope
					continue
				}
				blocked = false
				start = newStart
			} else if l.Solid(mx, my) && dist < radius {
				blocked = true
				l.castLight(cx, cy, dist+1, start, lSlope, radius, xx, xy, yx, yy)
				newStart = rSlope
			}
		}
	}
}

// LineOfSight is a Bresenham-style walk used for enemy awareness and hitscan.
func (l *Level) LineOfSight(a, b Vec) bool {
	d := b.Sub(a)
	steps := int(d.visual().len() * 2)
	if steps <= 0 {
		return true
	}
	step := d.Scale(1 / float64(steps))
	p := a
	for i := 0; i < steps; i++ {
		p = p.Add(step)
		if l.SolidAtPoint(p) {
			return false
		}
	}
	return true
}

// ---- flow field -------------------------------------------------------------

const flowUnreachable int32 = 1 << 20

// neighbors8 is shared rather than written inline in the search loops: as a
// composite literal inside the loop it is rebuilt on every iteration.
var neighbors8 = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// BuildFlow runs a breadth-first search out from the player so every floor tile
// knows how far it is from them. Enemies then just walk downhill.
func (l *Level) BuildFlow(px, py int) {
	if l.flowSrc[0] == px && l.flowSrc[1] == py {
		return
	}
	l.flowSrc = [2]int{px, py}
	copy(l.flow, l.flowReset)
	if !l.inBounds(px, py) || l.Solid(px, py) {
		return
	}
	w, tiles, flow := l.W, l.tiles, l.flow
	queue := l.flowQueue[:0]
	start := int32(l.idx(px, py))
	flow[start] = 0
	queue = append(queue, start)
	for head := 0; head < len(queue); head++ {
		cur := int(queue[head])
		cx, cy := cur%w, cur/w
		d := flow[cur] + 1
		for k := range neighbors8 {
			o := &neighbors8[k]
			nx, ny := cx+o[0], cy+o[1]
			if !l.inBounds(nx, ny) || tiles[ny*w+nx].solid() {
				continue
			}
			// Do not cut diagonally through wall corners.
			if o[0] != 0 && o[1] != 0 &&
				(tiles[cy*w+cx+o[0]].solid() || tiles[(cy+o[1])*w+cx].solid()) {
				continue
			}
			ni := ny*w + nx
			if flow[ni] > d {
				flow[ni] = d
				queue = append(queue, int32(ni))
			}
		}
	}
	l.flowQueue = queue[:0]
}

// FlowStep returns a unit direction (visual space) leading towards the player,
// or false if this tile has no downhill neighbour.
func (l *Level) FlowStep(x, y int) (Vec, bool) {
	if !l.inBounds(x, y) {
		return Vec{}, false
	}
	cur := l.flow[l.idx(x, y)]
	if cur >= flowUnreachable {
		return Vec{}, false
	}
	best := cur
	var bx, by int
	found := false
	for k := range neighbors8 {
		o := &neighbors8[k]
		nx, ny := x+o[0], y+o[1]
		if !l.inBounds(nx, ny) || l.Solid(nx, ny) {
			continue
		}
		// Do not cut diagonally through a wall corner. BuildFlow already
		// refuses to, and leaving it out here hands the mover a direction its
		// body cannot fit through: it gets blocked, the corner-slip pushes it
		// back, and it oscillates between the two tiles forever.
		if o[0] != 0 && o[1] != 0 && (l.Solid(x+o[0], y) || l.Solid(x, y+o[1])) {
			continue
		}
		if v := l.flow[l.idx(nx, ny)]; v < best {
			best, bx, by, found = v, o[0], o[1], true
		}
	}
	if !found {
		return Vec{}, false
	}
	return Vec{float64(bx), float64(by) * aspect}.norm(), true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
