package srt

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		wantErr  bool
	}{
		{
			name:     "Valid segments",
			segments: []Segment{{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"Hello"}}},
			wantErr:  false,
		},
		{
			name:     "Empty segments",
			segments: []Segment{},
			wantErr:  true,
		},
		{
			name:     "Segments with no text",
			segments: []Segment{{ID: 1, StartTime: "00:00:01,000", EndTime: "00:00:02,000", Lines: []string{"", "  "}}},
			wantErr:  true,
		},
		{
			name:     "Invalid timestamp format",
			segments: []Segment{{ID: 1, StartTime: "00:00:01.000", EndTime: "00:00:02,000", Lines: []string{"Hello"}}},
			wantErr:  true,
		},
		{
			name:     "EndTime before StartTime",
			segments: []Segment{{ID: 1, StartTime: "00:00:02,000", EndTime: "00:00:01,000", Lines: []string{"Hello"}}},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.segments); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseInternalTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid", "00:00:20,000", false},
		{"Valid max", "23:59:59,999", false},
		{"Valid over 24h", "25:00:00,000", false},
		{"Empty", "", true},
		{"Invalid format dots", "00:00:20.000", true},
		{"Invalid format short", "00:00:20", true},
		{"Invalid chars", "ab:cd:ef,ghi", true},
		{"Invalid minutes", "00:60:00,000", true},
		{"Invalid seconds", "00:00:60,000", true},
		{"Invalid millis", "00:00:00,1000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseInternalTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInternalTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
