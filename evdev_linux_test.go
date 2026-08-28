//go:build linux

package main

import (
	"encoding/binary"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestEvdevDecoding feeds the exact byte layout the kernel produces through the
// reader. Getting the struct offsets wrong would silently mis-map every key, and
// that cannot be caught on a machine without /dev/input permissions.
func TestEvdevDecoding(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ch := make(chan Event, 32)
	go readEvdev(r, ch)

	write := func(typ, code uint16, value int32) {
		var b [inputEventSize]byte
		if inputEventTimevalSize == 8 {
			binary.NativeEndian.PutUint32(b[0:], 12345) // tv_sec
			binary.NativeEndian.PutUint32(b[4:], 678)   // tv_usec
		} else {
			binary.NativeEndian.PutUint64(b[0:], 12345) // tv_sec
			binary.NativeEndian.PutUint64(b[8:], 678)   // tv_usec
		}
		binary.NativeEndian.PutUint16(b[inputEventTypeOffset:], typ)
		binary.NativeEndian.PutUint16(b[inputEventCodeOffset:], code)
		binary.NativeEndian.PutUint32(b[inputEventValueOffset:], uint32(value))
		if _, err := w.Write(b[:]); err != nil {
			t.Error(err)
		}
	}

	write(evKey, keyD, 1) // press D
	write(evKey, keyD, 2) // auto-repeat
	write(evKey, keyW, 1) // press W
	write(evKey, keyD, 0) // release D
	write(0x02, 0, 1)     // EV_REL, must be ignored
	write(evKey, 0x2a, 1) // shift, not a movement key: ignored
	write(evKey, keyLeft, 1)

	want := []Event{
		{Dir: dirRight, KeyAct: KeyPress},
		{Dir: dirRight, KeyAct: KeyRepeat},
		{Dir: dirUp, KeyAct: KeyPress},
		{Dir: dirRight, KeyAct: KeyRelease},
		{Dir: dirLeft, KeyAct: KeyPress},
	}
	for i, exp := range want {
		select {
		case got := <-ch:
			if got.Kind != EvKey || got.Src != SrcDevice {
				t.Fatalf("event %d: got kind=%v src=%v", i, got.Kind, got.Src)
			}
			if got.Dir != exp.Dir || got.KeyAct != exp.KeyAct {
				t.Errorf("event %d: got dir=%d action=%v, want dir=%d action=%v",
					i, got.Dir, got.KeyAct, exp.Dir, exp.KeyAct)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d never arrived", i)
		}
	}
	w.Close()

	select {
	case ev, ok := <-ch:
		if ok {
			t.Errorf("unexpected extra event %+v", ev)
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestTerminationSignalStopsEventLoop(t *testing.T) {
	signals := make(chan os.Signal, 1)
	events := make(chan Event, 1)
	go forwardTermination(signals, events)
	signals <- syscall.SIGTERM

	select {
	case ev := <-events:
		if ev.Kind != EvStop {
			t.Fatalf("termination produced %v, want EvStop", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("termination did not stop the event loop")
	}
}

// TestTTYEOFLeavesSharedChannelOpen covers the channel ownership rule. The TTY
// reader can end while SIGWINCH or evdev still has an event to publish.
func TestTTYEOFLeavesSharedChannelOpen(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ch := make(chan Event, 2)
	go readTTYEvents(r, nil, ch)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Kind != EvStop {
			t.Fatalf("TTY EOF produced %v, want EvStop", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TTY EOF did not stop the event stream")
	}

	// A second producer must still be able to use the shared channel.
	ch <- Event{Kind: EvResize, W: 80, H: 24}
	if ev := <-ch; ev.Kind != EvResize {
		t.Fatalf("event after TTY EOF = %v, want EvResize", ev.Kind)
	}
}
