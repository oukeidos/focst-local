package srt

import (
	"regexp"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/rivo/uniseg"
)

var (
	ellipsisRegex = regexp.MustCompile(`\.{3}`)
	bracketRegex  = regexp.MustCompile(`[<>]`)
	// Exception regexes for periods
	multiPeriodRegex  = regexp.MustCompile(`\.{2,}`)
	numberPeriodRegex = regexp.MustCompile(`\d\.\d`)
	abbrPeriodRegex   = regexp.MustCompile(`[a-zA-Z]\.[a-zA-Z]`)
	capsAbbrRegex     = regexp.MustCompile(`[A-Z]\.`)
	// Japanese punctuation
	jaCommaRegex  = regexp.MustCompile(`、`)
	jaPeriodRegex = regexp.MustCompile(`。`)
	// Spacing optimization
	multiSpaceRegex = regexp.MustCompile(`\s+`)
)

// Postprocess performs punctuation cleanup and timing correction.
func Postprocess(segments []Segment, targetLangCode string, targetCPS int) []Segment {
	return PostprocessWithOptions(segments, targetLangCode, targetCPS, true)
}

// PostprocessWithOptions performs timing correction and optional language-specific cleanup.
func PostprocessWithOptions(segments []Segment, targetLangCode string, targetCPS int, applyLangRules bool) []Segment {
	// 1. Punctuation Cleanup
	if applyLangRules {
		if targetLangCode == "ko" {
			for i := range segments {
				segments[i] = cleanPunctuation(segments[i])
			}
		} else if targetLangCode == "ja" {
			for i := range segments {
				segments[i] = cleanJapanesePunctuation(segments[i])
			}
		} else if targetLangCode == "zh-Hant" {
			for i := range segments {
				segments[i] = cleanTraditionalChinesePunctuation(segments[i])
			}
		} else if targetLangCode == "zh" || targetLangCode == "zh-Hans" {
			for i := range segments {
				segments[i] = cleanSimplifiedChinesePunctuation(segments[i])
			}
		}
	}

	// 2. Timing Correction
	return correctTiming(segments, targetCPS)
}

func cleanPunctuation(seg Segment) Segment {
	newLines := make([]string, 0, len(seg.Lines))
	for _, line := range seg.Lines {
		// 1.1 Ellipsis
		line = ellipsisRegex.ReplaceAllString(line, "…")

		// 1.4 Remove < and >
		line = bracketRegex.ReplaceAllString(line, "")

		// 1.2 Periods
		line = processPeriods(line)

		// 1.3 Trailing comma
		line = strings.TrimRight(line, ",")

		line = strings.TrimSpace(line)
		if line != "" {
			newLines = append(newLines, line)
		}
	}
	seg.Lines = newLines
	return seg
}

func cleanJapanesePunctuation(seg Segment) Segment {
	newLines := make([]string, 0, len(seg.Lines))
	for _, line := range seg.Lines {
		// 1. Ellipsis
		line = ellipsisRegex.ReplaceAllString(line, "…")

		// 2. Japanese Comma (読点)
		line = processJapaneseComma(line)

		// 3. Japanese Period (句点)
		line = processJapanesePeriod(line)

		line = strings.TrimSpace(line)
		if line != "" {
			newLines = append(newLines, line)
		}
	}
	seg.Lines = newLines
	return seg
}

