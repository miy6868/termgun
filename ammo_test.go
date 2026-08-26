package main

import "testing"

func armedGame(t *testing.T, seed int64) *Game {
	t.Helper()
	g := newGameWithInput(seed, InputKitty)
	s := newTestScreen(120, 40)
	g.Draw(s)
	g.enemies = g.enemies[:0]
	g.pickups = g.pickups[:0]
	g.mouseSet = true
	g.aim = g.player.pos.Add(Vec{6, 0})
	return g
}

// fireOnce pulls the trigger a single time, ignoring the cooldown.
func fireOnce(g *Game) {
	g.player.cooldown = 0
	g.firing = true
	g.Update(1.0 / 240)
	g.firing = false
}

// TestFiringSpendsAmmo checks each shot costs exactly one round, whatever the
// weapon puts on screen for it.
func TestFiringSpendsAmmo(t *testing.T) {
	g := armedGame(t, 1)
	g.player.owned[wpShotgun] = true
	g.player.ammo[wpShotgun] = 10
	g.player.weapon = wpShotgun

	fireOnce(g)
	if g.player.ammo[wpShotgun] != 9 {
		t.Fatalf("one shotgun shell should be spent per shot, ammo is %d", g.player.ammo[wpShotgun])
	}
	if len(g.bullets) != weapons[wpShotgun].Pellets {
		t.Fatalf("shotgun fired %d pellets, want %d", len(g.bullets), weapons[wpShotgun].Pellets)
	}
}

func TestAutoSwitchToPistol(t *testing.T) {
	g := armedGame(t, 2)
	p := &g.player
	p.owned[wpShotgun], p.owned[wpSMG], p.owned[wpRailgun] = true, true, true
	p.ammo[wpPistol] = 5
	p.ammo[wpShotgun] = 1
	p.ammo[wpSMG] = 40
	p.ammo[wpRailgun] = 3
	p.weapon = wpShotgun

	fireOnce(g)
	if p.ammo[wpShotgun] != 0 {
		t.Fatalf("shotgun ammo is %d, want 0", p.ammo[wpShotgun])
	}
	if p.weapon != wpPistol {
		t.Fatalf("auto-switched to %s, want pistol", weapons[p.weapon].Name)
	}
}

func TestAutoSwitchCanBeDisabled(t *testing.T) {
	g := armedGame(t, 12)
	p := &g.player
	p.owned[wpSMG] = true
	p.ammo[wpPistol] = 1
	p.ammo[wpSMG] = 40
	p.weapon = wpPistol
	g.autoWeapon = false

	fireOnce(g)
	if p.weapon != wpPistol {
		t.Fatalf("disabled auto-switch equipped %s", weapons[p.weapon].Name)
	}
}

// TestAutoSwitchFallsBackToMelee covers the all-empty case.
func TestAutoSwitchFallsBackToMelee(t *testing.T) {
	g := armedGame(t, 3)
	p := &g.player
	p.owned[wpShotgun] = true
	p.ammo[wpPistol] = 1
	p.ammo[wpShotgun] = 0
	p.weapon = wpPistol

	fireOnce(g)
	if p.weapon != wpMelee {
		t.Fatalf("with every magazine empty the player should hold melee, got %s",
			weapons[p.weapon].Name)
	}

	// Melee keeps working forever and costs nothing.
	before := append([]int(nil), p.ammo...)
	for i := 0; i < 20; i++ {
		fireOnce(g)
	}
	for i := range before {
		if p.ammo[i] != before[i] {
			t.Fatalf("melee consumed %s ammo", weapons[i].Name)
		}
	}
}

// TestMeleeHitsInFrontOnly checks the swing is a short arc, not a free hit on
// anything nearby.
func TestMeleeHitsInFrontOnly(t *testing.T) {
	g := armedGame(t, 4)
	g.level = testLevel([]string{
		"###############",
		"#.............#",
		"#.............#",
		"#.............#",
		"###############",
	})
	g.player.pos = Vec{7.5, 2.5}
	g.player.weapon = wpMelee
	g.aim = g.player.pos.Add(Vec{5, 0}) // facing right

	// A tough enemy, so a single swing cannot kill it and shuffle the slice.
	tough := enemyDefs[5] // Brute
	for _, at := range []Vec{
		g.player.pos.Add(Vec{1.5, 0}),  // in front
		g.player.pos.Add(Vec{-1.5, 0}), // behind
		g.player.pos.Add(Vec{6, 0}),    // out of reach
	} {
		g.addEnemy(tough, at)
	}
	byID := map[int]float64{}
	for i := range g.enemies {
		byID[g.enemies[i].id] = g.enemies[i].hp
	}
	front, behind, far := g.enemies[0].id, g.enemies[1].id, g.enemies[2].id

	fireOnce(g)

	hp := map[int]float64{}
	for i := range g.enemies {
		hp[g.enemies[i].id] = g.enemies[i].hp
	}
	if hp[front] >= byID[front] {
		t.Error("the enemy in front was not hit")
	}
	if hp[behind] < byID[behind] {
		t.Error("an enemy behind the player was hit by the swing")
	}
	if hp[far] < byID[far] {
		t.Error("an enemy well out of reach was hit")
	}
}

// TestMeleeAlwaysSelectable checks slot 6 is never locked out.
func TestMeleeAlwaysSelectable(t *testing.T) {
	g := armedGame(t, 5)
	g.handleKey(Event{Kind: EvKey, Rune: '6'})
	if g.player.weapon != wpMelee {
		t.Fatalf("[6] selected %s, want melee", weapons[g.player.weapon].Name)
	}
	// And back again, since the pistol still has rounds.
	g.handleKey(Event{Kind: EvKey, Rune: '1'})
	if g.player.weapon != wpPistol {
		t.Fatalf("could not switch back to the pistol")
	}
}

