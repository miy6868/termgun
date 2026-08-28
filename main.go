package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

type appConfig struct {
	seed      int64
	fps       int
	zoom      int
	inputMode string
}

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		os.Exit(2)
	}
	if err := runGame(cfg, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig(args []string, output io.Writer) (appConfig, error) {
	fs := flag.NewFlagSet("termgun", flag.ContinueOnError)
	fs.SetOutput(output)
	cfg := appConfig{fps: 60, zoom: defaultZoom, inputMode: "auto"}
	fs.Int64Var(&cfg.seed, "seed", 0, "던전 시드 (0이면 현재 시각)")
	fs.IntVar(&cfg.fps, "fps", cfg.fps, "목표 프레임 레이트")
	fs.IntVar(&cfg.zoom, "zoom", cfg.zoom,
		fmt.Sprintf("화면 배율: 월드 타일 하나를 터미널 칸 몇 개로 그릴지 (%d~%d)", minZoom, maxZoom))
	fs.StringVar(&cfg.inputMode, "input", cfg.inputMode,
		"키보드 입력 방식: auto | kitty | device | compat\n"+
			"  device 는 /dev/input 을 직접 읽습니다 (Linux, input 그룹 권한 필요)\n"+
			"  Windows 콘솔은 키 상태를 직접 알려주므로 이 옵션이 필요 없습니다")
	if err := fs.Parse(args); err != nil {
		return appConfig{}, err
	}
	if err := cfg.validate(); err != nil {
		fmt.Fprintln(output, "설정 오류:", err)
		fs.Usage()
		return appConfig{}, err
	}
	return cfg, nil
}

func (c appConfig) validate() error {
	if c.fps < minFPS || c.fps > maxFPS {
		return fmt.Errorf("-fps는 %d~%d 범위여야 합니다", minFPS, maxFPS)
	}
	if c.zoom < minZoom || c.zoom > maxZoom {
		return fmt.Errorf("-zoom은 %d~%d 범위여야 합니다", minZoom, maxZoom)
	}
	switch c.inputMode {
	case "auto", "kitty", "device", "compat":
		return nil
	default:
		return fmt.Errorf("-input은 auto, kitty, device, compat 중 하나여야 합니다")
	}
}
