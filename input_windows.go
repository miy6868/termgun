//go:build windows

package main

// Reading the Windows console.
//
// One API delivers everything the game needs: key presses *and releases*, mouse
// position and buttons, focus changes, and window resizes. On Linux those come
// from four different places (the terminal, SGR mouse reporting, CSI ?1004h,
// and SIGWINCH), and true key state needs either a terminal that speaks the
// kitty protocol or read access to /dev/input. Here it is just how the console
// works, so Windows gets precise movement without asking for anything.

import (
	"os"
	"time"
	"unsafe"
)

var (
	procReadConsoleInput    = kernel32.NewProc("ReadConsoleInputW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
)

const (
	waitObject0      = 0
	waitFailed       = ^uintptr(0)
	waitResizePollMS = 250
	waitMouseFlushMS = 8
)

// readEvents pumps console records onto ch. leftover is unused: nothing was
// read ahead of this point, because there is no capability handshake to run.
func readEvents(in *os.File, fd int, leftover []byte, ch chan Event) {
	handle := uintptr(fd)

	go func() {
		defer close(ch)
		held := winKeyState{}
		var buttons uint32
		recs := make([]winRecord, 64)
		coalescer := winEventCoalescer{}
		emit := func(ev Event) { ch <- ev }

		// The window origin shifts the mouse coordinates, which the console
		// reports against the screen buffer rather than the visible window.
		originX, originY := 0, 0
		if info, ok := screenBufferInfo(); ok {
			originX, originY = int(info.Window.Left), int(info.Window.Top)
		}

		w, h := terminalSize(fd)
		for {
			coalescer.flushDue(time.Now(), emit)
			// Waiting with a timeout keeps input and resize polling in one
			// producer. A separate polling goroutine could send after this one
			// closes ch when the console handle fails.
			waitMS := uintptr(waitResizePollMS)
			if coalescer.hasMouse {
				waitMS = waitMouseFlushMS
			}
			ready, _, _ := procWaitForSingleObject.Call(handle, waitMS)
			if ready == waitFailed {
				return
			}
			nw, nh := terminalSize(fd)
			if nw != w || nh != h {
				w, h = nw, nh
				if info, ok := screenBufferInfo(); ok {
					originX, originY = int(info.Window.Left), int(info.Window.Top)
				}
				coalescer.offer(Event{Kind: EvResize, W: w, H: h}, time.Now(), emit)
			}
			if ready != waitObject0 {
				continue
			}

			var read uint32
			r, _, _ := procReadConsoleInput.Call(
				handle,
				uintptr(unsafe.Pointer(&recs[0])),
				uintptr(len(recs)),
				uintptr(unsafe.Pointer(&read)),
			)
			if r == 0 {
				return
			}
			for i := 0; i < int(read); i++ {
				rec := &recs[i]
				switch rec.eventType {
				case winKeyEvent:
					if ev, ok := decodeKey(rec, held); ok {
						coalescer.offer(ev, time.Now(), emit)
					}
				case winMouseEvent:
					var ev Event
					var ok bool
					ev, buttons, ok = decodeMouse(rec, buttons, originX, originY)
					if ok {
						coalescer.offer(ev, time.Now(), emit)
					}
				case winFocusEvent:
					focus := rec.u32(0) != 0
					if !focus {
						// Every held key is released as far as this process is
						// concerned; otherwise the player keeps walking while
						// typing in another window.
						held = winKeyState{}
					}
					coalescer.offer(Event{Kind: EvFocus, Focus: focus}, time.Now(), emit)
				case winResizeEvent:
					if info, ok := screenBufferInfo(); ok {
						originX, originY = int(info.Window.Left), int(info.Window.Top)
					}
					w, h = terminalSize(fd)
					coalescer.offer(Event{Kind: EvResize, W: w, H: h}, time.Now(), emit)
				}
			}
		}
	}()
}

// chooseInput has nothing to choose between. The console reports releases
// whether or not the game wants them, so there is no honest way to offer the
// compat path here: it infers held keys from auto-repeat timing, and feeding it
// real releases as well would have the two fighting each other.
func chooseInput(want string, kitty bool, events chan Event) (InputMode, inputSource, string) {
	if want != "auto" {
		return InputWinConsole, nil,
			"Windows 콘솔은 키 상태를 직접 알려주므로 -input 설정은 무시됩니다."
	}
	return InputWinConsole, nil, ""
}
