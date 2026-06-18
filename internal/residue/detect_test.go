package residue

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/srt"
)

func TestDetectSelectedScriptResidue(t *testing.T) {
	source := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"今日は になって 話した"}},
		{ID: 2, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"ノー と 言った"}},
		{ID: 3, StartTime: "00:00:05,000", EndTime: "00:00:06,000", Lines: []string{"2026 年 の 話"}},
	}
	target := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"오늘 になって 말했다"}},
		{ID: 2, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"노라고 말했다"}},
		{ID: 3, StartTime: "00:00:05,000", EndTime: "00:00:06,000", Lines: []string{"2026년에 말했다"}},
	}
	artifact, err := Detect(source, target, DetectOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
		ScriptSpec:     "hiragana,katakana",
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(artifact.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %d: %#v", len(artifact.Candidates), artifact.Candidates)
	}
	got := artifact.Candidates[0]
	if got.ID != 1 {
		t.Fatalf("expected ID 1, got %d", got.ID)
	}
	if got.FilteredTargetText != "になって" {
		t.Fatalf("unexpected filtered target: %q", got.FilteredTargetText)
	}
}

func TestDetectAutoSelectsSourceDominantResidueScript(t *testing.T) {
	source := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"あいうえお かきくけこ さしすせそ ABC"}},
		{ID: 2, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"たちつてと なにぬねの"}},
	}
	target := []srt.Segment{
		{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{strings.Repeat("한국어문장", 20) + " あい"}},
		{ID: 2, StartTime: "00:00:03,000", EndTime: "00:00:04,000", Lines: []string{"한국어 문장"}},
	}
	artifact, err := Detect(source, target, DetectOptions{
		SourceLanguage: language.Languages["ja"],
		TargetLanguage: language.Languages["ko"],
		ScriptSpec:     AutoScripts,
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(artifact.Config.SelectedScripts) != 1 || artifact.Config.SelectedScripts[0] != "Hiragana" {
		t.Fatalf("expected auto Hiragana, got %#v", artifact.Config.SelectedScripts)
	}
	if len(artifact.Candidates) != 1 || artifact.Candidates[0].ID != 1 {
		t.Fatalf("expected one residue candidate for ID 1, got %#v", artifact.Candidates)
	}
}

func TestParseScriptListNormalizesNames(t *testing.T) {
	scripts, auto, err := ParseScriptList("old-hungarian, katakana")
	if err != nil {
		t.Fatalf("ParseScriptList failed: %v", err)
	}
	if auto {
		t.Fatalf("expected explicit scripts, got auto")
	}
	got := scriptNames(scripts)
	want := []string{"Katakana", "Old_Hungarian"}
	if len(got) != len(want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
}
