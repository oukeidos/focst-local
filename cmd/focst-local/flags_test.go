package main

import (
	"strings"
	"testing"
)

func TestOverwriteFlag_AcceptsYesAndShorthand(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "root_shorthand", args: []string{"-y"}},
		{name: "root_long", args: []string{"--yes"}},
		{name: "names_shorthand", args: []string{"names", "-y"}},
		{name: "names_long", args: []string{"names", "--yes"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := executeCommand(t, tc.args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown shorthand flag: 'y'") || strings.Contains(out, "unknown flag: --yes") {
				t.Fatalf("expected --yes/-y to be parsed, got output: %s", out)
			}
		})
	}
}

func TestOverwriteFlag_RejectsDeprecatedLongY(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "root_deprecated_long_y", args: []string{"--y"}},
		{name: "names_deprecated_long_y", args: []string{"names", "--y"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := executeCommand(t, tc.args...)
			if err == nil {
				t.Fatalf("expected unknown flag error for --y")
			}
			if !strings.Contains(out, "unknown flag: --y") {
				t.Fatalf("expected unknown flag: --y, got output: %s", out)
			}
		})
	}
}

func TestSentenceAwareChunkFlags_Parse(t *testing.T) {
	out, err := executeCommand(t,
		"translate",
		"--no-sentence-aware-chunks",
		"--min-chunk-size", "90",
		"--max-chunk-size", "110",
		"--chunk-boundary-planner", "deterministic",
	)
	if err == nil {
		t.Fatalf("expected command error from missing required args, got nil")
	}
	for _, unexpected := range []string{
		"unknown flag: --no-sentence-aware-chunks",
		"unknown flag: --min-chunk-size",
		"unknown flag: --max-chunk-size",
		"unknown flag: --chunk-boundary-planner",
	} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("expected sentence-aware flag to parse, got output: %s", out)
		}
	}
}
