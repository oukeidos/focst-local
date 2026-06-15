package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTranslation_InvalidPaths(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input.srt")
	os.WriteFile(inPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0644)

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "Same input and output",
			cfg: Config{
				InputPath:   inPath,
				OutputPath:  inPath,
				ChunkSize:   10,
				Concurrency: 1,
			},
			wantErr: "input and output files are the same",
		},
		{
			name: "Unsupported source language",
			cfg: Config{
				InputPath:   inPath,
				OutputPath:  filepath.Join(tmpDir, "out.srt"),
				SourceLang:  "invalid",
				TargetLang:  "ko",
				ChunkSize:   10,
				Concurrency: 1,
			},
			wantErr: "unsupported source language",
		},
		{
			name: "Same source and target",
			cfg: Config{
				InputPath:   inPath,
				OutputPath:  filepath.Join(tmpDir, "out.srt"),
				SourceLang:  "ko",
				TargetLang:  "ko",
				ChunkSize:   10,
				Concurrency: 1,
			},
			wantErr: "source and target languages must be different",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RunTranslation(context.Background(), tt.cfg)
			if err == nil || (tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("RunTranslation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigNormalize_ConcurrencyClamp(t *testing.T) {
	tests := []struct {
		name        string
		in          int
		want        int
		wantChanged bool
	}{
		{"below_min", 0, MinConcurrency, true},
		{"above_max", MaxConcurrency + 5, MaxConcurrency, true},
		{"within_range", MinConcurrency, MinConcurrency, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Concurrency: tt.in}
			gotCfg, notes := cfg.Normalize()
			if gotCfg.Concurrency != tt.want {
				t.Fatalf("Normalize() concurrency = %d, want %d", gotCfg.Concurrency, tt.want)
			}
			if tt.wantChanged && len(notes) == 0 {
				t.Fatalf("Normalize() expected notes for clamped value")
			}
			if !tt.wantChanged && len(notes) != 0 {
				t.Fatalf("Normalize() unexpected notes for unchanged value")
			}
		})
	}
}

func TestConfigValidate_RejectsNegativeTranslationTimeout(t *testing.T) {
	cfg := Config{
		InputPath:            "in.srt",
		OutputPath:           "out.srt",
		BaseURL:              "http://127.0.0.1:8080/v1",
		Model:                "test-model",
		ChunkSize:            10,
		ContextSize:          0,
		Concurrency:          1,
		SourceLang:           "ja",
		TargetLang:           "ko",
		TranslationTimeout:   -1,
		ChunkBoundaryPlanner: ChunkBoundaryPlannerOff,
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "translationTimeout must be 0 or greater") {
		t.Fatalf("expected negative translation timeout validation error, got %v", err)
	}
}

func TestConfigNormalize_GlossaryDefaultsUseProductShapedRuns(t *testing.T) {
	cfg := Config{}
	got, _ := cfg.Normalize()
	if got.GlossaryRuns != 3 {
		t.Fatalf("GlossaryRuns = %d, want 3", got.GlossaryRuns)
	}
	if got.GlossaryWindowChunks != 3 {
		t.Fatalf("GlossaryWindowChunks = %d, want 3", got.GlossaryWindowChunks)
	}
}

func TestConfigNormalize_PreservesExplicitGlossaryValidationRuns(t *testing.T) {
	cfg := Config{GlossaryRuns: 10, GlossaryWindowChunks: 4}
	got, _ := cfg.Normalize()
	if got.GlossaryRuns != 10 {
		t.Fatalf("GlossaryRuns = %d, want 10", got.GlossaryRuns)
	}
	if got.GlossaryWindowChunks != 4 {
		t.Fatalf("GlossaryWindowChunks = %d, want 4", got.GlossaryWindowChunks)
	}
}

func TestConfigNormalize_SentenceAwareDefaultRangeUsesClampedChunkSize(t *testing.T) {
	cfg := Config{
		ChunkSize:           MaxChunkSize + 50,
		Concurrency:         1,
		SentenceAwareChunks: true,
	}
	got, _ := cfg.Normalize()
	if got.ChunkSize != MaxChunkSize {
		t.Fatalf("ChunkSize = %d, want %d", got.ChunkSize, MaxChunkSize)
	}
	if got.MinChunkSize != MaxChunkSize-10 {
		t.Fatalf("MinChunkSize = %d, want %d", got.MinChunkSize, MaxChunkSize-10)
	}
	if got.MaxChunkSize != MaxChunkSize {
		t.Fatalf("MaxChunkSize = %d, want %d", got.MaxChunkSize, MaxChunkSize)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("normalized config should validate, got: %v", err)
	}
}
