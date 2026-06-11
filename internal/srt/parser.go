package srt

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/asticode/go-astisub"
	"github.com/oukeidos/focst-local/internal/files"
)

// Segment represents a single subtitle entry.
type Segment struct {
	ID        int
	StartTime string // Format: 00:00:00,000 (standardized for internal use)
	EndTime   string
	Lines     []string
}

// Load reads subtitles from a file and returns them as a slice of Segment.
// It automatically detects the format based on the file extension or content.
func Load(path string) ([]Segment, error) {
	subs, err := astisub.OpenFile(path)
	if err != nil {
		return nil, err
	}
	return fromAstisub(subs), nil
}

// Validate checks if the segments are valid for translation.
// It returns an error if there are no segments, no text, or invalid timestamps.
func Validate(segments []Segment) error {
	if len(segments) == 0 {
		return fmt.Errorf("no subtitles found in file")
	}

	hasText := false
	for i, seg := range segments {
		// 1. Text check
		for _, line := range seg.Lines {
			if strings.TrimSpace(line) != "" {
				hasText = true
				break
			}
		}

		// 2. Timestamp check
		start, err := ParseTimestamp(seg.StartTime)
		if err != nil {
			return fmt.Errorf("invalid StartTime at segment %d (ID: %d): %v", i+1, seg.ID, err)
		}
		end, err := ParseTimestamp(seg.EndTime)
		if err != nil {
			return fmt.Errorf("invalid EndTime at segment %d (ID: %d): %v", i+1, seg.ID, err)
		}

		// 3. Logic check
		if end < start {
			return fmt.Errorf("EndTime is before StartTime at segment %d (ID: %d)", i+1, seg.ID)
		}
	}

	if !hasText {
		return fmt.Errorf("file contains subtitles but no dialogue text")
	}

	return nil
}

// fromAstisub converts astisub.Subtitles to our internal Segment slice.
func fromAstisub(subs *astisub.Subtitles) []Segment {
	segments := make([]Segment, 0, len(subs.Items))
	for i, item := range subs.Items {
		lines := make([]string, 0, len(item.Lines))
		for _, l := range item.Lines {
			lines = append(lines, l.String())
		}

		segments = append(segments, Segment{
			ID:        i + 1,
			StartTime: formatDuration(item.StartAt),
			EndTime:   formatDuration(item.EndAt),
			Lines:     lines,
		})
	}
	return segments
}

func formatDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// Save writes segments to a file, determining the format by file extension.
func Save(path string, segments []Segment) error {
	subs, err := toAstisub(segments)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(path))

	var buf bytes.Buffer
	var writeErr error
	switch ext {
	case ".vtt":
		writeErr = subs.WriteToWebVTT(&buf)
	case ".srt":
		writeErr = subs.WriteToSRT(&buf)
	case ".ssa", ".ass":
		writeErr = subs.WriteToSSA(&buf)
		if writeErr == nil {
			// Replace library-generated Styles section with standard format
			content := fixASSStylesSection(buf.Bytes())
			// Replace \n with \N for better player compatibility
			content = bytes.ReplaceAll(content, []byte("\\n"), []byte("\\N"))
			buf.Reset()
			buf.Write(content)
		}
	case ".ttml":
		writeErr = subs.WriteToTTML(&buf)
	case ".stl":
		writeErr = subs.WriteToSTL(&buf)
	default:
		writeErr = subs.WriteToSRT(&buf)
	}

	if writeErr != nil {
		return fmt.Errorf("failed to write to buffer: %w", writeErr)
	}

	return files.AtomicWrite(path, buf.Bytes(), 0600)
}

// fixASSStylesSection replaces the library-generated Styles section with standard ASS format.
// Standard format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour,
// Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow,
// Alignment, MarginL, MarginR, MarginV, Encoding
func fixASSStylesSection(content []byte) []byte {
	// Standard Styles section to replace the library-generated one
	standardStyles := "[V4+ Styles]\n" +
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n" +
		"Style: Default,Sans,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,1,2,10,10,10,1\n\n"
	// Find and replace the V4+ Styles section
	startMarker := []byte("[V4+ Styles]")
	endMarker := []byte("\n[Events]")

	startIdx := bytes.Index(content, startMarker)
	endIdx := bytes.Index(content, endMarker)

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		// Fallback: also try V4 Styles (non-plus)
		startMarker = []byte("[V4 Styles]")
		startIdx = bytes.Index(content, startMarker)
		if startIdx == -1 || startIdx >= endIdx {
			return content // Can't find section, return as-is
		}
	}

	// Build new content: before + standard styles + from [Events] onwards
	var result bytes.Buffer
	result.Write(content[:startIdx])
	result.WriteString(standardStyles)
	result.Write(content[endIdx+1:]) // +1 to skip the \n before [Events]

	return result.Bytes()
}

