package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/oukeidos/focst-local/internal/cleanup"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/spf13/cobra"
)

var runGlossaryExtractionPipeline = pipeline.RunGlossaryExtraction

type glossaryOptions struct {
	modelName             string
	baseURL               string
	maxTokens             int
	translationTimeout    time.Duration
	chunkSize             int
	contextSize           int
	noSentenceAwareChunks bool
	minChunkSize          int
	maxChunkSize          int
	chunkBoundaryPlanner  string
	sourceLangCode        string
	targetLangCode        string
	runs                  int
	windowChunks          int
	artifactsDir          string
	logFilePath           string
	yes                   bool
	noPreprocess          bool
	noLangPreprocess      bool
	debug                 bool
	llama                 llamaServerOptions
}

func newGlossaryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "glossary",
		Short: "Generate and inspect local glossary artifacts",
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.AddCommand(newGlossaryExtractCmd())
	return cmd
}

func newGlossaryExtractCmd() *cobra.Command {
	opts := glossaryOptions{}
	cmd := &cobra.Command{
		Use:   "extract <input.srt> <glossary.json>",
		Short: "Extract a local glossary without translating",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				_ = cmd.Usage()
				return fmt.Errorf("input subtitle and glossary output are required")
			}
			return runGlossaryExtract(cmd, args, &opts)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addGlossaryExtractFlags(cmd, &opts)
	return cmd
}

func addGlossaryExtractFlags(cmd *cobra.Command, opts *glossaryOptions) {
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().StringVar(&opts.modelName, "model", localllm.DefaultModel, "Local model name")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", localllm.DefaultMaxTokens, "Maximum generated tokens per request")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per glossary request; 0 disables the timeout")
	cmd.Flags().IntVar(&opts.chunkSize, "chunk-size", 100, "Number of segments per translation chunk")
	cmd.Flags().IntVar(&opts.contextSize, "context-size", 5, "Number of context segments before/after")
	cmd.Flags().BoolVar(&opts.noSentenceAwareChunks, "no-sentence-aware-chunks", false, "Use exact fixed-size chunks without sentence-aware boundary planning")
	cmd.Flags().IntVar(&opts.minChunkSize, "min-chunk-size", 0, "Minimum target chunk size for sentence-aware planning (default: chunk-size - 10)")
	cmd.Flags().IntVar(&opts.maxChunkSize, "max-chunk-size", 0, "Maximum target chunk size for sentence-aware planning (default: chunk-size + 10)")
	cmd.Flags().StringVar(&opts.chunkBoundaryPlanner, "chunk-boundary-planner", pipeline.ChunkBoundaryPlannerLocalLLM, "Boundary planner: local-llm, deterministic, or off")
	cmd.Flags().StringVar(&opts.sourceLangCode, "source", "ja", "Source language code (default: ja)")
	cmd.Flags().StringVar(&opts.targetLangCode, "target", "ko", "Target language code (default: ko)")
	cmd.Flags().IntVar(&opts.runs, "glossary-runs", 3, "Number of local glossary extraction runs per glossary window")
	cmd.Flags().IntVar(&opts.windowChunks, "glossary-window-chunks", 3, "Number of translation chunks per glossary extraction window")
	cmd.Flags().StringVar(&opts.artifactsDir, "glossary-artifacts", "", "Directory for local glossary debug artifacts")
	cmd.Flags().StringVar(&opts.logFilePath, "log-file", "", "Path to save machine-readable JSONL logs")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite glossary output file without asking")
	cmd.Flags().BoolVar(&opts.noPreprocess, "no-preprocess", false, "Disable all preprocessing")
	cmd.Flags().BoolVar(&opts.noLangPreprocess, "no-lang-preprocess", false, "Disable language-specific preprocessing only")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
}

func runGlossaryExtract(cmd *cobra.Command, args []string, opts *glossaryOptions) error {
	if err := validateSubtitleExtension("input", args[0]); err != nil {
		return err
	}
	if opts.logFilePath != "" {
		if err := files.RejectSymlinkPath(opts.logFilePath); err != nil {
			return err
		}
	}
	logLevel := logger.LevelInfo
	if opts.debug {
		logLevel = logger.LevelDebug
	}
	var logFileW io.Writer
	if opts.logFilePath != "" {
		f, err := os.OpenFile(opts.logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		cleanup.Register(f.Close)
		logFileW = f
	}
	logger.Init(logLevel, logFileW)

	launchCfg, err := buildLlamaLaunchConfig(cmd, opts.llama, opts.modelName, opts.baseURL)
	if err != nil {
		return err
	}
	cfg := pipeline.Config{
		InputPath:            args[0],
		OutputPath:           args[1],
		BaseURL:              launchCfg.BaseURL,
		Model:                launchCfg.ModelAlias,
		LlamaServer:          launchCfg,
		MaxTokens:            opts.maxTokens,
		TranslationTimeout:   opts.translationTimeout,
		ChunkSize:            opts.chunkSize,
		ContextSize:          opts.contextSize,
		Concurrency:          1,
		SentenceAwareChunks:  !opts.noSentenceAwareChunks && opts.chunkBoundaryPlanner != pipeline.ChunkBoundaryPlannerOff,
		MinChunkSize:         opts.minChunkSize,
		MaxChunkSize:         opts.maxChunkSize,
		ChunkBoundaryPlanner: opts.chunkBoundaryPlanner,
		NoPreprocess:         opts.noPreprocess,
		NoLangPreprocess:     opts.noLangPreprocess,
		SourceLang:           opts.sourceLangCode,
		TargetLang:           opts.targetLangCode,
		GlossaryRuns:         opts.runs,
		GlossaryWindowChunks: opts.windowChunks,
		GlossaryArtifactsDir: opts.artifactsDir,
		Overwrite:            opts.yes,
		OnConfirmOverwrite: func(path string) bool {
			confirmed, err := prompt.DefaultConfirmer().ConfirmOverwrite(path, opts.yes)
			if err != nil {
				logger.Error("Overwrite confirmation failed", "error", err)
				return false
			}
			return confirmed
		},
	}

	ctx, stop := signalContext()
	defer stop()
	started := time.Now()
	result, err := runGlossaryExtractionPipeline(ctx, cfg)
	printUsageStats(&result.Usage, time.Since(started), launchCfg.ModelAlias)
	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Glossary extraction canceled", "error", err)
			return nil
		}
		return err
	}
	return nil
}