func processJapaneseComma(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		if runes[i] == '、' {
			// Check if it's the end of the line
			isEnd := true
			lastIdx := i
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' && runes[j] != '　' {
					isEnd = false
					break
				}
				lastIdx = j
			}

			if isEnd {
				// Remove it and skip all following spaces
				i = lastIdx
			} else {
				// Middle of line: replace with half-width space and skip following spaces
				sb.WriteRune(' ')
				i = lastIdx
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

func processJapanesePeriod(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		if runes[i] == '。' {
			// Check if it's the end of the line
			isEnd := true
			lastIdx := i
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' && runes[j] != '　' {
					isEnd = false
					break
				}
				lastIdx = j
			}

			if isEnd {
				// Remove it and skip all following spaces
				i = lastIdx
			} else {
				// Middle of line: replace with full-width space and skip following spaces
				sb.WriteRune('　')
				i = lastIdx
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

func cleanTraditionalChinesePunctuation(seg Segment) Segment {
	newLines := make([]string, 0, len(seg.Lines))
	for _, line := range seg.Lines {
		// 1. Ellipsis
		line = ellipsisRegex.ReplaceAllString(line, "…")

		// 2. Ideographic Comma (、) - Delete at end, Keep in middle
		line = processChineseIdeographicComma(line)

		// 3. Comma (,, ，) - Normalize to ，, Exception for digits, Delete at end
		line = processChineseComma(line)

		// 4. Period (., 。) - Normalize to ，, Exceptions, Delete at end
		line = processChinesePeriod(line)

		// 5. Spacing Optimization
		// Merge spaces
		line = multiSpaceRegex.ReplaceAllString(line, " ")
		// Remove space after full-width comma
		line = strings.ReplaceAll(line, "， ", "，")
		// Trim
		line = strings.TrimSpace(line)

		if line != "" {
			newLines = append(newLines, line)
		}
	}
	seg.Lines = newLines
	return seg
}

func processChineseIdeographicComma(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		if runes[i] == '、' {
			// Check if end of line (ignoring spaces)
			isEnd := true
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' {
					isEnd = false
					break
				}
			}
			if !isEnd {
				sb.WriteRune('、')
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

func processChineseComma(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == ',' || r == '，' {
			// Exception: digits (1,000) - only for half-width comma
			if r == ',' && i > 0 && i < n-1 && isDigit(runes[i-1]) && isDigit(runes[i+1]) {
				sb.WriteRune(',')
				continue
			}

			// Check end of line
			isEnd := true
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' {
					isEnd = false
					break
				}
			}

			if !isEnd {
				sb.WriteRune('，') // Normalize to full-width
			}
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func processChinesePeriod(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '.' || r == '。' {
			// Exceptions (only for half-width period usually, but checking for safety)
			if r == '.' && isException(runes, i) {
				sb.WriteRune('.')
				continue
			}

			// Check end of line
			isEnd := true
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' {
					isEnd = false
					break
				}
			}

			if !isEnd {
				sb.WriteRune('，') // Normalize to full-width comma
			}
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func cleanSimplifiedChinesePunctuation(seg Segment) Segment {
	newLines := make([]string, 0, len(seg.Lines))
	for _, line := range seg.Lines {
		// 1. Ellipsis
		line = ellipsisRegex.ReplaceAllString(line, "…")

		// 2. Ideographic Comma (、) - Delete at end, Keep in middle
		line = processChineseIdeographicComma(line)

		// 3. Comma/Period (,, ，, ., 。) - Replace with space, Exceptions, Delete at end
		line = processSimplifiedChinesePunctuation(line)

		// 4. Spacing Optimization
		// Merge spaces
		line = multiSpaceRegex.ReplaceAllString(line, " ")
		// Trim
		line = strings.TrimSpace(line)

		if line != "" {
			newLines = append(newLines, line)
		}
	}
	seg.Lines = newLines
	return seg
}

func processSimplifiedChinesePunctuation(line string) string {
	runes := []rune(line)
	n := len(runes)
	var sb strings.Builder

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == ',' || r == '，' || r == '.' || r == '。' {
			// Exception: digits (1,000 or 3.14)
			if (r == ',' || r == '.') && i > 0 && i < n-1 && isDigit(runes[i-1]) && isDigit(runes[i+1]) {
				sb.WriteRune(r)
				continue
			}
			// Exception: consecutive periods
			if r == '.' && isException(runes, i) {
				sb.WriteRune('.')
				continue
			}

			// Check end of line
			isEnd := true
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' {
					isEnd = false
					break
				}
			}

			if !isEnd {
				sb.WriteRune(' ') // Replace with space
			}
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func processPeriods(line string) string {
	// We iterate through the string and check periods that aren't part of exceptions
	var sb strings.Builder
	runes := []rune(line)
	n := len(runes)

	for i := 0; i < n; i++ {
		if runes[i] == '.' {
			// Check exceptions
			if isException(runes, i) {
				sb.WriteRune('.')
				continue
			}

			// Check if it's the end of the line (or followed only by spaces/trailing punctuation)
			isEnd := true
			for j := i + 1; j < n; j++ {
				if runes[j] != ' ' && runes[j] != ',' && runes[j] != '!' && runes[j] != '?' {
					isEnd = false
					break
				}
			}

			if isEnd {
				// Remove it (don't write to sb)
			} else {
				// Middle of line: change to comma
				sb.WriteRune(',')
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

func isException(runes []rune, idx int) bool {
	n := len(runes)

	// Multi-period (.. or ...)
	if (idx > 0 && runes[idx-1] == '.') || (idx < n-1 && runes[idx+1] == '.') {
		return true
	}

	// Number period (3.14)
	if idx > 0 && idx < n-1 && isDigit(runes[idx-1]) && isDigit(runes[idx+1]) {
		return true
	}

	// Abbr period (google.com, a.b)
	if idx > 0 && idx < n-1 && isAlpha(runes[idx-1]) && isAlpha(runes[idx+1]) {
		return true
	}

	// Caps abbr (U.S.A.)
	if idx > 0 && isUpper(runes[idx-1]) {
		return true
	}

	return false
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
func isAlpha(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func correctTiming(segments []Segment, targetCPS int) []Segment {
	if len(segments) == 0 {
		return segments
	}

	if targetCPS <= 0 {
		targetCPS = 12 // Default fallback
	}

	invalidTimingCount := 0

	// Step 1: Readability & Duration (Min 0.8s)
	for i := range segments {
		start, err1 := ParseTimestamp(segments[i].StartTime)
		end, err2 := ParseTimestamp(segments[i].EndTime)
		if err1 != nil || err2 != nil {
			invalidTimingCount++
			continue // Skip segments with invalid timestamps to avoid corruption
		}

		duration := end.Seconds() - start.Seconds()

		// Calculate total characters (grapheme clusters including spaces)
		totalChars := 0
		for _, line := range segments[i].Lines {
			totalChars += uniseg.GraphemeClusterCount(line)
		}

		// Ensure min duration 0.8s
		if duration < 0.8 {
			duration = 0.8
		}

		// Apply target CPS
		reqDuration := float64(totalChars) / float64(targetCPS)
		if duration < reqDuration {
			duration = reqDuration
		}

		newEnd := start + time.Duration(duration*float64(time.Second))
		segments[i].EndTime = FormatTimestamp(newEnd)
	}

	// Step 2: Overlap Prevention (5ms gap)
	invalidOverlapCount := 0
	for i := 0; i < len(segments)-1; i++ {
		currEnd, err1 := ParseTimestamp(segments[i].EndTime)
		nextStart, err2 := ParseTimestamp(segments[i+1].StartTime)
		if err1 != nil || err2 != nil {
			invalidOverlapCount++
			continue
		}

		// If end of current is > next start - 5ms, adjust current end.
		// Skip adjustment if it would create a negative duration.
		gap := 5 * time.Millisecond
		targetEnd := nextStart - gap
		if currEnd > targetEnd {
			currStart, err3 := ParseTimestamp(segments[i].StartTime)
			if err3 != nil {
				continue
			}
			if targetEnd >= currStart {
				segments[i].EndTime = FormatTimestamp(targetEnd)
			}
		}
	}
	if invalidTimingCount > 0 {
		logger.Warn("Postprocess skipped segments with invalid timestamps", "count", invalidTimingCount)
	}
	if invalidOverlapCount > 0 {
		logger.Warn("Postprocess skipped overlap checks due to invalid timestamps", "count", invalidOverlapCount)
	}
	return segments
}
