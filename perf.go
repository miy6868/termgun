package main

import (
	"runtime"
	"time"
)

const (
	perfSmoothing    = 0.15
	perfMemoryPeriod = 500 * time.Millisecond
	perfPeakPeriod   = time.Second
)

type perfStats struct {
	lastPresent time.Time
	lastMemory  time.Time
	peakStart   time.Time

	fps       float64
	frameMS   float64
	updateMS  float64
	drawMS    float64
	outputMS  float64
	peakMS    float64
	heapBytes uint64
	numGC     uint32
	goroutine int

	lines [4][96]byte
}

func smoothPerf(current, sample float64) float64 {
	if current == 0 {
		return sample
	}
	return current*(1-perfSmoothing) + sample*perfSmoothing
}

// recordPerformance runs only while the debug overlay is enabled. Component
// timers describe the completed frame; the panel displays them one frame later.
func (g *Game) recordPerformance(update, draw, output time.Duration, now time.Time) {
	p := &g.perf
	if !p.lastPresent.IsZero() {
		frameMS := float64(now.Sub(p.lastPresent)) / float64(time.Millisecond)
		if frameMS > 0 && frameMS < 1000 {
			p.fps = smoothPerf(p.fps, 1000/frameMS)
			p.frameMS = smoothPerf(p.frameMS, frameMS)
			if p.peakStart.IsZero() || now.Sub(p.peakStart) >= perfPeakPeriod {
				p.peakStart, p.peakMS = now, frameMS
			} else if frameMS > p.peakMS {
				p.peakMS = frameMS
			}
		}
	}
	p.lastPresent = now
	p.updateMS = smoothPerf(p.updateMS, float64(update)/float64(time.Millisecond))
	p.drawMS = smoothPerf(p.drawMS, float64(draw)/float64(time.Millisecond))
	p.outputMS = smoothPerf(p.outputMS, float64(output)/float64(time.Millisecond))

	if p.lastMemory.IsZero() || now.Sub(p.lastMemory) >= perfMemoryPeriod {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		p.heapBytes = mem.HeapAlloc
		p.numGC = mem.NumGC
		p.goroutine = runtime.NumGoroutine()
		p.lastMemory = now
	}
}
