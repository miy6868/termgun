package main

import (
	"strings"
	"testing"
)

func roomsOfKind(l *Level, k RoomKind) []int {
	var out []int
	for i, kind := range l.roomKind {
		if kind == k {
			out = append(out, i)
		}
	}
	return out
}

// TestFloorsHaveSpecialRooms is the point of the feature: a floor has to give
// the player somewhere worth choosing to go.
func TestFloorsHaveSpecialRooms(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		g := NewGame(seed)
		g.enterDepth(3)
		if len(roomsOfKind(g.level, RoomTreasure)) == 0 {
			t.Errorf("seed %d B3 has no treasure room", seed)
		}
		if g.ambushRoom < 0 {
			t.Errorf("seed %d B3 has no ambush room", seed)
		}
		if g.shrine == nil {
			t.Errorf("seed %d B3 has no shrine", seed)
		}
	}
}

// TestStartAndStairsRoomsStayOrdinary keeps a run from being sealed in at the
// entrance, or from an ambush sitting on top of the exit.
func TestStartAndStairsRoomsStayOrdinary(t *testing.T) {
	for seed := int64(1); seed <= 30; seed++ {
		g := NewGame(seed)
		for depth := 2; depth <= 4; depth++ {
			g.enterDepth(depth)
			if g.level.roomKind[0] != RoomNormal {
				t.Fatalf("seed %d B%d: the spawn room is special (%v)",
					seed, depth, g.level.roomKind[0])
			}
			if sr := g.level.RoomAt(g.stairsPos); sr >= 0 && g.level.roomKind[sr] != RoomNormal {
				t.Fatalf("seed %d B%d: the stairs room is special (%v)",
					seed, depth, g.level.roomKind[sr])
			}
			if sr := g.level.RoomAt(g.stairsPos); sr >= 0 && sr == g.ambushRoom {
				t.Fatalf("seed %d B%d: the ambush room holds the stairs", seed, depth)
			}
		}
	}
}

// TestTreasureRoomIsGuarded checks the reward has a price.
func TestTreasureRoomIsGuarded(t *testing.T) {
	g := NewGame(4)
	g.enterDepth(3)
	idx := roomsOfKind(g.level, RoomTreasure)
	if len(idx) == 0 {
		t.Fatal("no treasure room")
	}
	room := g.level.rooms[idx[0]]

	loot, guards, elites := 0, 0, 0
	for _, pk := range g.pickups {
		if room.contains(pk.pos) {
			loot++
		}
	}
	for i := range g.enemies {
		if room.contains(g.enemies[i].pos) {
			guards++
			if g.enemies[i].elite != EliteNone {
				elites++
			}
		}
	}
	if loot < 2 {
		t.Errorf("the treasure room holds %d pickups; it has to be worth the trip", loot)
	}
	if guards == 0 {
		t.Error("the treasure room is unguarded, so there is no decision to make")
	}
	if elites == 0 {
		t.Error("the treasure room has no elite guard")
	}
}

// TestAmbushSealsAndReleases walks the whole lifecycle: doors shut on entry,
// they stay shut while anything is alive, and they open again when it is over.
func TestAmbushSealsAndReleases(t *testing.T) {
	g := NewGame(11)
	g.enterDepth(3)
	if g.ambushRoom < 0 {
		t.Skip("no ambush room on this floor")
	}
	room := g.level.rooms[g.ambushRoom]
	openings := g.level.Openings(room)
	if len(openings) == 0 {
		t.Skip("ambush room has no openings to seal")
	}

	cx, cy := room.center()
	g.player.pos = Vec{float64(cx) + 0.5, float64(cy) + 0.5}
	g.Update(1.0 / 60)

	if !g.ambushOn {
		t.Fatal("walking into the ambush room did not trigger it")
	}
	shut := 0
	for _, p := range openings {
		if g.level.At(p[0], p[1]) == TileDoor {
			shut++
		}
	}
	if shut != len(openings) {
		t.Errorf("%d of %d openings were sealed; a gap makes the lockdown pointless",
			shut, len(openings))
	}
	if len(g.enemies) == 0 {
		t.Error("the ambush sealed the room but sent nothing")
	}

	// Doors must hold while the room is occupied.
	g.Update(1.0 / 60)
	if !g.ambushOn {
		t.Fatal("the ambush ended while enemies were still alive")
	}

	// Clear every wave. Levelling up pauses the simulation, so the perk menu
	// has to be answered the way a player would.
	for wave := 0; wave < ambushWaves+3 && g.ambushOn; wave++ {
		for i := range g.enemies {
			g.enemies[i].hp = 0
		}
		g.reapEnemies()
		for g.state == StateLevelUp {
			g.handleKey(Event{Kind: EvKey, Rune: '1'})
		}
		g.Update(1.0 / 60)
	}
	if g.ambushOn {
		t.Fatal("the ambush never finished after clearing every wave")
	}
	for _, p := range openings {
		if g.level.At(p[0], p[1]) == TileDoor {
			t.Fatalf("door at %v stayed shut after the ambush; the player is trapped", p)
		}
	}
	if !g.ambushDone {
		t.Error("the ambush can retrigger")
	}
}

