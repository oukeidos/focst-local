package main

import (
	"strings"
	"testing"

	"github.com/oukeidos/focst-local/internal/pipeline"
)

func TestTranslationStatusError(t *testing.T) {
	cases := []struct {
		name    string
		result  pipeline.TranslationResult
		wantErr string
	}{
		{
			name:    "success",
			result:  pipeline.TranslationResult{Status: pipeline.TranslationStatusSuccess},
			wantErr: "",
		},
		{
			name:    "partial_with_log",
			result:  pipeline.TranslationResult{Status: pipeline.TranslationStatusPartialSuccess, RecoveryLogPath: "/tmp/session.json"},
			wantErr: "translation finished with status: Partial Success (recovery log: /tmp/session.json)",
		},
		{
			name:    "failure_without_log",
			result:  pipeline.TranslationResult{Status: pipeline.TranslationStatusFailure},
			wantErr: "translation finished with status: Failure",
		},
		{
			name:    "skipped",
			result:  pipeline.TranslationResult{Status: pipeline.TranslationStatusSkipped},
			wantErr: "",
		},
		{
			name:    "unknown_status",
			result:  pipeline.TranslationResult{},
			wantErr: `translation finished with unknown status: ""`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := translationStatusError(tc.result)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.wantErr)
			}
		})
	}
}
