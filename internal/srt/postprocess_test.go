package srt

import (
	"reflect"
	"testing"
)

func TestCleanPunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Ellipsis conversion",
			input:    []string{"Hello...", "Wait... for it"},
			expected: []string{"Hello…", "Wait… for it"},
		},
		{
			name:     "Period exceptions (multi, number, domain, abbr)",
			input:    []string{"Two periods..", "Pi is 3.14", "Visit google.com", "Made in U.S.A."},
			expected: []string{"Two periods..", "Pi is 3.14", "Visit google.com", "Made in U.S.A."},
		},
		{
			name:     "Period to comma conversion (middle)",
			input:    []string{"Hello. World", "One. Two. Three"},
			expected: []string{"Hello, World", "One, Two, Three"},
		},
		{
			name:     "Period removal (end)",
			input:    []string{"The end.", "Fine. "},
			expected: []string{"The end", "Fine"},
		},
		{
			name:     "Trailing comma removal",
			input:    []string{"Hello,", "Wait, for it,"},
			expected: []string{"Hello", "Wait, for it"},
		},
		{
			name:     "Bracket removal",
			input:    []string{"<Hello>", "Value > 10"},
			expected: []string{"Hello", "Value  10"}, // The space remains if we just remove the char
		},
		{
			name:     "Complex mix",
			input:    []string{"Hello. It's 10.5 degrees... in the U.K.,"},
			expected: []string{"Hello, It's 10.5 degrees… in the U.K."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := Segment{Lines: tt.input}
			got := cleanPunctuation(seg)
			if !reflect.DeepEqual(got.Lines, tt.expected) {
				t.Errorf("%s: got %v, want %v", tt.name, got.Lines, tt.expected)
			}
		})
	}
}

func TestCleanJapanesePunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Ellipsis conversion (JA)",
			input:    []string{"これですね..."},
			expected: []string{"これですね…"},
		},
		{
			name:     "Japanese comma (JA)",
			input:    []string{"こんにちは、元気?", "これは、ですね、はい。"},
			expected: []string{"こんにちは 元気?", "これは ですね はい"}, // Note: Period at end also removed
		},
		{
			name:     "Japanese period middle (JA)",
			input:    []string{"そう。分かった"},
			expected: []string{"そう　分かった"},
		},
		{
			name:     "Japanese period end (JA)",
			input:    []string{"終わりました。"},
			expected: []string{"終わりました"},
		},
		{
			name:     "Combined cases (JA)",
			input:    []string{"はい、そうです。分かった...ね？"},
			expected: []string{"はい そうです　分かった…ね？"},
		},
		{
			name:     "Japanese punctuation with trailing spaces (JA)",
			input:    []string{"はい、  そうです。  "}, // Comma followed by spaces, Period followed by spaces
			expected: []string{"はい そうです"},
		},
		{
			name:     "Consecutive Japanese punctuation (JA)",
			input:    []string{"あ、。うん"},
			expected: []string{"あ 　うん"}, // Comma replaced by space, Period replaced by full-width space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := Segment{Lines: tt.input}
			got := cleanJapanesePunctuation(seg)
			if !reflect.DeepEqual(got.Lines, tt.expected) {
				t.Errorf("%s: got %v, want %v", tt.name, got.Lines, tt.expected)
			}
		})
	}
}

func TestCleanTraditionalChinesePunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Ellipsis conversion (ZH)",
			input:    []string{"等一下..."},
			expected: []string{"等一下…"},
		},
		{
			name:     "Ideographic comma (ZH) - Delete at end, Keep in middle",
			input:    []string{"蘋果、", "橘子、香蕉"},
			expected: []string{"蘋果", "橘子、香蕉"},
		},
		{
			name:     "Comma (ZH) - Digits exception, Delete at end, Normalize to Full-width",
			input:    []string{"1,000元,", "你好, 世界", "結束了，"},
			expected: []string{"1,000元", "你好，世界", "結束了"},
		},
		{
			name:     "Period (ZH) - Exceptions, Delete at end, Normalize to Full-width Comma",
			input:    []string{"3.14", "A.B", "你好. 世界。", "結束."},
			expected: []string{"3.14", "A.B", "你好，世界", "結束"},
		},
		{
			name:     "Spacing Optimization (ZH)",
			input:    []string{"  你好，  世界  ", "你好， 世界", "你好，  世界"},
			expected: []string{"你好，世界", "你好，世界", "你好，世界"}, // Trims, merges spaces, removes space after comma
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := Segment{Lines: tt.input}
			got := cleanTraditionalChinesePunctuation(seg)
			if !reflect.DeepEqual(got.Lines, tt.expected) {
				t.Errorf("%s: got %v, want %v", tt.name, got.Lines, tt.expected)
			}
		})
	}
}

func TestCleanSimplifiedChinesePunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Ellipsis conversion (ZH_CN)",
			input:    []string{"等一下..."},
			expected: []string{"等一下…"},
		},
		{
			name:     "Ideographic comma (ZH_CN) - Keep in middle",
			input:    []string{"苹果、", "橘子、香蕉"},
			expected: []string{"苹果", "橘子、香蕉"},
		},
		{
			name:     "Comma/Period (ZH_CN) - Digits exception, Delete at end, Replace with Space",
			input:    []string{"1,000元,", "你好, 世界。", "通过。"},
			expected: []string{"1,000元", "你好 世界", "通过"},
		},
		{
			name:     "Period (ZH_CN) - Exceptions, Delete at end, Replace with Space",
			input:    []string{"3.14", "A.B", "你好. 世界。", "结束.", "U.S.A."},
			expected: []string{"3.14", "A.B", "你好 世界", "结束", "U.S.A."},
		},
		{
			name:     "Spacing Optimization (ZH_CN)",
			input:    []string{"  你好，  世界  ", "你好， 世界", "你好，  世界"},
			expected: []string{"你好 世界", "你好 世界", "你好 世界"}, // Trims, merges spaces, replaces comma with space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := Segment{Lines: tt.input}
			got := cleanSimplifiedChinesePunctuation(seg)
			if !reflect.DeepEqual(got.Lines, tt.expected) {
				t.Errorf("%s: got %v, want %v", tt.name, got.Lines, tt.expected)
			}
		})
	}
}

func TestPostprocess(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		lang     string
		cps      int
		expected []Segment
	}{
		{
			name: "Korean integration",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,500", Lines: []string{"안녕하세요. 반갑습니다..."}},
			},
			lang: "ko",
			cps:  12,
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,083", Lines: []string{"안녕하세요, 반갑습니다…"}}, // 13 chars / 12 = 1.083s
			},
		},
		{
			name: "Japanese integration",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,500", Lines: []string{"こんにちは、元気?。"}},
			},
			lang: "ja",
			cps:  4,
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:03,250", Lines: []string{"こんにちは 元気?"}}, // 13 chars / 4 = 3.25s
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Postprocess(tt.segments, tt.lang, tt.cps)
			if !reflect.DeepEqual(got[0].Lines, tt.expected[0].Lines) {
				t.Errorf("%s lines: got %v, want %v", tt.name, got[0].Lines, tt.expected[0].Lines)
			}
			if got[0].EndTime != tt.expected[0].EndTime {
				t.Errorf("%s timing: got %s, want %s", tt.name, got[0].EndTime, tt.expected[0].EndTime)
			}
		})
	}
}

func TestCorrectTiming(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		expected []Segment
	}{
		{
			name: "Minimum duration 0.8s",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,200", Lines: []string{"Hi"}},
			},
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,800", Lines: []string{"Hi"}},
			},
		},
		{
			name: "CPS Limit 12",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"This is a very long sentence that exceeds twelve cps."}}, // 50+ chars
			},
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:05,416", Lines: []string{"This is a very long sentence that exceeds twelve cps."}}, // 53 chars / 12 = 4.416s
			},
		},
		{
			name: "Overlap prevention 5ms",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"First"}},
				{ID: 2, StartTime: "00:00:02,000", EndTime: "00:00:03,000", Lines: []string{"Second"}},
			},
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,995", Lines: []string{"First"}},
				{ID: 2, StartTime: "00:00:02,000", EndTime: "00:00:03,000", Lines: []string{"Second"}},
			},
		},
		{
			name: "Overlap adjustment skipped on negative duration",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:02,000", EndTime: "00:00:03,000", Lines: []string{"First"}},
				{ID: 2, StartTime: "00:00:02,001", EndTime: "00:00:03,000", Lines: []string{"Second"}},
			},
			expected: []Segment{
				{ID: 1, StartTime: "00:00:02,000", EndTime: "00:00:03,000", Lines: []string{"First"}},
				{ID: 2, StartTime: "00:00:02,001", EndTime: "00:00:03,000", Lines: []string{"Second"}},
			},
		},
		{
			name: "Grapheme Cluster CPS",
			segments: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,500", Lines: []string{"Family: 👨‍👩‍👧‍👦"}}, // 10 chars "Family: " + 1 complex emoji
			},
			expected: []Segment{
				{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:01,800", Lines: []string{"Family: 👨‍👩‍👧‍👦"}}, // 9 visual chars / 12 = 0.75s, so Min 0.8s applies
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := correctTiming(tt.segments, 12)
			for i := range got {
				if got[i].StartTime != tt.expected[i].StartTime || got[i].EndTime != tt.expected[i].EndTime {
					t.Errorf("%s segment %d: got %s->%s, want %s->%s", tt.name, i, got[i].StartTime, got[i].EndTime, tt.expected[i].StartTime, tt.expected[i].EndTime)
				}
			}
		})
	}
}
