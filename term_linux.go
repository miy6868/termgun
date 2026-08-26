//go:build linux

package main

// Terminal control on Linux: raw mode, size, and the kitty capability probe.
// The Windows half of these lives in term_windows.go; everything above the two
// files — the screen buffer, the ANSI sequences — is shared.

import (
	"bufio"
	"bytes"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// Mouse and focus reporting are asked for with escape sequences here. On
// Windows the console reports both natively, so there is nothing to enable.
const (
	inputEnableSeq  = seqMouseOn + seqFocusOn
	inputDisableSeq = seqFocusOff + seqMouseOff
)

// ---- raw mode ---------------------------------------------------------------

type termState struct {
	fd   int
	orig syscall.Termios
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

// enterRaw disables echo, canonical mode and signal generation so the game sees
// every keystroke the instant it is typed.
func enterRaw(fd int) (*termState, error) {
	var t syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, unsafe.Pointer(&t)); err != nil {
		return nil, err
	}
	st := &termState{fd: fd, orig: t}

	t.Iflag &^= syscall.IXON | syscall.ICRNL | syscall.BRKINT | syscall.INPCK | syscall.ISTRIP
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, unsafe.Pointer(&t)); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *termState) restore() {
	orig := s.orig
	ioctl(s.fd, syscall.TCSETS, unsafe.Pointer(&orig))
}

// detectKitty asks whether the terminal implements the kitty keyboard protocol
// and returns any bytes that were read past the reply, so the caller can feed
// them back to the normal input parser.
//
// The query is followed by a Primary Device Attributes request: every terminal
// answers DA1, so its reply marks the end of the window in which a kitty reply
// could have arrived. That avoids waiting on a timeout alone.
func detectKitty(fd int, out *bufio.Writer) (supported bool, leftover []byte) {
	var t syscall.Termios
	if ioctl(fd, syscall.TCGETS, unsafe.Pointer(&t)) != nil {
		return false, nil
	}
	saved := t
	// Read with a 300 ms inactivity timeout instead of blocking forever, in
	// case the terminal answers neither query.
	t.Cc[syscall.VMIN] = 0
	t.Cc[syscall.VTIME] = 3
	if ioctl(fd, syscall.TCSETS, unsafe.Pointer(&t)) != nil {
		return false, nil
	}
	defer ioctl(fd, syscall.TCSETS, unsafe.Pointer(&saved))

	out.WriteString(seqKittyQuery)
	out.Flush()

	buf := make([]byte, 0, 256)
	tmp := make([]byte, 128)
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, err := syscall.Read(fd, tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil || n == 0 {
			break
		}
		if i := indexDA1(buf); i >= 0 {
			// Everything before the DA1 reply is the answer window.
			return bytes.Contains(buf[:i], []byte("\x1b[?")) && kittyReplyBefore(buf[:i]),
				dropDA1(buf, i)
		}
	}
	return kittyReplyBefore(buf), nil
}

type winsize struct {
	Row, Col, X, Y uint16
}

func terminalSize(fd int) (w, h int) {
	var ws winsize
	if err := ioctl(fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil || ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

func isTTY(f *os.File) bool {
	var t syscall.Termios
	return ioctl(int(f.Fd()), syscall.TCGETS, unsafe.Pointer(&t)) == nil
}
