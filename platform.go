package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// platformSession joins the platform-specific terminal and input primitives
// into one lifecycle. The implementations remain selected by Go build tags.
type platformSession struct {
	in          *os.File
	fd          int
	out         *bufio.Writer
	state       *termState
	kitty       bool
	pending     []byte
	eventCh     chan Event
	termination inputSource
	startEvents sync.Once
	close       sync.Once
}

func openPlatform(in, output *os.File, inputMode string) (*platformSession, error) {
	if !isTTY(output) || !isTTY(in) {
		return nil, fmt.Errorf("이 게임은 대화형 터미널에서 실행해야 합니다.")
	}

	fd := int(in.Fd())
	state, err := enterRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("raw 모드 진입 실패: %w", err)
	}

	s := &platformSession{
		in:      in,
		fd:      fd,
		out:     bufio.NewWriterSize(output, 1<<16),
		state:   state,
		eventCh: make(chan Event, 256),
	}
	// Register before probing terminal capabilities. A termination signal during
	// that probe must be queued until runGame can restore the raw terminal.
	s.termination = watchTermination(s.eventCh)

	// Probe before switching screens, so a terminal that echoes the query does
	// not leave stray text on the game screen.
	if inputMode == "auto" || inputMode == "kitty" {
		s.kitty, s.pending = detectKitty(fd, s.out)
	}

	s.out.WriteString(seqEnterAlt + seqHideCursor + seqClear + inputEnableSeq)
	if s.kitty {
		s.out.WriteString(seqKittyPush)
	}
	s.out.Flush()
	return s, nil
}

func (s *platformSession) Close() {
	s.close.Do(func() {
		if s.termination != nil {
			s.termination.Close()
		}
		if s.kitty {
			s.out.WriteString(seqKittyPop)
		}
		s.out.WriteString(inputDisableSeq + seqShowCursor + seqReset + seqLeaveAlt)
		s.out.Flush()
		s.state.restore()
	})
}

func (s *platformSession) size() (int, int) {
	return terminalSize(s.fd)
}

func (s *platformSession) events() <-chan Event {
	s.startEvents.Do(func() {
		readEvents(s.in, s.fd, s.pending, s.eventCh)
	})
	return s.eventCh
}

func (s *platformSession) input(inputMode string) (InputMode, inputSource, string) {
	return chooseInput(inputMode, s.kitty, s.eventCh)
}
