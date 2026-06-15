package glossary

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

func ValidateCandidates(candidates []Candidate, segments []Segment) ([]ValidatedCandidate, []RejectedCandidate) {
	seen := make(map[string]bool)
	accepted := make([]ValidatedCandidate, 0, len(candidates))
	rejected := make([]RejectedCandidate, 0)
	for _, candidate := range candidates {
		source := collapseSpaces(candidate.Source)
		rendering := strings.TrimSpace(candidate.Rendering)
		if source == "" || rendering == "" {
			rejected = append(rejected, rejectCandidate(candidate, "empty source or rendering"))
			continue
		}
		key := source + "\x00" + rendering + "\x00" + itoa(candidate.WindowIndex) + "\x00" + itoa(candidate.RunIndex)
		if seen[key] {
			continue
		}
		seen[key] = true
		ids := OccurrenceIDs(source, segments)
		if len(ids) == 0 {
			rejected = append(rejected, rejectCandidate(candidate, "source expression not found in normalized subtitles"))
			continue
		}
		candidate.Source = source
		candidate.Rendering = rendering
		accepted = append(accepted, ValidatedCandidate{
			Candidate:       candidate,
			CanonicalSource: source,
			OccurrenceIDs:   ids,
		})
	}
	return accepted, rejected
}

func OccurrenceIDs(source string, segments []Segment) []int {
	source = collapseSpaces(source)
	if source == "" {
		return nil
	}
	normSource := norm.NFKC.String(source)
	var ids []int
	for _, segment := range segments {
		text := collapseSpaces(segment.SourceText)
		if strings.Contains(text, source) || strings.Contains(norm.NFKC.String(text), normSource) {
			ids = append(ids, segment.ID)
		}
	}
	return ids
}

func rejectCandidate(candidate Candidate, reason string) RejectedCandidate {
	return RejectedCandidate{
		Source:      candidate.Source,
		Rendering:   candidate.Rendering,
		Reason:      reason,
		WindowIndex: candidate.WindowIndex,
		RunIndex:    candidate.RunIndex,
	}
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	n := v
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
