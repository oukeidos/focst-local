package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/translation"
)

func TestShouldPrintRepairStats(t *testing.T) {
	t.Run("empty_result", func(t *testing.T) {
		if shouldPrintRepairStats(pipeline.RepairResult{}) {
			t.Fatalf("expected false for empty result")
		}
	})

	t.Run("model_present", func(t *testing.T) {
		if !shouldPrintRepairStats(pipeline.RepairResult{Model: "gemma-4-26b-a4b-qat-q4_0"}) {
			t.Fatalf("expected true when model is present")
		}
	})

	t.Run("usage_present", func(t *testing.T) {
		result := pipeline.RepairResult{
			Usage: translation.UsageMetadata{TotalTokenCount: 42},
		}
		if !shouldPrintRepairStats(result) {
			t.Fatalf("expected true when usage is present")
		}
	})
}

func TestRunRepair_StatsPrinting(t *testing.T) {
	prevRunRepairPipeline := runRepairPipeline
	prevPrintRepairStatsFunc := printRepairStatsFunc
	defer func() {
		runRepairPipeline = prevRunRepairPipeline
		printRepairStatsFunc = prevPrintRepairStatsFunc
	}()

	args := []string{"/tmp/session_log.json"}
	opts := &repairOptions{baseURL: "http://127.0.0.1:1/v1"}

	t.Run("early_failure_skips_stats", func(t *testing.T) {
		runRepairPipeline = func(_ context.Context, _ pipeline.Config) (pipeline.RepairResult, error) {
			return pipeline.RepairResult{}, errors.New("invalid recovery log")
		}
		statsCalls := 0
		printRepairStatsFunc = func(_ *translation.UsageMetadata, _ time.Duration, _ string) {
			statsCalls++
		}

		err := runRepair(nil, args, opts)
		if err == nil {
			t.Fatalf("expected error")
		}
		if statsCalls != 0 {
			t.Fatalf("expected stats to be skipped, got %d calls", statsCalls)
		}
	})

	t.Run("failure_with_usage_prints_stats", func(t *testing.T) {
		runRepairPipeline = func(_ context.Context, _ pipeline.Config) (pipeline.RepairResult, error) {
			return pipeline.RepairResult{
				Model: "gemma-4-26b-a4b-qat-q4_0",
				Usage: translation.UsageMetadata{TotalTokenCount: 100},
			}, errors.New("repair failed after api calls")
		}
		statsCalls := 0
		printRepairStatsFunc = func(_ *translation.UsageMetadata, _ time.Duration, _ string) {
			statsCalls++
		}

		err := runRepair(nil, args, opts)
		if err == nil {
			t.Fatalf("expected error")
		}
		if statsCalls != 1 {
			t.Fatalf("expected stats to be printed once, got %d calls", statsCalls)
		}
	})

	t.Run("success_prints_stats", func(t *testing.T) {
		runRepairPipeline = func(_ context.Context, _ pipeline.Config) (pipeline.RepairResult, error) {
			return pipeline.RepairResult{}, nil
		}
		statsCalls := 0
		printRepairStatsFunc = func(_ *translation.UsageMetadata, _ time.Duration, _ string) {
			statsCalls++
		}

		err := runRepair(nil, args, opts)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if statsCalls != 1 {
			t.Fatalf("expected stats to be printed once, got %d calls", statsCalls)
		}
	})
}
