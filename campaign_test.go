package main

import (
	"strings"
	"testing"
)

func TestCampaignEndsAtB15(t *testing.T) {
	g := NewGame(5150)
	g.enterDepth(campaignDepth)
	out := g.enemies[:0]
	for _, e := range g.enemies {
		if e.def.Behavior != BeBoss {
			out = append(out, e)
		}
	}
	g.enemies = out
	g.player.pos = g.stairsPos
	g.tryStairs()

	if g.state != StateVictory {
		t.Fatalf("leaving B%d entered state %v, want victory", campaignDepth, g.state)
	}
	if g.depth != campaignDepth {
		t.Fatalf("victory generated B%d instead of ending at B%d", g.depth, campaignDepth)
	}
}

func TestVictoryScreenShowsRunSummary(t *testing.T) {
	g := NewGame(5150)
	g.depth = campaignDepth
	g.state = StateVictory
	s := newTestScreen(80, 28)
	g.Draw(s)
	text := overlayText(s)
	for _, want := range []string{"C O R E", "귀환", "B15층", "점수", "시드", "[R]"} {
		if !strings.Contains(text, want) {
			t.Errorf("victory screen does not show %q:\n%s", want, text)
		}
	}
}

func TestCampaignActNames(t *testing.T) {
	cases := map[int]string{1: "정비 구역", 5: "정비 구역", 6: "배양 구역", 10: "배양 구역", 11: "반응로 구역", 15: "반응로 구역"}
	for depth, want := range cases {
		if got := actName(depth); got != want {
			t.Errorf("B%d act = %q, want %q", depth, got, want)
		}
	}
}
