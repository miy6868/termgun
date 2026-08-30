package main

import (
	"testing"
	"time"
)

func TestPerformanceSamplerReportsPresentedFPS(t *testing.T) {
	g := NewGame(91)
	start := time.Unix(100, 0)
	g.recordPerformance(100*time.Microsecond, 200*time.Microsecond, 300*time.Microsecond, start)
	g.recordPerformance(100*time.Microsecond, 200*time.Microsecond, 300*time.Microsecond,
		start.Add(time.Second/60))

	if g.perf.fps < 59.9 || g.perf.fps > 60.1 {
		t.Fatalf("presented FPS = %.2f, want 60", g.perf.fps)
	}
	if g.perf.updateMS <= 0 || g.perf.drawMS <= 0 || g.perf.outputMS <= 0 {
		t.Fatalf("component timings were not recorded: %+v", g.perf)
	}
}
