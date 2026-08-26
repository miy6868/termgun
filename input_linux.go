//go:build linux

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// readEvents parses stdin in a goroutine and publishes decoded events onto ch.
// The caller owns the channel because other input sources — the kernel keyboard
// reader — publish onto the same one.
func readEvents(in *os.File, fd int, leftover []byte, ch chan Event) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			w, h := terminalSize(fd)
			ch <- Event{Kind: EvResize, W: w, H: h}
		}
	}()

	go func() {
		// Anything typed during the capability handshake is replayed first.
		buf := append(make([]byte, 0, 1024), leftover...)
		tmp := make([]byte, 512)
		for {
			n, err := in.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				for {
					ev, used, ok := parseEvent(buf)
					if !ok {
						break
					}
					buf = buf[used:]
					if ev.Kind != EvKey || ev.Key != KeyNone || ev.Rune != 0 {
						ch <- ev
					}
				}
				// Drop a lone dangling ESC-prefixed fragment that never
				// completed; otherwise a partial sequence would wedge parsing.
				if len(buf) > 64 {
					buf = buf[:0]
				}
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()
}

// chooseInput picks the best key-state source available.
//
// The kitty protocol is preferred: it needs no permissions and is already
// scoped to this terminal. Reading /dev/input is the fallback that gives
// Konsole and friends the same movement as foot or kitty.
func chooseInput(want string, kitty bool, events chan Event) (InputMode, inputSource, string) {
	switch {
	case kitty && want != "device" && want != "compat":
		return InputKitty, nil, ""
	case want == "auto" || want == "device":
		dev, err := openEvdev(events)
		if err == nil {
			return InputDevice, dev, ""
		}
		if want == "device" {
			return InputCompat, nil, "장치 입력 실패: " + err.Error()
		}
		return InputCompat, nil, err.Error()
	}
	return InputCompat, nil, ""
}
