package main

import (
	"strings"
	"testing"
)

func overlayText(s *Screen) string {
	var b strings.Builder
	for y := 0; y < s.H; y++ {
		b.WriteString(screenRow(s, y))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHelpPagesReachableAtMinimumSize(t *testing.T) {
	g := NewGame(41)
	s := newTestScreen(minCols, minRows)
	g.handleKey(Event{Kind: EvKey, Rune: '?'})

	pages := [][]string{
		{"1/3", "W A S D", "[설정]"},
		{"2/3", "O 보스", "정예"},
		{"3/3", "0 폭발통", "퍼머데스"},
	}
	for i, wants := range pages {
		g.Draw(s)
		text := overlayText(s)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("help page %d at %dx%d does not show %q", i+1, minCols, minRows, want)
			}
		}
		if !strings.Contains(text, "페이지") {
			t.Errorf("help page %d navigation is clipped at %dx%d", i+1, minCols, minRows)
		}
		if i+1 < len(pages) {
			g.handleKey(Event{Kind: EvKey, Key: KeyRight})
		}
	}

	g.handleKey(Event{Kind: EvKey, Rune: 'x'})
	if g.state != StatePlaying {
		t.Fatalf("closing paged help left state %v", g.state)
	}
}

func TestSettingsToggleAndSurviveRestart(t *testing.T) {
	g := NewGame(71)
	s := newTestScreen(60, 20)
	if !g.autoPause || !g.showSeed || !g.autoWeapon || !g.screenShake {
		t.Fatal("settings should default on")
	}

	g.handleKey(Event{Kind: EvKey, Rune: '0'})
	if g.state == StateSettings {
		t.Fatal("0 still opens settings")
	}
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	if g.state != StateSettings {
		t.Fatalf("O opened state %v, want settings", g.state)
	}
	g.Draw(s)
	text := overlayText(s)
	for _, want := range []string{"> 포커스", "자동 일시 정지", "현재 시드 표시", "자동 무기 변경", "화면 흔들림", "화면 배율", "^/v", "<-/->"} {
		if !strings.Contains(text, want) {
			t.Errorf("settings page does not show %q", want)
		}
	}
	g.handleKey(Event{Kind: EvKey, Key: KeyUp})
	if g.settingRow != settingsCount-1 {
		t.Fatalf("up from the first setting focused row %d", g.settingRow)
	}
	g.handleKey(Event{Kind: EvKey, Key: KeyDown})
	if g.settingRow != 0 {
		t.Fatalf("down from the last setting focused row %d", g.settingRow)
	}

	startZoom := g.zoom
	for i := 0; i < 4; i++ {
		g.handleKey(Event{Kind: EvKey, Key: KeyLeft})
		g.handleKey(Event{Kind: EvKey, Key: KeyDown})
	}
	g.handleKey(Event{Kind: EvKey, Key: KeyRight})
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	if g.autoPause || g.showSeed || g.autoWeapon || g.screenShake {
		t.Fatal("settings toggles did not turn off")
	}
	if g.zoom != startZoom+1 {
		t.Fatalf("settings zoom is x%d, want x%d", g.zoom, startZoom+1)
	}

	g.state = StateDead
	g.handleKey(Event{Kind: EvKey, Rune: 'r'})
	if g.state != StatePlaying || g.autoPause || g.showSeed || g.autoWeapon || g.screenShake || g.zoom != startZoom+1 {
		t.Fatalf("restart lost settings: state=%v autoPause=%v showSeed=%v autoWeapon=%v screenShake=%v zoom=%d",
			g.state, g.autoPause, g.showSeed, g.autoWeapon, g.screenShake, g.zoom)
	}
}

func TestFocusAutoPauseCanBeDisabled(t *testing.T) {
	g := newGameWithInput(83, InputDevice)
	g.firing = true
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyPress})
	g.HandleEvent(Event{Kind: EvFocus, Focus: false})
	if g.state != StatePaused {
		t.Fatalf("focus loss left state %v, want paused", g.state)
	}
	if g.firing || g.moveDir().len() != 0 {
		t.Fatal("focus loss did not clear held movement and fire")
	}

	g.HandleEvent(Event{Kind: EvFocus, Focus: true})
	if g.state != StatePaused {
		t.Fatal("focus gain resumed the game without normal resume input")
	}
	g.HandleEvent(Event{Kind: EvKey, Rune: 'p'})
	if g.state != StatePlaying {
		t.Fatal("P did not resume an auto-paused game")
	}

	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	g.handleKey(Event{Kind: EvKey, Key: KeyLeft})
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	g.firing = true
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirLeft, KeyAct: KeyPress})
	g.HandleEvent(Event{Kind: EvFocus, Focus: false})
	if g.state != StatePlaying {
		t.Fatalf("disabled auto-pause changed state to %v", g.state)
	}
	if g.firing || g.moveDir().len() != 0 {
		t.Fatal("disabling auto-pause also disabled the input safety clear")
	}
}

func TestGameOverSeedVisibility(t *testing.T) {
	const seed = int64(424242)
	s := newTestScreen(60, 20)
	g := NewGame(seed)
	g.state = StateDead
	g.Draw(s)
	if text := overlayText(s); !strings.Contains(text, "시드") || !strings.Contains(text, "424242") {
		t.Fatalf("default game-over screen does not show the run seed:\n%s", text)
	}

	g = NewGame(seed)
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	g.handleKey(Event{Kind: EvKey, Key: KeyDown})
	g.handleKey(Event{Kind: EvKey, Key: KeyLeft})
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	g.state = StateDead
	g.Draw(s)
	if text := overlayText(s); strings.Contains(text, "시드") {
		t.Fatalf("disabled seed setting still renders a seed:\n%s", text)
	}
}

func TestDeviceInputLetsSettingsUseArrowKeys(t *testing.T) {
	g := newGameWithInput(91, InputDevice)
	g.HandleEvent(Event{Kind: EvKey, Rune: 'o', Src: SrcTTY})

	g.HandleEvent(Event{Kind: EvKey, Dir: dirDown, Src: SrcDevice})
	if g.settingRow != 0 || g.moveDir().len() != 0 {
		t.Fatal("device movement event changed settings focus or movement state")
	}
	g.HandleEvent(Event{Kind: EvKey, Key: KeyDown, Src: SrcTTY})
	if g.settingRow != 1 {
		t.Fatalf("terminal down key focused row %d in device mode, want 1", g.settingRow)
	}
}
