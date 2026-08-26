//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Reading the keyboard straight from the kernel.
//
// Terminals that do not implement the kitty keyboard protocol never report key
// releases, which makes held keys and diagonals guesswork. The kernel's evdev
// interface has the real thing: every press and release, independent of the
// terminal. This is what closes the gap between, say, foot and Konsole.
//
// Two things make it more than "just open the device":
//
//   - Permissions. /dev/input/event* is usually root:input mode 0660, so the
//     user has to be in the "input" group. When that is not the case the game
//     says so and falls back instead of failing.
//
//   - Focus. These events arrive whether or not the terminal is focused, so a
//     game reading them naively keeps walking while you type in another window.
//     The terminal's focus reporting (CSI ?1004h) tells us when to ignore them.
//
// Only the movement keys are read. Everything else still comes from the
// terminal, which keeps the amount of global keyboard traffic this process
// touches to the smallest set that actually needs it.

const (
	evKey = 0x01

	keyW     = 17
	keyA     = 30
	keyS     = 31
	keyD     = 32
	keyUp    = 103
	keyLeft  = 105
	keyRight = 106
	keyDown  = 108
)

// evdevKeys is the entire set of keys this process looks at.
var evdevKeys = map[uint16]int{
	keyW: dirUp, keyA: dirLeft, keyS: dirDown, keyD: dirRight,
	keyUp: dirUp, keyLeft: dirLeft, keyDown: dirDown, keyRight: dirRight,
}

// inputEvent mirrors the kernel's struct input_event on 64-bit Linux.
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

const inputEventSize = 24

// evdevSource streams movement key transitions from every attached keyboard.
type evdevSource struct {
	files []*os.File
}

// openEvdev finds the keyboards and starts reading them. A nil source with a
// non-nil error means the caller should fall back to terminal input; the error
// is written for a player to act on, not just to log.
func openEvdev(ch chan<- Event) (*evdevSource, error) {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("/dev/input 에 입력 장치가 없습니다")
	}

	src := &evdevSource{}
	denied := 0
	for _, p := range paths {
		fd, err := syscall.Open(p, syscall.O_RDONLY, 0)
		if err != nil {
			if err == syscall.EACCES || err == syscall.EPERM {
				denied++
			}
			continue
		}
		if !isKeyboard(fd) {
			syscall.Close(fd)
			continue
		}
		f := os.NewFile(uintptr(fd), p)
		src.files = append(src.files, f)
		go readEvdev(f, ch)
	}

	if len(src.files) == 0 {
		src.Close()
		if denied > 0 {
			return nil, fmt.Errorf("/dev/input 읽기 권한이 없습니다 (sudo usermod -aG input $USER 후 재로그인)")
		}
		return nil, fmt.Errorf("키보드 장치를 찾지 못했습니다")
	}
	return src, nil
}

func (s *evdevSource) Close() {
	for _, f := range s.files {
		f.Close()
	}
	s.files = nil
}

// isKeyboard checks the device advertises the letter keys we care about, which
// separates real keyboards from mice, touchpads and power buttons.
func isKeyboard(fd int) bool {
	const keyMax = 0x2ff
	bits := make([]byte, keyMax/8+1)
	// EVIOCGBIT(EV_KEY, len): _IOC(_IOC_READ, 'E', 0x20+EV_KEY, len)
	req := uintptr(2<<30 | len(bits)<<16 | 'E'<<8 | (0x20 + evKey))
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req,
		uintptr(unsafe.Pointer(&bits[0]))); errno != 0 {
		return false
	}
	for _, code := range []uint16{keyW, keyA, keyS, keyD} {
		if bits[code/8]&(1<<(code%8)) == 0 {
			return false
		}
	}
	return true
}

func readEvdev(f *os.File, ch chan<- Event) {
	buf := make([]byte, inputEventSize*32)
	for {
		n, err := f.Read(buf)
		if err != nil {
			return
		}
		for off := 0; off+inputEventSize <= n; off += inputEventSize {
			var ev inputEvent
			ev.Type = binary.LittleEndian.Uint16(buf[off+16:])
			ev.Code = binary.LittleEndian.Uint16(buf[off+18:])
			ev.Value = int32(binary.LittleEndian.Uint32(buf[off+20:]))
			if ev.Type != evKey {
				continue
			}
			dir, ok := evdevKeys[ev.Code]
			if !ok {
				continue
			}
			out := Event{Kind: EvKey, Src: SrcDevice, Dir: dir}
			switch ev.Value {
			case 0:
				out.KeyAct = KeyRelease
			case 1:
				out.KeyAct = KeyPress
			case 2:
				out.KeyAct = KeyRepeat
			default:
				continue
			}
			select {
			case ch <- out:
			default: // never block the reader on a full queue
			}
		}
	}
}
