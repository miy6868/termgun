package main

import "fmt"

const (
	objectiveDirectionTime = 45.0
	objectiveSectorTime    = 60.0
	objectiveExactTime     = 70.0
	objectiveDirectionSeen = 0.55
	objectiveSectorSeen    = 0.75
)

// objectiveTarget follows the floor's current blocker. Boss floors point at
// the living boss first, then switch to the stairs after the kill.
func (g *Game) objectiveTarget() (Vec, string, bool) {
	if b := g.bossEnemy(); b != nil {
		return b.pos, "보스", true
	}
	return g.stairsPos, "계단", false
}

func (g *Game) exploredFloorFraction() float64 {
	explored, total := 0, 0
	for i, tile := range g.level.tiles {
		if tile.solid() {
			continue
		}
		total++
		if g.level.explored[i] {
			explored++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(explored) / float64(total)
}

func (g *Game) updateObjectiveHint(dt float64) {
	if g.ambushOn {
		return
	}
	g.objectiveHintTime += dt
	g.objectiveHintTick -= dt
	if g.objectiveHintTick > 0 {
		return
	}
	g.objectiveHintTick = 0.5

	target, name, boss := g.objectiveTarget()
	if !boss && g.level.Explored(int(target.X), int(target.Y)) {
		return
	}
	seen := g.exploredFloorFraction()
	next := g.objectiveHint
	if g.objectiveHintTime >= objectiveExactTime {
		next = 3
	} else if g.objectiveHintTime >= objectiveSectorTime || seen >= objectiveSectorSeen {
		next = max(next, 2)
	} else if g.objectiveHintTime >= objectiveDirectionTime || seen >= objectiveDirectionSeen {
		next = max(next, 1)
	}
	if next <= g.objectiveHint {
		return
	}
	g.objectiveHint = next
	dir := objectiveDirection(g.player.pos, target)
	switch next {
	case 1:
		g.log(fmt.Sprintf("목표 기류: %s은 %s쪽", name, dir), 117)
	case 2:
		g.log(fmt.Sprintf("%s 구역이 미니맵에서 점멸한다.", name), 51)
	case 3:
		g.log(fmt.Sprintf("%s 위치가 미니맵에 고정됐다.", name), 226)
	}
}

func objectiveDirection(from, to Vec) string {
	dx, dy := to.X-from.X, to.Y-from.Y
	x, y := "", ""
	if dx > 1 {
		x = "동"
	} else if dx < -1 {
		x = "서"
	}
	if dy > 1 {
		y = "남"
	} else if dy < -1 {
		y = "북"
	}
	if y != "" && x != "" {
		return y + x
	}
	if y != "" {
		return y
	}
	if x != "" {
		return x
	}
	return "가까운 곳"
}
