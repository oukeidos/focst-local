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
	"github.com/oukeidos/focst-local/internal/phraseanchor"
	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/spf13/cobra"
)

var runPhraseAnchorExtractionPipeline = pipeline.RunPhraseAnchorExtraction

type phraseAnchorOptions struct {
	modelName                string
	baseURL                  string
	maxTokens                int
	translationTimeout       time.Duration
	chunkSize                int
	contextSize              int
	noSentenceAwareChunks    bool
	minChunkSize             int
	maxChunkSize             int
	chunkBoundaryPlanner     string
	sourceLangCode           string
	targetLangCode           string
	thesisRounds             int
	votes                    int
	quoteFilterBatchSize     int
	properFilterRuns         int
	properFilterWindowChunks int
	artifactsDir             string
	logFilePath              string
	yes                      bool
	noPreprocess             bool
	noLangPreprocess         bool
	debug                    bool
	llama                    llamaServerOptions
}

func newPhraseAnchorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phrase-anchors",
		Short: "Generate and inspect local phrase anchor artifacts",
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.AddCommand(newPhraseAnchorsExtractCmd())
	return cmd
}

func newPhraseAnchorsExtractCmd() *cobra.Command {
	opts := phraseAnchorOptions{}
	cmd := &cobra.Command{
		Use:   "extract <input.srt> <phrase-anchors.json>",
		Short: "Extract local phrase anchors without translating",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				_ = cmd.Usage()
				return fmt.Errorf("input subtitle and phrase anchors output are required")
			}
			return runPhraseAnchorsExtract(cmd, args, &opts)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addPhraseAnchorsExtractFlags(cmd, &opts)
	return cmd
}

func addPhraseAnchorsExtractFlags(cmd *cobra.Command, opts *phraseAnchorOptions) {
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().StringVar(&opts.modelName, "model", localllm.DefaultModel, "Local model name")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", localllm.DefaultMaxTokens, "Maximum generated tokens per request")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per phrase anchors request; 0 disables the timeout")
	cmd.Flags().IntVar(&opts.chunkSize, "chunk-size", 100, "Number of segments per translation chunk")
	cmd.Flags().IntVar(&opts.contextSize, "context-size", 5, "Number of context segments before/after")
	cmd.Flags().BoolVar(&opts.noSentenceAwareChunks, "no-sentence-aware-chunks", false, "Use exact fixed-size chunks without sentence-aware boundary planning")
	cmd.Flags().IntVar(&opts.minChunkSize, "min-chunk-size", 0, "Minimum target chunk size for sentence-aware planning (default: chunk-size - 10)")
	cmd.Flags().IntVar(&opts.maxChunkSize, "max-chunk-size", 0, "Maximum target chunk size for sentence-aware planning (default: chunk-size + 10)")
	cmd.Flags().StringVar(&opts.chunkBoundaryPlanner, "chunk-boundary-planner", pipeline.ChunkBoundaryPlannerLocalLLM, "Boundary planner: local-llm, deterministic, or off")
	cmd.Flags().StringVar(&opts.sourceLangCode, "source", "ja", "Source language code (default: ja)")
	cmd.Flags().StringVar(&opts.targetLangCode, "target", "ko", "Target language code (default: ko)")
	cmd.Flags().IntVar(&opts.thesisRounds, "phrase-anchor-thesis-rounds", phraseanchor.DefaultThesisRounds, "Number of phrase anchor candidate discovery rounds")
	cmd.Flags().IntVar(&opts.votes, "phrase-anchor-votes", phraseanchor.DefaultSynthesisVotes, "Number of phrase anchor A/B synthesis votes")
	cmd.Flags().IntVar(&opts.quoteFilterBatchSize, "phrase-anchor-quote-filter-batch-size", phraseanchor.DefaultQuoteFilterBatchSize, "Batch size for phrase anchor quote-kind filtering")
	cmd.Flags().IntVar(&opts.properFilterRuns, "phrase-anchor-proper-filter-runs", phraseanchor.DefaultProperFilterRuns, "Number of phrase anchor source-name filter runs")
	cmd.Flags().IntVar(&opts.properFilterWindowChunks, "phrase-anchor-proper-filter-window-chunks", phraseanchor.DefaultProperFilterWindowChunks, "Number of translation chunks per phrase anchor source-name filter window")
	cmd.Flags().StringVar(&opts.artifactsDir, "phrase-anchors-artifacts", "", "Directory for phrase anchors debug artifacts")
	cmd.Flags().StringVar(&opts.logFilePath, "log-file", "", "Path to save machine-readable JSONL logs")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite phrase anchors output file without asking")
	cmd.Flags().BoolVar(&opts.noPreprocess, "no-preprocess", false, "Disable all preprocessing")
	cmd.Flags().BoolVar(&opts.noLangPreprocess, "no-lang-preprocess", false, "Disable language-specific preprocessing only")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
}

func runPhraseAnchorsExtract(cmd *cobra.Command, args []string, opts *phraseAnchorOptions) error {
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
		InputPath:                            args[0],
		OutputPath:                           args[1],
		BaseURL:                              launchCfg.BaseURL,
		Model:                                launchCfg.ModelAlias,
		LlamaServer:                          launchCfg,
		MaxTokens:                            opts.maxTokens,
		TranslationTimeout:                   opts.translationTimeout,
		ChunkSize:                            opts.chunkSize,
		ContextSize:                          opts.contextSize,
		Concurrency:                          1,
		SentenceAwareChunks:                  !opts.noSentenceAwareChunks && opts.chunkBoundaryPlanner != pipeline.ChunkBoundaryPlannerOff,
		MinChunkSize:                         opts.minChunkSize,
		MaxChunkSize:                         opts.maxChunkSize,
		ChunkBoundaryPlanner:                 opts.chunkBoundaryPlanner,
		NoPreprocess:                         opts.noPreprocess,
		NoLangPreprocess:                     opts.noLangPreprocess,
		SourceLang:                           opts.sourceLangCode,
		TargetLang:                           opts.targetLangCode,
		PhraseAnchorThesisRounds:             opts.thesisRounds,
		PhraseAnchorVotes:                    opts.votes,
		PhraseAnchorQuoteFilterBatchSize:     opts.quoteFilterBatchSize,
		PhraseAnchorProperFilterRuns:         opts.properFilterRuns,
		PhraseAnchorProperFilterWindowChunks: opts.properFilterWindowChunks,
		PhraseAnchorsArtifactsDir:            opts.artifactsDir,
		Overwrite:                            opts.yes,
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
	result, err := runPhraseAnchorExtractionPipeline(ctx, cfg)
	printUsageStats(&result.Usage, time.Since(started), launchCfg.ModelAlias)
	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Phrase anchors extraction canceled", "error", err)
			return nil
		}
		return err
	}
	return nil
}
