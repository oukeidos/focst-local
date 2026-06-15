package glossary

import (
	"sort"
)

func Merge(candidates []ValidatedCandidate, allSegments []Segment) []Entry {
	type sourceBucket struct {
		votes           map[string]int
		renderingWindow map[string]map[int]bool
		windows         map[int]bool
	}
	buckets := make(map[string]*sourceBucket)
	for _, candidate := range candidates {
		source := candidate.CanonicalSource
		if source == "" {
			source = candidate.Source
		}
		bucket := buckets[source]
		if bucket == nil {
			bucket = &sourceBucket{
				votes:           make(map[string]int),
				renderingWindow: make(map[string]map[int]bool),
				windows:         make(map[int]bool),
			}
			buckets[source] = bucket
		}
		bucket.votes[candidate.Rendering]++
		if bucket.renderingWindow[candidate.Rendering] == nil {
			bucket.renderingWindow[candidate.Rendering] = make(map[int]bool)
		}
		bucket.renderingWindow[candidate.Rendering][candidate.WindowIndex] = true
		bucket.windows[candidate.WindowIndex] = true
	}

	sources := make([]string, 0, len(buckets))
	for source := range buckets {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	entries := make([]Entry, 0, len(sources))
	for _, source := range sources {
		bucket := buckets[source]
		rendering := chooseRendering(bucket.votes, bucket.renderingWindow)
		totalVotes := 0
		for _, count := range bucket.votes {
			totalVotes += count
		}
		winnerVotes := bucket.votes[rendering]
		entries = append(entries, Entry{
			Source:        source,
			Rendering:     rendering,
			Confidence:    confidence(winnerVotes, totalVotes),
			Votes:         copyVotes(bucket.votes),
			OccurrenceIDs: OccurrenceIDs(source, allSegments),
			WindowsSeen:   sortedKeys(bucket.windows),
		})
	}
	return entries
}

func chooseRendering(votes map[string]int, windows map[string]map[int]bool) string {
	var best string
	for rendering, count := range votes {
		if best == "" {
			best = rendering
			continue
		}
		bestCount := votes[best]
		switch {
		case count > bestCount:
			best = rendering
		case count < bestCount:
		case len(windows[rendering]) > len(windows[best]):
			best = rendering
		case len(windows[rendering]) < len(windows[best]):
		case len(rendering) < len(best):
			best = rendering
		case len(rendering) > len(best):
		case rendering < best:
			best = rendering
		}
	}
	return best
}

func confidence(winnerVotes, totalVotes int) string {
	if totalVotes <= 0 {
		return ConfidenceLow
	}
	share := float64(winnerVotes) / float64(totalVotes)
	if winnerVotes >= 3 && share >= 0.80 {
		return ConfidenceHigh
	}
	if winnerVotes >= 2 && share >= 0.60 {
		return ConfidenceMedium
	}
	return ConfidenceLow
}

func copyVotes(votes map[string]int) map[string]int {
	out := make(map[string]int, len(votes))
	for rendering, count := range votes {
		out[rendering] = count
	}
	return out
}

func sortedKeys(values map[int]bool) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}
