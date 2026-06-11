package main

import (
	"strings"
	"testing"
)

func TestValidateSubtitlePathExtensions(t *testing.T) {
	t.Run("accepts_supported_extensions", func(t *testing.T) {
		if err := validateSubtitlePathExtensions("in.srt", "out.ass"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("rejects_unsupported_input_extension", func(t *testing.T) {
		err := validateSubtitlePathExtensions("in.txt", "out.srt")
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), `unsupported input extension ".txt"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects_unsupported_output_extension", func(t *testing.T) {
		err := validateSubtitlePathExtensions("in.srt", "out.foo")
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), `unsupported output extension ".foo"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDefaultAndTranslateInvocation_ExtensionValidationConsistency(t *testing.T) {
	t.Run("unsupported_input_extension", func(t *testing.T) {
		rootOut, rootErr := executeCommand(t, "/tmp/focst_sample.txt", "/tmp/out.srt")
		if rootErr == nil {
			t.Fatalf("expected root invocation error")
		}
		if !strings.Contains(rootErr.Error(), `unsupported input extension ".txt"`) {
			t.Fatalf("unexpected root error: %v", rootErr)
		}
		if strings.Contains(rootErr.Error(), "unknown command") || strings.Contains(rootOut, "unknown command") {
			t.Fatalf("root invocation should not fail as unknown command, out=%q err=%v", rootOut, rootErr)
		}

		subOut, subErr := executeCommand(t, "translate", "/tmp/focst_sample.txt", "/tmp/out.srt")
		if subErr == nil {
			t.Fatalf("expected translate subcommand error")
		}
		if !strings.Contains(subErr.Error(), `unsupported input extension ".txt"`) {
			t.Fatalf("unexpected translate error: %v", subErr)
		}
		if strings.Contains(subErr.Error(), "unknown command") || strings.Contains(subOut, "unknown command") {
			t.Fatalf("translate subcommand should not fail as unknown command, out=%q err=%v", subOut, subErr)
		}
	})

	t.Run("unsupported_output_extension", func(t *testing.T) {
		rootOut, rootErr := executeCommand(t, "/tmp/focst_sample.srt", "/tmp/out.foo")
		if rootErr == nil {
			t.Fatalf("expected root invocation error")
		}
		if !strings.Contains(rootErr.Error(), `unsupported output extension ".foo"`) {
			t.Fatalf("unexpected root error: %v", rootErr)
		}
		if strings.Contains(rootErr.Error(), "unknown command") || strings.Contains(rootOut, "unknown command") {
			t.Fatalf("root invocation should not fail as unknown command, out=%q err=%v", rootOut, rootErr)
		}

		subOut, subErr := executeCommand(t, "translate", "/tmp/focst_sample.srt", "/tmp/out.foo")
		if subErr == nil {
			t.Fatalf("expected translate subcommand error")
		}
		if !strings.Contains(subErr.Error(), `unsupported output extension ".foo"`) {
			t.Fatalf("unexpected translate error: %v", subErr)
		}
		if strings.Contains(subErr.Error(), "unknown command") || strings.Contains(subOut, "unknown command") {
			t.Fatalf("translate subcommand should not fail as unknown command, out=%q err=%v", subOut, subErr)
		}
	})
}
