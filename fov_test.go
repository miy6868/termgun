package main

import "testing"

// TestFovCacheMatchesFreshCast plays real runs and checks that every time
// ComputeFOV serves its cache, the visibility set it hands back is identical to
// one cast from scratch.
//
// The cache exists because ComputeFOV is called every frame while the player
// only crosses a tile a few times a second. What makes it risky is that the
// result also depends on the terrain: a door shutting or a cracked wall opening
// changes what can be seen from a tile the player is already standing on. Both
// tile writers therefore call invalidateTerrain, and dropping either one turns
// into a player seeing through a closed ambush door — which this catches at the
// frame the doors shut, not several floors later.
func TestFovCacheMatchesFreshCast(t *testing.T) {
	const radius = 15 // what Game always asks for
	for seed := int64(1); seed <= 12; seed++ {
		g := NewGame(seed)
		g.mouseSet = true
		for f := 0; f < 3000; f++ {
			botFrame(g)
			if g.state == StateDead {
				break
			}
			// A frame that invalidated the cache after casting serves no cached
			// data on the next frame either, so there is nothing to compare.
			if g.level.fovSrc == [3]int{-1, -1, -1} {
				continue
			}
			cached := append([]bool(nil), g.level.visible...)
			g.level.fovSrc = [3]int{-1, -1, -1}
			g.level.ComputeFOV(int(g.player.pos.X), int(g.player.pos.Y), radius)
			for i := range cached {
				if cached[i] != g.level.visible[i] {
					t.Fatalf("seed %d frame %d: cached visibility disagrees with a fresh "+
						"cast at tile (%d,%d) — a terrain change did not invalidate the FOV",
						seed, f, i%g.level.W, i/g.level.W)
				}
			}
		}
	}
}
