package glossary

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
)

func TestRenderRenderingSafetyPromptFrozenKorean(t *testing.T) {
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	got := RenderRenderingSafetyPrompt(src, tgt, []renderingSafetyRow{{
		Row:       1,
		Source:    "紙雲係",
		Rendering: "카미구모가카리",
		Occurrences: []Segment{
			{ID: 47, SourceText: "あの紙雲係"},
		},
	}})
	want := `Review proposed glossary entries for Japanese-to-Korean subtitle translation.

Each input row is a generated global glossary entry. First decide the expected rendering strategy for the source expression in this subtitle context. Then judge whether the proposed rendering fits that expected strategy.

Choose exactly one Expected strategy:
- name_form: the source identifies a specific person, character, place, organization, product, work, or other unique entity, and should be rendered as that entity's natural Korean name.
- meaning_translation: the source does not identify a unique entity; its meaning, description, role, nickname, or wording should be translated rather than sounded out.
- conventional_term: the source is a fixed public, cultural, or specialized term with a recognized Korean equivalent; ordinary words and routine dialogue are not this category.
- context_dependent: the source should not be globally fixed because the same source text can require different valid renderings in different subtitle contexts.

Choose exactly one Fit:
- fits: the proposed rendering fits the expected strategy.
- uncertain: the fit is uncertain.
- mismatch: the proposed rendering clearly does not fit the expected strategy.

Choose exactly one Decision:
- keep: the proposed rendering fits well enough for global glossary injection.
- review: uncertain, or potentially useful but not safe to trust blindly.
- reject: the proposed rendering is clearly unsafe as a global glossary entry.

Rules:
- Expected strategy describes the source expression, not the proposed rendering.
- Fit compares the proposed rendering against the expected strategy.
- Judge only the proposed rendering. Do not create replacement renderings.
- Return only a Markdown table with exactly this header:

| Row | Expected strategy | Fit | Decision |

Input rows:

| Row | Source | Proposed rendering | Occurrence snippets |
| ---: | --- | --- | --- |
| 1 | 紙雲係 | 카미구모가카리 | 47: あの紙雲係 |
`
	if got != want {
		t.Fatalf("prompt drifted\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestRenderRenderingSafetyPromptDoesNotIncludeRemovedRules(t *testing.T) {
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	got := RenderRenderingSafetyPrompt(src, tgt, nil)
	for _, forbidden := range []string{
		"Do not reject a true person",
		"- preserve:",
		"preserved as-is",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt contains forbidden text %q:\n%s", forbidden, got)
		}
	}
}

func TestParseRenderingSafetyTableAndSourcePolicy(t *testing.T) {
	rows := []renderingSafetyRow{
		{Row: 1, Source: "紙雲係", Rendering: "카미구모가카리"},
		{Row: 2, Source: "架空田一郎", Rendering: "가공다 이치로"},
		{Row: 3, Source: "House", Rendering: "하원"},
	}
	content := `| Row | Expected strategy | Fit | Decision |
| ---: | --- | --- | --- |
| 1 | meaning_translation | mismatch | reject |
| 2 | name_form | mismatch | reject |
| 3 | context_dependent | mismatch | review |
`
	judgments, violations := ParseRenderingSafetyTable(content, rows)
	if len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
	if got := judgments[0].SourcePolicyResult; got != SourcePolicyDrop {
		t.Fatalf("row 1 policy = %s, want drop", got)
	}
	if got := judgments[1].SourcePolicyResult; got != SourcePolicyKeep {
		t.Fatalf("row 2 policy = %s, want keep", got)
	}
	if got := judgments[1].SourcePolicyReason; got != SourcePolicyReasonKeepNameForm {
		t.Fatalf("row 2 reason = %s, want %s", got, SourcePolicyReasonKeepNameForm)
	}
	if got := judgments[2].SourcePolicyResult; got != SourcePolicyKeep {
		t.Fatalf("row 3 policy = %s, want keep", got)
	}
}

func TestParseRenderingSafetyTableRejectsBadOutput(t *testing.T) {
	rows := []renderingSafetyRow{{Row: 1, Source: "紙雲係", Rendering: "카미구모가카리"}}
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "bad header",
			content: `| Row | Policy | Decision |
| --- | --- | --- |
| 1 | meaning_translation | reject |
`,
		},
		{
			name: "unknown strategy",
			content: `| Row | Expected strategy | Fit | Decision |
| --- | --- | --- | --- |
| 1 | preserve | fits | keep |
`,
		},
		{
			name: "missing row",
			content: `| Row | Expected strategy | Fit | Decision |
| --- | --- | --- | --- |
`,
		},
		{
			name: "duplicate row",
			content: `| Row | Expected strategy | Fit | Decision |
| --- | --- | --- | --- |
| 1 | meaning_translation | mismatch | reject |
| 1 | meaning_translation | mismatch | reject |
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, violations := ParseRenderingSafetyTable(tt.content, rows)
			if len(violations) == 0 {
				t.Fatalf("expected violations")
			}
		})
	}
}
