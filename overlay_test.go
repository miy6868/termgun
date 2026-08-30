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
	if g.debugPerf {
		t.Fatal("performance debugging should default off")
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
	for _, want := range []string{"> 포커스", "자동 일시 정지", "현재 시드 표시", "자동 무기 변경", "화면 흔들림", "화면 배율", "성능 디버깅 정보", "^/v", "<-/->"} {
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
	g.handleKey(Event{Kind: EvKey, Key: KeyDown})
	g.handleKey(Event{Kind: EvKey, Key: KeyRight})
	g.handleKey(Event{Kind: EvKey, Rune: 'o'})
	if g.autoPause || g.showSeed || g.autoWeapon || g.screenShake {
		t.Fatal("settings toggles did not turn off")
	}
	if !g.debugPerf {
		t.Fatal("performance debugging did not turn on")
	}
	if g.zoom != startZoom+1 {
		t.Fatalf("settings zoom is x%d, want x%d", g.zoom, startZoom+1)
	}
	g.perf.fps, g.perf.frameMS, g.perf.peakMS = 60, 16.67, 18.2
	g.perf.updateMS, g.perf.drawMS, g.perf.outputMS = 0.1, 0.2, 0.3
	g.Draw(s)
	text = overlayText(s)
	for _, want := range []string{"FPS", "FRAME", "UPD", "DRAW", "HEAP", "GC", "OBJ"} {
		if !strings.Contains(text, want) {
			t.Errorf("performance overlay does not show %q", want)
		}
	}

	g.state = StateDead
	g.handleKey(Event{Kind: EvKey, Rune: 'r'})
	if g.state != StatePlaying || g.autoPause || g.showSeed || g.autoWeapon || g.screenShake ||
		!g.debugPerf || g.zoom != startZoom+1 {
		t.Fatalf("restart lost settings: state=%v autoPause=%v showSeed=%v autoWeapon=%v "+
			"screenShake=%v debugPerf=%v zoom=%d", g.state, g.autoPause, g.showSeed,
			g.autoWeapon, g.screenShake, g.debugPerf, g.zoom)
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

func TestPauseToggleIgnoresKeyRepeat(t *testing.T) {
	g := newGameWithInput(85, InputKitty)
	g.state = StatePaused
	g.handleKey(Event{Kind: EvKey, Key: KeyEscape, KeyAct: KeyPress})
	if g.state != StatePlaying {
		t.Fatalf("ESC press left state %v, want playing", g.state)
	}
	g.handleKey(Event{Kind: EvKey, Key: KeyEscape, KeyAct: KeyRepeat})
	if g.state != StatePlaying {
		t.Fatalf("ESC repeat toggled state back to %v", g.state)
	}

	g.handleKey(Event{Kind: EvKey, Rune: 'p', KeyAct: KeyPress})
	if g.state != StatePaused {
		t.Fatalf("P press left state %v, want paused", g.state)
	}
	g.handleKey(Event{Kind: EvKey, Rune: 'p', KeyAct: KeyRepeat})
	if g.state != StatePaused {
		t.Fatalf("P repeat toggled state back to %v", g.state)
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

func TestDeviceReleaseInMenuDoesNotLeaveMovementStuck(t *testing.T) {
	g := newGameWithInput(93, InputDevice)
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyPress})
	if g.moveDir().X <= 0 {
		t.Fatal("device key did not start movement")
	}
	g.HandleEvent(Event{Kind: EvKey, Src: SrcTTY, Rune: 'o'})
	g.HandleEvent(Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyRelease})
	g.HandleEvent(Event{Kind: EvKey, Src: SrcTTY, Rune: 'o'})
	if d := g.moveDir(); d.len() != 0 {
		t.Fatalf("device release inside settings left movement stuck at %v", d)
	}
}

func TestModalMovementPressDoesNotArmPlayer(t *testing.T) {
	states := []struct {
		name  string
		state State
	}{
		{"paused", StatePaused},
		{"level-up", StateLevelUp},
		{"weapon-core", StateWeaponCore},
		{"help", StateHelp},
		{"settings", StateSettings},
		{"dead", StateDead},
		{"victory", StateVictory},
	}
	inputs := []struct {
		name  string
		mode  InputMode
		event Event
	}{
		{"terminal", InputKitty, Event{Kind: EvKey, Src: SrcTTY, Rune: 'd', KeyAct: KeyPress}},
		{"device", InputDevice, Event{Kind: EvKey, Src: SrcDevice, Dir: dirRight, KeyAct: KeyPress}},
	}

	for _, input := range inputs {
		for _, state := range states {
			t.Run(input.name+"/"+state.name, func(t *testing.T) {
				g := newGameWithInput(95, input.mode)
				g.state = state.state
				g.HandleEvent(input.event)
				g.state = StatePlaying
				if d := g.moveDir(); d.len() != 0 {
					t.Fatalf("movement pressed in modal state %v remained armed at %v", state.state, d)
				}
			})
		}
	}
}

func TestPausedGameplayInputsDoNotChangeResumeState(t *testing.T) {
	g := newGameWithInput(97, InputKitty)
	g.player.owned[wpPistol] = true
	g.player.ammo[wpPistol] = 10
	g.player.weapon = wpMelee
	g.state = StatePaused

	g.HandleEvent(Event{Kind: EvKey, Src: SrcTTY, Rune: '1', KeyAct: KeyPress})
	g.HandleEvent(Event{Kind: EvMouse, Action: MousePress, Button: BtnLeft})

	if g.player.weapon != wpMelee {
		t.Fatalf("paused weapon input selected weapon %d", g.player.weapon)
	}
	if g.firing || g.fireBuf > 0 {
		t.Fatal("paused mouse press armed firing for resume")
	}
}
