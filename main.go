package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"
)

const (
	minCols = 60
	minRows = 20
)

func main() {
	seed := flag.Int64("seed", 0, "던전 시드 (0이면 현재 시각)")
	fps := flag.Int("fps", 60, "목표 프레임 레이트")
	zoom := flag.Int("zoom", defaultZoom,
		fmt.Sprintf("화면 배율: 월드 타일 하나를 터미널 칸 몇 개로 그릴지 (%d~%d)", minZoom, maxZoom))
	inputFlag := flag.String("input", "auto",
		"키보드 입력 방식: auto | kitty | device | compat\n"+
			"  device 는 /dev/input 을 직접 읽습니다 (Linux, input 그룹 권한 필요)\n"+
			"  Windows 콘솔은 키 상태를 직접 알려주므로 이 옵션이 필요 없습니다")
	flag.Parse()

	if !isTTY(os.Stdout) || !isTTY(os.Stdin) {
		fmt.Fprintln(os.Stderr, "이 게임은 대화형 터미널에서 실행해야 합니다.")
		os.Exit(1)
	}

	fd := int(os.Stdin.Fd())
	st, err := enterRaw(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "raw 모드 진입 실패:", err)
		os.Exit(1)
	}

	out := bufio.NewWriterSize(os.Stdout, 1<<16)

	// Ask about the kitty keyboard protocol before switching screens, so any
	// terminal that echoes the query instead of answering it does not leave
	// stray text on the game screen.
	kitty, pending := false, []byte(nil)
	if *inputFlag == "auto" || *inputFlag == "kitty" {
		kitty, pending = detectKitty(fd, out)
	}

	out.WriteString(seqEnterAlt + seqHideCursor + seqClear + inputEnableSeq)
	if kitty {
		out.WriteString(seqKittyPush)
	}
	out.Flush()

	// Always put the terminal back, even on panic.
	restore := func() {
		if kitty {
			out.WriteString(seqKittyPop)
		}
		out.WriteString(inputDisableSeq + seqShowCursor + seqReset + seqLeaveAlt)
		out.Flush()
		st.restore()
	}
	defer restore()
	defer func() {
		if r := recover(); r != nil {
			restore()
			panic(r)
		}
	}()

	w, h := terminalSize(fd)
	screen := NewScreen(w, h, out)
	events := make(chan Event, 256)
	readEvents(os.Stdin, fd, pending, events)

	// Where key state comes from is a per-platform question: see chooseInput in
	// input_linux.go and input_windows.go.
	mode, dev, inputNote := chooseInput(*inputFlag, kitty, events)
	if dev != nil {
		defer dev.Close()
	}

	s := *seed
	if s == 0 {
		s = time.Now().UnixNano()
	}
	game := newGameWithInput(s, mode)
	game.zoom = clamp(*zoom, minZoom, maxZoom)
	if mode.trueState() {
		game.log("키보드: "+mode.String()+" - 대각선 이동 지원", 46)
	} else {
		game.log("키보드: "+mode.String()+" - [?] 도움말 참고", 244)
	}
	if inputNote != "" {
		game.log(inputNote, 214)
	}

	frame := time.Second / time.Duration(maxInt(*fps, 15))
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
					return
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
			return
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
