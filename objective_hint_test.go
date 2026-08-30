package main

import "testing"

func TestObjectiveHintsAdvanceByTime(t *testing.T) {
	g := arena(t, 71)
	for i := range g.level.explored {
		g.level.explored[i] = false
	}
	g.objectiveHintTime = objectiveDirectionTime
	g.objectiveHintTick = 0
	g.updateObjectiveHint(0)
	if g.objectiveHint != 1 {
		t.Fatalf("direction hint stage = %d, want 1", g.objectiveHint)
	}
	g.objectiveHintTime = objectiveSectorTime
	g.objectiveHintTick = 0
	g.updateObjectiveHint(0)
	if g.objectiveHint != 2 {
		t.Fatalf("sector hint stage = %d, want 2", g.objectiveHint)
	}
	g.objectiveHintTime = objectiveExactTime
	g.objectiveHintTick = 0
	g.updateObjectiveHint(0)
	if g.objectiveHint != 3 {
		t.Fatalf("exact hint stage = %d, want 3", g.objectiveHint)
	}
}

func TestObjectiveHintsAdvanceByExploration(t *testing.T) {
	g := arena(t, 72)
	g.stairsPos = Vec{28.5, 4.5}
	open := make([]int, 0, len(g.level.tiles))
	for i, tile := range g.level.tiles {
		g.level.explored[i] = false
		if !tile.solid() {
			open = append(open, i)
		}
	}
	for _, i := range open[:int(float64(len(open))*objectiveSectorSeen)+1] {
		g.level.explored[i] = true
	}
	// Keep the stair tile unknown so finding unrelated rooms drives the hint.
	g.level.explored[g.level.idx(int(g.stairsPos.X), int(g.stairsPos.Y))] = false
	g.objectiveHintTick = 0
	g.updateObjectiveHint(0)
	if g.objectiveHint != 2 {
		t.Fatalf("75%% exploration produced stage %d, want 2", g.objectiveHint)
	}
}

func TestAmbushPausesObjectiveHintClock(t *testing.T) {
	g := arena(t, 73)
	g.ambushOn = true
	g.objectiveHintTime = objectiveDirectionTime - 1
	g.updateObjectiveHint(10)
	if g.objectiveHintTime != objectiveDirectionTime-1 || g.objectiveHint != 0 {
		t.Fatalf("ambush advanced hint clock/stage: %.1f/%d",
			g.objectiveHintTime, g.objectiveHint)
	}
}

func TestObjectiveTargetsBossBeforeStairs(t *testing.T) {
	g := arena(t, 74)
	g.addEnemy(bossDef, Vec{10.5, 10.5})
	pos, name, boss := g.objectiveTarget()
	if !boss || name != "보스" || pos != g.enemies[0].pos {
		t.Fatalf("living boss was not the objective: %v %q %v", pos, name, boss)
	}
	g.enemies[0].hp = 0
	pos, name, boss = g.objectiveTarget()
	if boss || name != "계단" || pos != g.stairsPos {
		t.Fatalf("stairs did not become the objective after the kill: %v %q %v", pos, name, boss)
	}
}
