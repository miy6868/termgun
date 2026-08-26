//go:build windows

package main

// Terminal control on Windows.
//
// The console has to be talked into behaving like a terminal in three separate
// ways, and each one is a different API:
//
//   - Output. ENABLE_VIRTUAL_TERMINAL_PROCESSING makes the console interpret
//     the ANSI sequences the renderer already emits. Without it they are
//     printed literally.
//   - Text. The console's output code page decides how bytes become glyphs, and
//     it is not UTF-8 by default, so the Korean in the HUD comes out as
//     mojibake until it is switched.
//   - Input. Rather than asking for VT input sequences, the game reads console
//     records directly (input_windows.go). Those carry key *releases*, which is
//     the thing a plain terminal cannot report and the reason diagonal movement
//     needs the kitty protocol or /dev/input on Linux. On Windows it is simply
//     how the console works, so this platform gets precise movement for free.

import (
	"bufio"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// The console reports mouse and focus itself, so unlike Linux there is nothing
// to switch on with escape sequences.
const (
	inputEnableSeq  = ""
	inputDisableSeq = ""
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procGetConsoleOutputCP         = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP         = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP               = kernel32.NewProc("SetConsoleCP")
)

// Console mode flags.
const (
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableWindowInput    = 0x0008
	enableMouseInput     = 0x0010
	enableExtendedFlags  = 0x0080
	enableQuickEdit      = 0x0040

	enableProcessedOutput    = 0x0001
	enableVTProcessing       = 0x0004
	disableNewlineAutoReturn = 0x0008

	cpUTF8 = 65001
)

func getConsoleMode(h uintptr) (uint32, bool) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	return mode, r != 0
}

func setConsoleMode(h uintptr, mode uint32) bool {
	r, _, _ := procSetConsoleMode.Call(h, uintptr(mode))
	return r != 0
}

type termState struct {
	inHandle, outHandle uintptr
	inMode, outMode     uint32
	outCP               uint32
}

// enterRaw puts the console into the shape the game needs and remembers enough
// to put it back.
func enterRaw(fd int) (*termState, error) {
	in := uintptr(fd)
	out := os.Stdout.Fd()

	inMode, ok := getConsoleMode(in)
	if !ok {
		return nil, errors.New("콘솔이 아닙니다 (Windows Terminal 이나 PowerShell 창에서 실행하세요)")
	}
	outMode, ok := getConsoleMode(out)
	if !ok {
		return nil, errors.New("콘솔 출력 핸들을 열 수 없습니다")
	}
	cp, _, _ := procGetConsoleOutputCP.Call()

	st := &termState{
		inHandle: in, outHandle: out,
		inMode: inMode, outMode: outMode, outCP: uint32(cp),
	}

	// Line editing and echo off; window, mouse and key records on. Quick-edit
	// has to go too: with it on, the console treats a click as the start of a
	// text selection and the game never sees the button at all. Turning it off
	// requires setting the extended-flags bit in the same call.
	newIn := inMode
	newIn &^= enableProcessedInput | enableLineInput | enableEchoInput | enableQuickEdit
	newIn |= enableWindowInput | enableMouseInput | enableExtendedFlags
	if !setConsoleMode(in, newIn) {
		return nil, errors.New("콘솔 입력 모드를 설정할 수 없습니다")
	}

	newOut := outMode | enableProcessedOutput | enableVTProcessing | disableNewlineAutoReturn
	if !setConsoleMode(out, newOut) {
		// Without VT processing every escape sequence would be printed as text,
		// which is worse than refusing to start.
		setConsoleMode(in, inMode)
		return nil, errors.New(
			"콘솔이 ANSI 이스케이프를 지원하지 않습니다. Windows 10 1511 이상이 필요하며, " +
				"Windows Terminal 을 권장합니다")
	}

	procSetConsoleOutputCP.Call(cpUTF8)
	procSetConsoleCP.Call(cpUTF8)
	return st, nil
}

func (s *termState) restore() {
	setConsoleMode(s.inHandle, s.inMode)
	setConsoleMode(s.outHandle, s.outMode)
	if s.outCP != 0 {
		procSetConsoleOutputCP.Call(uintptr(s.outCP))
		procSetConsoleCP.Call(uintptr(s.outCP))
	}
}

// detectKitty is a no-op here. No Windows console implements the kitty
// keyboard protocol, and it is not needed: console records already carry key
// releases. Probing anyway would just stall startup waiting for a reply that
// never comes.
func detectKitty(fd int, out *bufio.Writer) (bool, []byte) { return false, nil }

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func screenBufferInfo() (consoleScreenBufferInfo, bool) {
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(
		os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	return info, r != 0
}

// terminalSize reports the *window*, not the buffer. The buffer can be taller
// than the visible window, and drawing to the buffer's height would put most of
// the frame where nobody can see it.
func terminalSize(fd int) (w, h int) {
	info, ok := screenBufferInfo()
	if !ok {
		return 80, 24
	}
	w = int(info.Window.Right-info.Window.Left) + 1
	h = int(info.Window.Bottom-info.Window.Top) + 1
	if w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func isTTY(f *os.File) bool {
	_, ok := getConsoleMode(f.Fd())
	return ok
}