// TestCannotSelectEmptyWeapon stops the player from equipping a dry gun by hand
// and standing there clicking.
func TestCannotSelectEmptyWeapon(t *testing.T) {
	g := armedGame(t, 6)
	g.player.owned[wpRailgun] = true
	g.player.ammo[wpRailgun] = 0

	g.handleKey(Event{Kind: EvKey, Rune: '4'})
	if g.player.weapon == wpRailgun {
		t.Fatal("equipped the railgun with no ammunition")
	}
}

func TestAmmoPickupKeepsEquippedWeapon(t *testing.T) {
	g := armedGame(t, 7)
	p := &g.player
	p.ammo[wpPistol] = 0
	p.weapon = wpMelee

	g.pickups = []Pickup{{pos: p.pos, kind: PickAmmo, weapon: wpPistol}}
	g.updatePickups(0.016)

	if p.ammo[wpPistol] != weapons[wpPistol].PickupAmmo {
		t.Fatalf("picked up %d rounds, want %d", p.ammo[wpPistol], weapons[wpPistol].PickupAmmo)
	}
	if p.weapon != wpMelee {
		t.Fatalf("ammo pickup equipped %s instead of keeping melee", weapons[p.weapon].Name)
	}
}

// TestAmmoNeverExceedsCapacity guards the top-up arithmetic.
func TestAmmoNeverExceedsCapacity(t *testing.T) {
	g := armedGame(t, 8)
	for i := range weapons {
		if !weapons[i].needsAmmo() {
			continue
		}
		g.player.owned[i] = true
		for n := 0; n < 30; n++ {
			g.giveAmmo(i, weapons[i].PickupAmmo)
		}
		if got := g.player.ammo[i]; got != weapons[i].MaxAmmo {
			t.Errorf("%s holds %d rounds, capacity is %d", weapons[i].Name, got, weapons[i].MaxAmmo)
		}
	}
}

// TestStrongWeaponsResupplySlower is the balance intent: the more powerful the
// weapon, the fewer rounds a floor hands you.
func TestStrongWeaponsResupplySlower(t *testing.T) {
	order := []int{wpPistol, wpSMG, wpShotgun, wpRailgun, wpLauncher}
	for i := 1; i < len(order); i++ {
		prev, cur := weapons[order[i-1]], weapons[order[i]]
		if cur.AmmoWeight > prev.AmmoWeight {
			t.Errorf("%s drops more often than %s", cur.Name, prev.Name)
		}
	}
	// Damage per magazine should not simply grow without bound.
	if weapons[wpLauncher].MaxAmmo >= weapons[wpPistol].MaxAmmo {
		t.Error("the launcher carries as much ammunition as the pistol")
	}
}

// TestAmmoDropsOnlyForOwnedWeapons stops floors littering rounds for guns the
// player cannot use.
func TestAmmoDropsOnlyForOwnedWeapons(t *testing.T) {
	g := armedGame(t, 9)
	g.player.owned[wpShotgun] = true
	for n := 0; n < 200; n++ {
		w := g.rollAmmoFor()
		if !g.player.owned[w] {
			t.Fatalf("dropped ammo for %s, which the player does not own", weapons[w].Name)
		}
		if !weapons[w].needsAmmo() {
			t.Fatalf("dropped ammo for %s, which uses none", weapons[w].Name)
		}
	}
}

// TestFloorAlwaysHasAmmo makes sure a run cannot be stranded on melee.
func TestFloorAlwaysHasAmmo(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		g := NewGame(seed)
		for depth := 0; depth < 3; depth++ {
			n := 0
			for _, pk := range g.pickups {
				if pk.kind == PickAmmo {
					n++
				}
			}
			if n == 0 {
				t.Fatalf("seed %d B%d has no ammunition anywhere", seed, g.depth)
			}
			g.enterDepth(g.depth + 1)
		}
	}
}

// TestAmmoShownInHUD keeps the system visible: the counts have to be on screen
// at every width, or the player cannot plan around them.
func TestAmmoShownInHUD(t *testing.T) {
	for _, w := range []int{60, 80, 100, 120, 160} {
		g := NewGame(3)
		g.player.ammo[wpPistol] = 137
		s := newTestScreen(w, 30)
		g.Draw(s)
		row := screenRow(s, 1)
		if !contains(row, "137") {
			t.Errorf("width %d: pistol ammo count missing from %q", w, row)
		}
		if !contains(row, "6MELE") && !contains(row, "6 Melee") {
			t.Errorf("width %d: melee slot missing from %q", w, row)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestDryRunResupplyIsGenerous checks the anti-death-spiral rule: with every
// magazine empty, kills drop ammunition far more often than normal.
func TestDryRunResupplyIsGenerous(t *testing.T) {
	count := func(dry bool) int {
		g := armedGame(t, 11)
		if dry {
			for i := range g.player.ammo {
				g.player.ammo[i] = 0
			}
		} else {
			g.player.ammo[wpPistol] = 100
		}
		drops := 0
		for n := 0; n < 400; n++ {
			g.pickups = g.pickups[:0]
			g.enemies = g.enemies[:0]
			g.addEnemy(enemyDefs[0], g.player.pos.Add(Vec{10, 0}))
			g.enemies[0].hp = 0
			g.reapEnemies()
			for _, pk := range g.pickups {
				if pk.kind == PickAmmo {
					drops++
				}
			}
		}
		return drops
	}
	stocked, dry := count(false), count(true)
	if dry <= stocked {
		t.Fatalf("dry runs got %d ammo drops from 400 kills, stocked runs got %d; "+
			"being empty should resupply faster", dry, stocked)
	}
	t.Logf("ammo drops per 400 kills: stocked %d, dry %d", stocked, dry)
}
