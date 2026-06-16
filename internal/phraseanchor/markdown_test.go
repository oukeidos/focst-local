package phraseanchor

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/translation"
)

func TestRenderCandidateDiscoveryPromptUsesDynamicLanguages(t *testing.T) {
	got := RenderCandidateDiscoveryPrompt(1, 2, `{"target":[]}`, "English", "Korean")
	if !strings.Contains(got, "English-to-Korean subtitle translation") {
		t.Fatalf("prompt does not contain dynamic language pair:\n%s", got)
	}
	if strings.Contains(got, "Japanese-to-Korean") {
		t.Fatalf("prompt still contains hard-coded language pair:\n%s", got)
	}
	if !strings.Contains(got, "| Segment ID | Source text | Type | Source quote | Korean rendering |") {
		t.Fatalf("prompt does not contain Korean rendering header:\n%s", got)
	}
	if strings.Contains(got, "Target rendering") {
		t.Fatalf("prompt still contains generic target rendering header:\n%s", got)
	}
	englishTarget := RenderCandidateDiscoveryPrompt(1, 2, `{"target":[]}`, "Japanese", "English")
	if !strings.Contains(englishTarget, "| Segment ID | Source text | Type | Source quote | English rendering |") {
		t.Fatalf("prompt does not contain dynamic English rendering header:\n%s", englishTarget)
	}
}

func TestParseCandidateTableRepairsSegmentIDFromExactSourceText(t *testing.T) {
	window := Window{
		Index: 1,
		Target: []translation.SegmentData{
			{ID: 10, SourceText: "alpha one"},
			{ID: 11, SourceText: "beta two"},
		},
	}
	content := `| Segment ID | Source text | Type | Source quote | Korean rendering |
| ---: | --- | --- | --- | --- |
| 10 | beta two | idiom | beta | 베타 |`

	got, rejected := parseCandidateTable(content, window, CandidateDiscoveryName, "Korean")
	if len(rejected) != 0 {
		t.Fatalf("unexpected rejected rows: %+v", rejected)
	}
	if len(got) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(got))
	}
	if got[0].SegmentID != 11 || got[0].SourceText != "beta two" {
		t.Fatalf("candidate was not repaired from exact source text: %+v", got[0])
	}
}

func TestParseCandidateTableRepairsSourceTextFromSegmentID(t *testing.T) {
	window := Window{
		Index: 1,
		Target: []translation.SegmentData{
			{ID: 10, SourceText: "alpha one"},
		},
	}
	content := `| Segment ID | Source text | Type | Source quote | Korean rendering |
| ---: | --- | --- | --- | --- |
| 10 | stale text | ambiguity | alpha | 알파 |`

	got, rejected := parseCandidateTable(content, window, CandidateDiscoveryName, "Korean")
	if len(rejected) != 0 {
		t.Fatalf("unexpected rejected rows: %+v", rejected)
	}
	if len(got) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(got))
	}
	if got[0].SourceText != "alpha one" {
		t.Fatalf("candidate source text was not repaired: %+v", got[0])
	}
}

func TestParseAlternativeTableRepairsIdentityFieldsFromInputCandidate(t *testing.T) {
	input := Candidate{
		SegmentID:         20,
		SourceText:        "gamma phrase",
		Type:              TypeWordplay,
		SourceQuote:       "gamma",
		Rendering:         "감마",
		PhraseWindowIndex: 3,
	}
	content := `| Segment ID | Source text | Type | Source quote | Korean rendering | Alternative Rendering |
| ---: | --- | --- | --- | --- | --- |
| 999 | wrong text | idiom | wrong | wrong rendering | 다른 감마 |`

	got, rejected := parseAlternativeTable(content, input, "Korean")
	if len(rejected) != 0 {
		t.Fatalf("unexpected rejected rows: %+v", rejected)
	}
	if got.Candidate.SegmentID != input.SegmentID ||
		got.Candidate.SourceText != input.SourceText ||
		got.Candidate.Type != input.Type ||
		got.Candidate.SourceQuote != input.SourceQuote ||
		got.Candidate.Rendering != input.Rendering {
		t.Fatalf("alternative candidate identity was not repaired: %+v", got.Candidate)
	}
	if got.AlternativeRendering != "다른 감마" {
		t.Fatalf("alternative rendering = %q, want %q", got.AlternativeRendering, "다른 감마")
	}
}
