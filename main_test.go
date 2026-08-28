package main

import (
	"io"
	"testing"
)

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	cases := [][]string{
		{"-fps", "14"},
		{"-fps", "1001"},
		{"-zoom", "0"},
		{"-zoom", "5"},
		{"-input", "unknown"},
	}
	for _, args := range cases {
		if _, err := parseConfig(args, io.Discard); err == nil {
			t.Errorf("parseConfig(%v) accepted an invalid value", args)
		}
	}
}

func TestParseConfigAcceptsSupportedValues(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-seed", "42", "-fps", "30", "-zoom", "3", "-input", "device",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.seed != 42 || cfg.fps != 30 || cfg.zoom != 3 || cfg.inputMode != "device" {
		t.Fatalf("parsed config = %+v", cfg)
	}
}