// TestAmbushDoorsBlockMovement checks a shut door is actually solid — the
// lockdown is only real if a body cannot walk through it.
func TestAmbushDoorsBlockMovement(t *testing.T) {
	if !TileDoor.solid() {
		t.Fatal("a shut door is not solid")
	}
	g := NewGame(11)
	g.enterDepth(3)
	if g.ambushRoom < 0 {
		t.Skip("no ambush room")
	}
	room := g.level.rooms[g.ambushRoom]
	if len(g.level.Openings(room)) == 0 {
		t.Skip("no openings")
	}
	cx, cy := room.center()
	g.player.pos = Vec{float64(cx) + 0.5, float64(cy) + 0.5}
	g.Update(1.0 / 60)

	// Try to shove the player straight out through every sealed opening.
	for _, d := range g.ambushDoors {
		target := Vec{float64(d[0]) + 0.5, float64(d[1]) + 0.5}
		g.player.pos = Vec{float64(cx) + 0.5, float64(cy) + 0.5}
		for i := 0; i < 400; i++ {
			step := target.Sub(g.player.pos).norm().Scale(0.05)
			g.moveWithCollision(&g.player.pos, step, g.player.radius)
		}
		if g.level.At(int(g.player.pos.X), int(g.player.pos.Y)) == TileDoor {
			t.Fatalf("the player walked into a shut door at %v", d)
		}
	}
}

// TestAmbushNeverEatsTheStairs is the trap this could set: rooms can sit close
// enough that the wall ring of one clips another's floor, and sealing a stairs
// tile would replace the exit with a door and restore it as blank floor —
// leaving a run with no way down and no way to tell why.
func TestAmbushNeverEatsTheStairs(t *testing.T) {
	for seed := int64(1); seed <= 60; seed++ {
		g := NewGame(seed)
		for depth := 2; depth <= 5; depth++ {
			g.enterDepth(depth)
			if g.ambushRoom < 0 {
				continue
			}
			room := g.level.rooms[g.ambushRoom]
			for _, p := range g.level.Openings(room) {
				if g.level.At(p[0], p[1]) == TileStairs {
					t.Fatalf("seed %d B%d: the ambush would seal the stairs at %v",
						seed, depth, p)
				}
			}
			// And after a full seal/release cycle the stairs must still be there.
			sx, sy := int(g.stairsPos.X), int(g.stairsPos.Y)
			g.startAmbush(room)
			g.endAmbush()
			if g.level.At(sx, sy) != TileStairs {
				t.Fatalf("seed %d B%d: the stairs were destroyed by an ambush cycle",
					seed, depth)
			}
		}
	}
}

// TestShrineIsATradeNotAnUpgrade is the design requirement. Every perk in the
// game is a straight gain; the shrine is the one place with a cost, and a
// shrine that only gives is just another perk.
func TestShrineIsATradeNotAnUpgrade(t *testing.T) {
	for i, off := range shrineOffers {
		p := newPlayer()
		before := p
		off.Apply(&p)

		better, worse := 0, 0
		type stat struct {
			name       string
			b, a       float64
			higherGood bool
		}
		for _, s := range []stat{
			{"damageMul", before.damageMul, p.damageMul, true},
			{"fireMul", before.fireMul, p.fireMul, true},
			{"speedMul", before.speedMul, p.speedMul, true},
			{"maxHP", before.maxHP, p.maxHP, true},
			{"damageTaken", before.damageTaken, p.damageTaken, false},
		} {
			if s.a == s.b {
				continue
			}
			gain := s.a > s.b
			if !s.higherGood {
				gain = !gain
			}
			if gain {
				better++
			} else {
				worse++
			}
		}
		if better == 0 {
			t.Errorf("offer %d (%s) has no upside", i, off.Desc)
		}
		if worse == 0 {
			t.Errorf("offer %d (%s) has no cost; it is a perk, not a trade", i, off.Desc)
		}
		if p.hp > p.maxHP {
			t.Errorf("offer %d left hp %.1f above maxHP %.1f", i, p.hp, p.maxHP)
		}
	}
}

// TestShrineUsesEAndOnlyOnce covers the interaction.
func TestShrineUsesEAndOnlyOnce(t *testing.T) {
	g := NewGame(7)
	g.enterDepth(3)
	if g.shrine == nil {
		t.Skip("no shrine")
	}
	g.player.pos = g.shrine.pos
	before := g.player.damageMul + g.player.speedMul + g.player.fireMul + g.player.maxHP

	g.handleKey(Event{Kind: EvKey, Rune: 'e'})
	if !g.shrine.used {
		t.Fatal("[E] next to a shrine did nothing")
	}
	after := g.player.damageMul + g.player.speedMul + g.player.fireMul + g.player.maxHP
	if after == before {
		t.Error("using the shrine changed nothing about the player")
	}

	g.handleKey(Event{Kind: EvKey, Rune: 'e'})
	if again := g.player.damageMul + g.player.speedMul + g.player.fireMul + g.player.maxHP; again != after {
		t.Error("the shrine can be used more than once")
	}
}

