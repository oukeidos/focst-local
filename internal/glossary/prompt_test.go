package glossary

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/language"
)

func TestRenderUserPromptFrozenKorean(t *testing.T) {
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	got, err := RenderUserPrompt(src, tgt, []Segment{{ID: 1, SourceText: "架空田一郎です"}})
	if err != nil {
		t.Fatalf("RenderUserPrompt failed: %v", err)
	}
	want := `Extract proper nouns from the provided Japanese subtitle script and provide their Korean renderings.

Return a Markdown table with exactly these columns:

| Source | Korean rendering |

Rules:
- Use exact source expressions from the script.
- For names, "Korean rendering" must be a Korean name form, not a translation of the characters' meanings.
- For specialized terms, do not translate word by word when an established Korean rendering exists; use the conventional Korean term instead.
- Do not repeat the same source expression; output at most one row for each distinct source value.
- Do not wrap the table in a code block.
- Do not include explanations before or after the table.
- If there are no useful entries, output a Markdown table with only the header row.

Input JSON:
{"segments":[{"id":1,"source_text":"架空田一郎です"}]}`
	if got != want {
		t.Fatalf("prompt drifted\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestSystemPromptFrozen(t *testing.T) {
	if SystemPrompt != "You extract glossary entries for subtitle translation." {
		t.Fatalf("SystemPrompt drifted: %q", SystemPrompt)
	}
}

func TestRenderUserPromptDoesNotReintroduceDiscardedFields(t *testing.T) {
	src, _ := language.GetLanguage("ja")
	tgt, _ := language.GetLanguage("ko")
	got, err := RenderUserPrompt(src, tgt, []Segment{{ID: 1, SourceText: "架空田一郎です"}})
	if err != nil {
		t.Fatalf("RenderUserPrompt failed: %v", err)
	}
	for _, forbidden := range []string{
		"Evidence IDs",
		"evidence_ids",
		"Decide each rendering from the script context in this same request.",
		"confidence",
		"reason",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt contains discarded element %q:\n%s", forbidden, got)
		}
	}
}
