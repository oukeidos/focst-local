package glossary

import "testing"

func TestParseMarkdownTable(t *testing.T) {
	got := ParseMarkdownTable(`| Source | Korean rendering |
| --- | --- |
| 架空田一郎 | 가공다 이치로 |
| 合成子花 | 합성자 하나 |`, "Korean rendering", 2, 3)
	if len(got.Violations) != 0 {
		t.Fatalf("violations = %+v", got.Violations)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates len = %d", len(got.Candidates))
	}
	if got.Candidates[0].Source != "架空田一郎" || got.Candidates[0].Rendering != "가공다 이치로" {
		t.Fatalf("first candidate = %+v", got.Candidates[0])
	}
	if got.Candidates[0].WindowIndex != 2 || got.Candidates[0].RunIndex != 3 {
		t.Fatalf("candidate indexes = %+v", got.Candidates[0])
	}
}

func TestParseMarkdownTableRecordsFenceViolation(t *testing.T) {
	got := ParseMarkdownTable("```markdown\n| Source | Korean rendering |\n| --- | --- |\n| A | 에이 |\n```", "Korean rendering", 0, 1)
	if len(got.Candidates) != 1 {
		t.Fatalf("candidates len = %d", len(got.Candidates))
	}
	if len(got.Violations) == 0 {
		t.Fatalf("expected code fence violation")
	}
}

func TestParseMarkdownTableRejectsUnexpectedHeader(t *testing.T) {
	got := ParseMarkdownTable(`| Source | Translation |
| --- | --- |
| A | 에이 |`, "Korean rendering", 0, 1)
	if len(got.Candidates) != 0 {
		t.Fatalf("expected no candidates for unexpected header, got %+v", got.Candidates)
	}
	if len(got.Violations) == 0 {
		t.Fatalf("expected header violation")
	}
}