// TestShrineOfferIsReadableBeforeCommitting is the discoverability rule: the
// trade has to be on screen before the player presses the key.
func TestShrineOfferIsReadableBeforeCommitting(t *testing.T) {
	g := NewGame(7)
	g.enterDepth(3)
	if g.shrine == nil {
		t.Skip("no shrine")
	}
	g.player.pos = g.shrine.pos
	s := newTestScreen(120, 40)
	g.Draw(s)

	joined := ""
	for y := 0; y < s.H; y++ {
		joined += screenRow(s, y) + "\n"
	}
	if !strings.Contains(joined, "[E]") {
		t.Error("standing at a shrine shows no prompt")
	}
	if !strings.Contains(joined, g.shrine.offer.Desc) {
		t.Errorf("the shrine's terms (%q) are not on screen before committing",
			g.shrine.offer.Desc)
	}
}

// ---- descent pressure -------------------------------------------------------

// TestFloorBonusDecays is the whole point: staying has to cost something, or
// "clear the floor first" is always right and there is no decision.
func TestFloorBonusDecays(t *testing.T) {
	g := NewGame(3)
	if m := g.descentMul(); m < 1 {
		t.Errorf("the bonus is already reduced on arrival (x%.2f)", m)
	}
	g.floorTime = bonusDecayTime / 2
	half := g.descentMul()
	if half >= 1 {
		t.Errorf("the bonus did not decay after half the window (x%.2f)", half)
	}
	g.floorTime = bonusDecayTime * 10
	if end := g.descentMul(); end != minBonusMul {
		t.Errorf("the bonus bottomed out at x%.2f, want x%.2f", end, minBonusMul)
	}
	if minBonusMul <= 0 {
		t.Error("the bonus reaches zero, which makes the stairs worthless rather than urgent")
	}
}

// TestDescendingPaysTheDecayedBonus checks the decay reaches the score.
func TestDescendingPaysTheDecayedBonus(t *testing.T) {
	score := func(wait float64) int {
		g := NewGame(3)
		g.player.pos = g.stairsPos
		g.floorTime = wait
		before := g.score
		g.tryStairs()
		return g.score - before
	}
	fast, slow := score(0), score(bonusDecayTime*2)
	if slow >= fast {
		t.Errorf("descending immediately paid %d and dawdling paid %d; "+
			"they must not be the same", fast, slow)
	}
}

// TestReinforcementsArriveAndAreAnnounced covers the other half of the pressure
// and the warning that makes it fair.
func TestReinforcementsArriveAndAreAnnounced(t *testing.T) {
	g := NewGame(8)
	g.enemies = g.enemies[:0]

	// Nothing should arrive early.
	g.floorTime = reinforceAfter - 20
	g.updatePressure(1.0 / 60)
	if len(g.enemies) > 0 {
		t.Fatal("reinforcements arrived before the timer was up")
	}

	// A warning has to land before the first wave.
	g.floorTime = reinforceAfter - reinforceWarn + 0.1
	g.updatePressure(1.0 / 60)
	warned := false
	for _, m := range g.msgs {
		if strings.Contains(m.text, "올라오고") {
			warned = true
		}
	}
	if !warned {
		t.Error("reinforcements were never announced in advance")
	}

	g.floorTime = reinforceAfter + 1
	g.reinfTimer = 0
	g.updatePressure(1.0 / 60)
	if len(g.enemies) == 0 {
		t.Fatal("no reinforcements arrived after the timer expired")
	}
	// And they must not land on top of the player.
	for i := range g.enemies {
		if d := vdist(g.enemies[i].pos, g.player.pos); d < 20 {
			t.Errorf("a reinforcement spawned %.1f from the player; that is a gotcha, "+
				"not pressure", d)
		}
	}
}

// TestPressureIsOnScreen keeps the mechanic visible at every supported width.
func TestPressureIsOnScreen(t *testing.T) {
	for _, w := range []int{100, 120, 160} {
		g := NewGame(3)
		g.floorTime = reinforceAfter * 0.9
		s := newTestScreen(w, 30)
		g.Draw(s)
		row := screenRow(s, 0)
		if !strings.Contains(row, "층 x") {
			t.Errorf("width %d: no floor-bonus readout on the status row: %q", w, row)
		}
	}
}

// TestFloorResetsPressure makes sure the clock is per floor.
func TestFloorResetsPressure(t *testing.T) {
	g := NewGame(3)
	g.floorTime = bonusDecayTime
	g.reinfWaves = 4
	g.enterDepth(g.depth + 1)
	if g.floorTime != 0 || g.reinfWaves != 0 {
		t.Errorf("a new floor kept the old pressure (time %.1f, waves %d)",
			g.floorTime, g.reinfWaves)
	}
	if g.descentMul() != 1 {
		t.Error("a new floor does not start at the full bonus")
	}
}
