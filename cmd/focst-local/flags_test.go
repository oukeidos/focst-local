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

func TestPostPolishFlags_Parse(t *testing.T) {
	cases := [][]string{
		{"--post-polish"},
		{"--post-polish-profile", "segment-local"},
		{"--post-polish-profile", "chunk-flow"},
		{"--post-polish-profile", "legacy"},
		{"--save-polish-corrections", "out.polish.json"},
		{"--polish-artifacts", "out.polish"},
		{"--polish-broad-chunk-size", "30"},
		{"--polish-repair-chunk-size", "100"},
		{"--polish-max-tokens", "2048"},
		{"--polish-chunk-size", "8"},
		{"--polish-min-chunk-size", "5"},
		{"--polish-max-chunk-size", "9"},
		{"--no-polish-sentence-aware-chunks"},
		{"--polish-chunk-boundary-planner", "deterministic"},
		{"translate", "--post-polish"},
		{"translate", "--post-polish-profile", "chunk-flow"},
		{"translate", "--polish-chunk-size", "8"},
		{"polish", "--post-polish-profile", "segment-local"},
		{"polish", "--post-polish-profile", "chunk-flow"},
		{"polish", "--post-polish-profile", "legacy"},
		{"polish", "--polish-broad-chunk-size", "30"},
		{"polish", "--polish-repair-chunk-size", "100"},
		{"polish", "--polish-max-tokens", "2048"},
		{"polish", "--polish-chunk-size", "8"},
		{"polish", "--polish-min-chunk-size", "5"},
		{"polish", "--polish-max-chunk-size", "9"},
		{"polish", "--no-polish-sentence-aware-chunks"},
		{"polish", "--polish-chunk-boundary-planner", "deterministic"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := executeCommand(t, args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown flag") {
				t.Fatalf("expected post-polish flag to parse, got output: %s", out)
			}
		})
	}
}

func TestPostPolishFlags_RejectInvalidProfile(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--post-polish-profile", "auto",
	)
	if err == nil {
		t.Fatalf("expected invalid profile error")
	}
	if !strings.Contains(err.Error(), "invalid post-polish profile: auto") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostPolishFlags_RejectLegacyChunkSizesForV2Profiles(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--post-polish",
		"--post-polish-profile", "segment-local",
		"--polish-broad-chunk-size", "20",
	)
	if err == nil {
		t.Fatalf("expected legacy chunk size error")
	}
	if !strings.Contains(err.Error(), "only supported with --post-polish-profile legacy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostPolishFlags_RejectSaveWithoutPostPolish(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--save-polish-corrections", "out.polish.json",
	)
	if err == nil {
		t.Fatalf("expected save-polish-corrections dependency error")
	}
	if !strings.Contains(err.Error(), "--save-polish-corrections requires --post-polish") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResidueFlags_Parse(t *testing.T) {
	cases := [][]string{
		{"--repair-residue", "--residue-scripts", "hiragana,katakana"},
		{"--repair-residue", "--residue-scripts", "auto"},
		{"--repair-residue", "--save-residue-candidates", "residue.json"},
		{"--repair-residue", "--residue-report", "residue.md"},
		{"translate", "--repair-residue", "--residue-scripts", "hiragana"},
		{"residue", "detect", "--residue-scripts", "hiragana"},
		{"residue", "detect", "--save-residue-candidates", "residue.json"},
		{"residue", "detect", "--residue-report", "residue.md"},
		{"residue", "repair", "--residue-scripts", "hiragana"},
		{"residue", "repair", "--residue-candidates", "residue.json"},
		{"residue", "repair", "--max-tokens", "1024"},
		{"residue", "repair", "--glossary-file", "glossary.json"},
		{"residue", "repair", "--names", "names.json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := executeCommand(t, args...)
			if err == nil {
				t.Fatalf("expected command error from missing required args, got nil")
			}
			if strings.Contains(out, "unknown flag") {
				t.Fatalf("expected residue flag to parse, got output: %s", out)
			}
		})
	}
}

func TestResidueFlags_RejectSaveWithoutRepairResidue(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--save-residue-candidates", "residue.json",
	)
	if err == nil {
		t.Fatalf("expected save-residue-candidates dependency error")
	}
	if !strings.Contains(err.Error(), "--save-residue-candidates requires --repair-residue") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResidueFlags_RejectRepairWithoutScripts(t *testing.T) {
	_, err := executeCommand(t,
		"translate",
		"input.srt",
		"output.srt",
		"--repair-residue",
	)
	if err == nil {
		t.Fatalf("expected repair-residue dependency error")
	}
	if !strings.Contains(err.Error(), "--repair-residue requires --residue-scripts") {
		t.Fatalf("unexpected error: %v", err)
	}
}
