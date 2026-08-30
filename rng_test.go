package main

import "testing"

// Presentation may be sampled at a different cadence on a slow terminal. It
// must never advance the simulation stream that owns generation, loot and AI.
func TestPresentationDoesNotAdvanceSimulationRNG(t *testing.T) {
	control := NewGame(424242)
	drawn := NewGame(424242)
	drawn.shake = 1
	s := newTestScreen(120, 40)
	for i := 0; i < 20; i++ {
		drawn.Draw(s)
		drawn.spawnParticles(drawn.player.pos, 5, 0.3, 231, '*')
		drawn.stairsShimmer()
		drawn.acidBubbles()
	}

	for i := 0; i < 32; i++ {
		if got, want := drawn.rng.Int63(), control.rng.Int63(); got != want {
			t.Fatalf("presentation changed simulation RNG at sample %d: got %d, want %d", i, got, want)
		}
	}
}
