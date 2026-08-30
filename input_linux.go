//go:build linux

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type terminationSource struct {
	signals chan os.Signal
	close   sync.Once
}

// watchTermination turns process termination into an ordinary application
// event so deferred terminal restoration still runs.
func watchTermination(events chan<- Event) inputSource {
	s := &terminationSource{signals: make(chan os.Signal, 1)}
	signal.Notify(s.signals, syscall.SIGTERM, syscall.SIGHUP)
	go forwardTermination(s.signals, events)
	return s
}

func forwardTermination(signals <-chan os.Signal, events chan<- Event) {
	if _, ok := <-signals; ok {
		events <- Event{Kind: EvStop}
	}
}

func (s *terminationSource) Close() {
	s.close.Do(func() {
		signal.Stop(s.signals)
		close(s.signals)
	})
}

// readEvents publishes decoded events onto ch. No producer closes the channel:
// stdin, SIGWINCH, and the kernel keyboard reader all share it.
func readEvents(in *os.File, fd int, leftover []byte, ch chan Event) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			w, h := terminalSize(fd)
			ch <- Event{Kind: EvResize, W: w, H: h}
		}
	}()

	go readTTYEvents(in, leftover, ch)
}

func readTTYEvents(in *os.File, leftover []byte, ch chan<- Event) {
	// Anything typed during the capability handshake is replayed first.
	buf := append(make([]byte, 0, 1024), leftover...)
	tmp := make([]byte, 512)
	timedReads := isTTY(in)
	for {
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
		// Drop a dangling ESC-prefixed fragment that grew beyond any supported
		// sequence; otherwise malformed input would wedge parsing indefinitely.
		if len(buf) > 64 {
			buf = buf[:0]
		}

		n, err := syscall.Read(int(in.Fd()), tmp)
		if err == syscall.EINTR {
			continue
		}
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			continue
		}
		if n == 0 && timedReads && err == nil {
			// ESC is both a complete key and the prefix of every legacy control
			// sequence. The terminal's 100 ms idle read is the only reliable way
			// to distinguish a lone press without delaying normal sequences.
			if len(buf) == 1 && buf[0] == 0x1b {
				ch <- Event{Kind: EvKey, Key: KeyEscape, KeyAct: KeyPress, Src: SrcTTY}
				buf = buf[:0]
			}
			continue
		}
		if err != nil || n == 0 {
			ch <- Event{Kind: EvStop}
			return
		}
	}
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
