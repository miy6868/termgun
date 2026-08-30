package main

import (
	"math/rand"
	"testing"
)

func TestRoomGraphIsConnectedAndLoopedAcrossActs(t *testing.T) {
	for _, depth := range []int{1, 6, 11} {
		for seed := int64(1); seed <= 100; seed++ {
			l := GenerateLevel(depth, rand.New(rand.NewSource(seed)))
			n := len(l.rooms)
			if n < 3 {
				t.Fatalf("seed %d B%d generated only %d rooms", seed, depth, n)
			}
			if len(l.roomLinks) < n+1 {
				t.Fatalf("seed %d B%d has %d rooms but only %d links; loops are missing",
					seed, depth, n, len(l.roomLinks))
			}

			adj := make([][]int, n)
			for _, edge := range l.roomLinks {
				a, b := edge[0], edge[1]
				if a < 0 || b < 0 || a >= n || b >= n || a == b {
					t.Fatalf("seed %d B%d has invalid room link %v", seed, depth, edge)
				}
				adj[a] = append(adj[a], b)
				adj[b] = append(adj[b], a)
			}
			seen := make([]bool, n)
			queue := []int{0}
			seen[0] = true
			for len(queue) > 0 {
				a := queue[0]
				queue = queue[1:]
				for _, b := range adj[a] {
					if !seen[b] {
						seen[b] = true
						queue = append(queue, b)
					}
				}
			}
			for i, ok := range seen {
				if !ok {
					t.Fatalf("seed %d B%d room %d is disconnected in the planned graph", seed, depth, i)
				}
			}
		}
	}
}

func TestImprovedMapTilesStayReachableAcrossActs(t *testing.T) {
	for _, depth := range []int{1, 6, 11} {
		for seed := int64(1); seed <= 80; seed++ {
			l := GenerateLevel(depth, rand.New(rand.NewSource(seed)))
			sx, sy := l.rooms[0].center()
			l.BuildFlow(sx, sy)
			for i, room := range l.rooms {
				cx, cy := room.center()
				if l.Solid(cx, cy) || l.flow[l.idx(cx, cy)] >= flowUnreachable {
					t.Fatalf("seed %d B%d room %d center (%d,%d) is unreachable",
						seed, depth, i, cx, cy)
				}
			}
		}
	}
}

func TestActsHaveDifferentRoomSilhouettes(t *testing.T) {
	organicCorners, reactorCover := 0, 0
	for seed := int64(1); seed <= 40; seed++ {
		bio := GenerateLevel(6, rand.New(rand.NewSource(seed)))
		for _, r := range bio.rooms {
			for _, p := range [][2]int{{r.X, r.Y}, {r.X + r.W - 1, r.Y}, {r.X, r.Y + r.H - 1}, {r.X + r.W - 1, r.Y + r.H - 1}} {
				if bio.Solid(p[0], p[1]) {
					organicCorners++
				}
			}
		}

		reactor := GenerateLevel(11, rand.New(rand.NewSource(seed)))
		for _, r := range reactor.rooms {
			for y := r.Y + 2; y < r.Y+r.H-2; y++ {
				for x := r.X + 2; x < r.X+r.W-2; x++ {
					if reactor.Solid(x, y) {
						reactorCover++
					}
				}
			}
		}
	}
	if organicCorners == 0 {
		t.Fatal("biolab rooms are still all rectangles")
	}
	if reactorCover == 0 {
		t.Fatal("reactor rooms contain no interior cover")
	}
}