// Pointer helper functions for astisub.StyleAttributes
func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }
func ptrBool(v bool) *bool          { return &v }

// defaultASSStyle returns a default ASS style for editor compatibility.
// Output format: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,2,2,10,10,10,1
func defaultASSStyle() *astisub.Style {
	return &astisub.Style{
		ID: "Default",
		InlineStyle: &astisub.StyleAttributes{
			SSAFontName:        "Arial",
			SSAFontSize:        ptrFloat64(20),
			SSAPrimaryColour:   &astisub.Color{Red: 255, Green: 255, Blue: 255, Alpha: 0}, // &H00FFFFFF
			SSASecondaryColour: &astisub.Color{Red: 255, Green: 0, Blue: 0, Alpha: 0},     // &H000000FF
			SSAOutlineColour:   &astisub.Color{Red: 0, Green: 0, Blue: 0, Alpha: 0},       // &H00000000
			SSABackColour:      &astisub.Color{Red: 0, Green: 0, Blue: 0, Alpha: 0},       // &H00000000
			SSABold:            ptrBool(false),
			SSAItalic:          ptrBool(false),
			SSAUnderline:       ptrBool(false),
			SSAStrikeout:       ptrBool(false),
			SSAScaleX:          ptrFloat64(100),
			SSAScaleY:          ptrFloat64(100),
			SSASpacing:         ptrFloat64(0),
			SSAAngle:           ptrFloat64(0),
			SSABorderStyle:     ptrInt(1),
			SSAOutline:         ptrFloat64(2),
			SSAShadow:          ptrFloat64(1),
			SSAAlignment:       ptrInt(2), // Bottom center
			SSAMarginLeft:      ptrInt(10),
			SSAMarginRight:     ptrInt(10),
			SSAMarginVertical:  ptrInt(10),
			SSAEncoding:        ptrInt(1),
		},
	}
}

func toAstisub(segments []Segment) (*astisub.Subtitles, error) {
	subs := astisub.NewSubtitles()
	// Initialize Metadata to prevent nil pointer dereference in WriteToSSA
	subs.Metadata = &astisub.Metadata{SSAScriptType: "v4.00+"}
	// Add default style for ASS/SSA editor compatibility
	subs.Styles = map[string]*astisub.Style{
		"Default": defaultASSStyle(),
	}
	for _, seg := range segments {
		start, err := parseInternalTime(seg.StartTime)
		if err != nil {
			return nil, err
		}
		end, err := parseInternalTime(seg.EndTime)
		if err != nil {
			return nil, err
		}
		item := &astisub.Item{
			StartAt: start,
			EndAt:   end,
		}
		for _, l := range seg.Lines {
			item.Lines = append(item.Lines, astisub.Line{
				Items: []astisub.LineItem{{Text: l}},
			})
		}
		subs.Items = append(subs.Items, item)
	}
	return subs, nil
}

// ParseTimestamp parses an SRT timestamp into a duration since 00:00:00,000.
// It supports hours beyond 23.
func ParseTimestamp(s string) (time.Duration, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid timestamp format: %s", s)
	}

	msStr := parts[1]
	if len(msStr) != 3 {
		return 0, fmt.Errorf("invalid millisecond format: %s", s)
	}

	ms, err := strconv.Atoi(msStr)
	if err != nil || ms < 0 || ms > 999 {
		return 0, fmt.Errorf("invalid milliseconds: %s", s)
	}

	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", s)
	}

	hours, err := strconv.Atoi(hms[0])
	if err != nil || hours < 0 {
		return 0, fmt.Errorf("invalid hours: %s", s)
	}

	minutes, err := strconv.Atoi(hms[1])
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("invalid minutes: %s", s)
	}

	seconds, err := strconv.Atoi(hms[2])
	if err != nil || seconds < 0 || seconds > 59 {
		return 0, fmt.Errorf("invalid seconds: %s", s)
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(ms)*time.Millisecond, nil
}

// FormatTimestamp formats a duration since 00:00:00,000 into an SRT timestamp.
func FormatTimestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func parseInternalTime(s string) (time.Duration, error) {
	return ParseTimestamp(s)
}

// Deprecated: use Load instead.
func Parse(r io.Reader) ([]Segment, error) {
	// Dummy implementation for compatibility if needed, but better to refactor callers
	subs, err := astisub.ReadFromSRT(r)
	if err != nil {
		return nil, err
	}
	return fromAstisub(subs), nil
}

// Deprecated: use Save instead.
func Generate(w io.Writer, segments []Segment) error {
	subs, err := toAstisub(segments)
	if err != nil {
		return err
	}
	return subs.WriteToSRT(w)
}
