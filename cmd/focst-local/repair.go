package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/translator"
	"github.com/spf13/cobra"
)

var (
	runRepairPipeline    = pipeline.RunRepair
	printRepairStatsFunc = printUsageStats
)

type repairOptions struct {
	forceRepair        bool
	baseURL            string
	translationTimeout time.Duration
	debug              bool
	llama              llamaServerOptions
}

func newRepairCmd() *cobra.Command {
	opts := repairOptions{}
	cmd := &cobra.Command{
		Use:   "repair <session_log.json>",
		Short: "Resume a failed translation using a session log",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				_ = cmd.Usage()
				return fmt.Errorf("session_log.json is required")
			}
			return runRepair(cmd, args, &opts)
		},
		SilenceUsage: true,
	}

	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.Flags().BoolVar(&opts.forceRepair, "force-repair", false, "Ignore existing output and re-translate all chunks")
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per translation request; 0 disables the timeout")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
	return cmd
}

func runRepair(cmd *cobra.Command, args []string, opts *repairOptions) error {
	startTime := time.Now()
	logPath := args[0]

	logLevel := logger.LevelInfo
	if opts.debug {
		logLevel = logger.LevelDebug
	}
	logger.Init(logLevel, nil)
	launchCfg, err := buildLlamaLaunchConfig(cmd, opts.llama, "", opts.baseURL)
	if err != nil {
		return err
	}

	cfg := pipeline.Config{
		LogPath:            logPath,
		BaseURL:            launchCfg.BaseURL,
		LlamaServer:        launchCfg,
		TranslationTimeout: opts.translationTimeout,
		ForceRepair:        opts.forceRepair,
		OnProgress: func(p translator.TranslationProgress) {
			switch p.State {
			case translator.StateCompleted:
				logger.Info("Chunk completed", "chunk", p.ChunkIndex)
			case translator.StateInProgress:
				logger.Warn("Chunk retry", "chunk", p.ChunkIndex, "attempt", p.Attempt, "error", p.Error)
			}
		},
	}

	ctx, stop := signalContext()
	defer stop()
	result, err := runRepairPipeline(ctx, cfg)

	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Repair canceled", "error", err)
			return nil
		}
		if shouldPrintRepairStats(result) {
			printRepairStatsFunc(&result.Usage, time.Since(startTime), result.Model)
		}
		return err
	}
	printRepairStatsFunc(&result.Usage, time.Since(startTime), result.Model)

	return nil
}

func shouldPrintRepairStats(result pipeline.RepairResult) bool {
	if strings.TrimSpace(result.Model) != "" {
		return true
	}
	usage := result.Usage
	return usage.PromptTokenCount > 0 ||
		usage.CandidatesTokenCount > 0 ||
		usage.TotalTokenCount > 0 ||
		usage.WebSearchCount > 0
}
