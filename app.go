package main

import (
	"fmt"
	"os"
	"time"
)

const (
	minCols = 60
	minRows = 20
	minFPS  = 15
	maxFPS  = 1000
)

// runGame owns the platform-neutral application lifecycle. OS-specific
// terminal and input details stay behind functions in the platform files.
func runGame(cfg appConfig, in, output *os.File) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	platform, err := openPlatform(in, output, cfg.inputMode)
	if err != nil {
		return err
	}
	defer platform.Close()
	defer func() {
		if r := recover(); r != nil {
			platform.Close()
			panic(r)
		}
	}()

	w, h := platform.size()
	screen := NewScreen(w, h, platform.out)
	events := platform.events()
	mode, dev, inputNote := platform.input(cfg.inputMode)
	if dev != nil {
		defer dev.Close()
	}

	s := cfg.seed
	if s == 0 {
		s = time.Now().UnixNano()
	}
	game := newGameWithInput(s, mode)
	game.zoom = clamp(cfg.zoom, minZoom, maxZoom)
	if mode.trueState() {
		game.log("키보드: "+mode.String()+" - 대각선 이동 지원", 46)
	} else {
		game.log("키보드: "+mode.String()+" - [?] 도움말 참고", 244)
	}
	if inputNote != "" {
		game.log(inputNote, 214)
	}

	frame := time.Second / time.Duration(cfg.fps)
	ticker := time.NewTicker(frame)
	defer ticker.Stop()

	last := time.Now()
	for {
		// Drain everything that arrived since the previous frame.
		draining := true
		for draining {
			select {
			case ev, ok := <-events:
				if !ok {
					return nil
				}
				if ev.Kind == EvStop {
					return nil
				}
				if ev.Kind == EvResize {
					screen.Resize(ev.W, ev.H)
					continue
				}
				game.HandleEvent(ev)
			default:
				draining = false
			}
		}
		if game.state == StateQuit {
			return nil
		}

		now := <-ticker.C
		dt := now.Sub(last).Seconds()
		last = now
		if dt > 0.1 {
			// Keeps a stall from advancing the simulation by an absurd amount at
			// once. It is not what stops bodies crossing walls — 0.1s of dash is
			// still 5.1 tiles, and it used to carry the player clean through a
			// two-tile wall. moveWithCollision substeps for that.
			dt = 0.1
		}

		if game.debugPerf {
			started := time.Now()
			game.Update(dt)
			updated := time.Now()
			if screen.W < minCols || screen.H < minRows {
				drawTooSmall(screen)
			} else {
				game.Draw(screen)
			}
			drawn := time.Now()
			screen.Flush()
			presented := time.Now()
			game.recordPerformance(updated.Sub(started), drawn.Sub(updated),
				presented.Sub(drawn), presented)
			continue
		}

		game.Update(dt)
		if screen.W < minCols || screen.H < minRows {
			drawTooSmall(screen)
		} else {
			game.Draw(screen)
		}
		screen.Flush()
	}
}

func drawTooSmall(s *Screen) {
	s.Clear()
	msg := fmt.Sprintf("터미널이 너무 작습니다 (%dx%d). 최소 %dx%d 필요", s.W, s.H, minCols, minRows)
	s.Str(0, 0, msg, 203, colorDefault)
}
