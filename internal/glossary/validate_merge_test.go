package glossary

import "testing"

func TestValidateCandidatesComputesLocalOccurrenceIDs(t *testing.T) {
	segments := []Segment{
		{ID: 10, SourceText: "架空田一郎が登場する"},
		{ID: 11, SourceText: "一郎は合成都市へ行く"},
	}
	accepted, rejected := ValidateCandidates([]Candidate{
		{Source: "一郎", Rendering: "이치로", WindowIndex: 0, RunIndex: 1},
		{Source: "存在しない", Rendering: "없음", WindowIndex: 0, RunIndex: 1},
	}, segments)
	if len(accepted) != 1 {
		t.Fatalf("accepted len = %d", len(accepted))
	}
	if len(rejected) != 1 {
		t.Fatalf("rejected len = %d", len(rejected))
	}
	if got := accepted[0].OccurrenceIDs; len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Fatalf("occurrence ids = %+v", got)
	}
}

func TestMergeVotingDeterministicTieBreak(t *testing.T) {
	segments := []Segment{{ID: 1, SourceText: "合成星団"}}
	entries := Merge([]ValidatedCandidate{
		{Candidate: Candidate{Source: "合成星団", Rendering: "합성 스타 클러스터", WindowIndex: 0}, CanonicalSource: "合成星団"},
		{Candidate: Candidate{Source: "合成星団", Rendering: "합성단", WindowIndex: 0}, CanonicalSource: "合成星団"},
	}, segments)
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	// Same vote count and same window count: shorter rendering wins.
	if entries[0].Rendering != "합성단" {
		t.Fatalf("rendering = %q", entries[0].Rendering)
	}
	if entries[0].Confidence != ConfidenceLow {
		t.Fatalf("confidence = %q", entries[0].Confidence)
	}
}
