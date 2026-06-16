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

func TestTranslationTimeoutFlag_Parse(t *testing.T) {
	cases := [][]string{
		{"--translation-timeout", "0"},
		{"--translation-timeout", "30m"},
		{"translate", "--translation-timeout", "30m"},
		{"repair", "--translation-timeout", "30m"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := executeCommand(t, args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown flag: --translation-timeout") {
				t.Fatalf("expected --translation-timeout to parse, got output: %s", out)
			}
		})
	}
}

func TestGlossaryFlags_Parse(t *testing.T) {
	cases := [][]string{
		{"--auto-glossary"},
		{"--save-glossary", "out.glossary.json"},
		{"--glossary-file", "existing.glossary.json"},
		{"--glossary-artifacts", "out.glossary"},
		{"--glossary-runs", "10"},
		{"--glossary-window-chunks", "4"},
		{"translate", "--auto-glossary"},
		{"glossary", "extract", "--glossary-runs", "10"},
		{"glossary", "extract", "--glossary-window-chunks", "4"},
		{"glossary", "extract", "--glossary-artifacts", "artifacts"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := executeCommand(t, args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown flag") {
				t.Fatalf("expected glossary flag to parse, got output: %s", out)
			}
		})
	}
}

func TestGlossaryFlags_RejectAmbiguousAutoAndFile(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--auto-glossary",
		"--glossary-file", "existing.glossary.json",
	)
	if err == nil {
		t.Fatalf("expected ambiguous glossary flag error")
	}
	if !strings.Contains(err.Error(), "--auto-glossary and --glossary-file cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPhraseAnchorFlags_Parse(t *testing.T) {
	cases := [][]string{
		{"--auto-phrase-anchors"},
		{"--save-phrase-anchors", "out.phrase-anchors.json"},
		{"--phrase-anchors-file", "existing.phrase-anchors.json"},
		{"--phrase-anchors-artifacts", "out.phrase-anchors"},
		{"--phrase-anchor-thesis-rounds", "3"},
		{"--phrase-anchor-votes", "5"},
		{"--phrase-anchor-quote-filter-batch-size", "80"},
		{"--phrase-anchor-proper-filter-runs", "3"},
		{"--phrase-anchor-proper-filter-window-chunks", "3"},
		{"translate", "--auto-phrase-anchors"},
		{"phrase-anchors", "extract", "--phrase-anchor-thesis-rounds", "3"},
		{"phrase-anchors", "extract", "--phrase-anchor-votes", "5"},
		{"phrase-anchors", "extract", "--phrase-anchors-artifacts", "artifacts"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := executeCommand(t, args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown flag") {
				t.Fatalf("expected phrase anchor flag to parse, got output: %s", out)
			}
		})
	}
}

func TestPhraseAnchorFlags_RejectAmbiguousAutoAndFile(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--auto-phrase-anchors",
		"--phrase-anchors-file", "existing.phrase-anchors.json",
	)
	if err == nil {
		t.Fatalf("expected ambiguous phrase anchors flag error")
	}
	if !strings.Contains(err.Error(), "--auto-phrase-anchors and --phrase-anchors-file cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}
