package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/cleanup"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/phraseanchor"
	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/postpolish"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/oukeidos/focst-local/internal/translator"
	"github.com/spf13/cobra"
)

type translateOptions struct {
	modelName                            string
	baseURL                              string
	maxTokens                            int
	translationTimeout                   time.Duration
	chunkSize                            int
	contextSize                          int
	concurrency                          int
	noSentenceAwareChunks                bool
	minChunkSize                         int
	maxChunkSize                         int
	chunkBoundaryPlanner                 string
	yes                                  bool
	logFilePath                          string
	namesPath                            string
	autoGlossary                         bool
	saveGlossaryPath                     string
	glossaryFilePath                     string
	glossaryArtifactsDir                 string
	glossaryRuns                         int
	glossaryWindowChunks                 int
	autoPhraseAnchors                    bool
	savePhraseAnchorsPath                string
	phraseAnchorsFilePath                string
	phraseAnchorsArtifactsDir            string
	phraseAnchorThesisRounds             int
	phraseAnchorVotes                    int
	phraseAnchorQuoteFilterBatchSize     int
	phraseAnchorProperFilterRuns         int
	phraseAnchorProperFilterWindowChunks int
	postPolish                           bool
	postPolishProfile                    string
	savePolishCorrectionsPath            string
	polishArtifactsDir                   string
	polishBroadChunkSize                 int
	polishRepairChunkSize                int
	polishMaxTokens                      int
	polishChunkSize                      int
	polishMinChunkSize                   int
	polishMaxChunkSize                   int
	noPolishSentenceAwareChunks          bool
	polishChunkBoundaryPlanner           string
	repairResidue                        bool
	residueScripts                       string
	saveResidueCandidatesPath            string
	residueReportPath                    string
	noPreprocess                         bool
	noPostprocess                        bool
	noLangPreprocess                     bool
	noLangPostprocess                    bool
	sourceLangCode                       string
	targetLangCode                       string
	debug                                bool
	llama                                llamaServerOptions
}

func newTranslateCmd() *cobra.Command {
	opts := translateOptions{}
	cmd := &cobra.Command{
		Use:   "translate <input.srt> <output.srt>",
		Short: "Translate subtitle files using a local llama.cpp server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				_ = cmd.Usage()
				return fmt.Errorf("input and output files are required")
			}
			return runTranslate(cmd, args, &opts)
		},
		SilenceUsage: true,
	}

	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addTranslateFlags(cmd, &opts)
	return cmd
}

func addTranslateFlags(cmd *cobra.Command, opts *translateOptions) {
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().StringVar(&opts.modelName, "model", localllm.DefaultModel, "Local model name")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", localllm.DefaultMaxTokens, "Maximum generated tokens per request")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per translation request; 0 disables the timeout")
	cmd.Flags().IntVar(&opts.chunkSize, "chunk-size", 100, "Number of segments per chunk")
	cmd.Flags().IntVar(&opts.contextSize, "context-size", 5, "Number of context segments before/after")
	cmd.Flags().IntVar(&opts.concurrency, "concurrency", 1, "Number of concurrent local LLM requests (1-20)")
	cmd.Flags().BoolVar(&opts.noSentenceAwareChunks, "no-sentence-aware-chunks", false, "Use exact fixed-size chunks without sentence-aware boundary planning")
	cmd.Flags().IntVar(&opts.minChunkSize, "min-chunk-size", 0, "Minimum target chunk size for sentence-aware planning (default: chunk-size - 10)")
	cmd.Flags().IntVar(&opts.maxChunkSize, "max-chunk-size", 0, "Maximum target chunk size for sentence-aware planning (default: chunk-size + 10)")
	cmd.Flags().StringVar(&opts.chunkBoundaryPlanner, "chunk-boundary-planner", pipeline.ChunkBoundaryPlannerLocalLLM, "Boundary planner: local-llm, deterministic, or off")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite output file without asking")
	cmd.Flags().StringVar(&opts.logFilePath, "log-file", "", "Path to save machine-readable JSONL logs")
	cmd.Flags().StringVar(&opts.namesPath, "names", "", "Path to character name mapping JSON file")
	cmd.Flags().BoolVar(&opts.autoGlossary, "auto-glossary", false, "Generate and use a local glossary for this translation")
	cmd.Flags().StringVar(&opts.saveGlossaryPath, "save-glossary", "", "Path to save the generated local glossary JSON")
	cmd.Flags().StringVar(&opts.glossaryFilePath, "glossary-file", "", "Path to an existing local glossary JSON file to use")
	cmd.Flags().StringVar(&opts.glossaryArtifactsDir, "glossary-artifacts", "", "Directory for local glossary debug artifacts")
	cmd.Flags().IntVar(&opts.glossaryRuns, "glossary-runs", 3, "Number of local glossary extraction runs per glossary window")
	cmd.Flags().IntVar(&opts.glossaryWindowChunks, "glossary-window-chunks", 3, "Number of translation chunks per glossary extraction window")
	cmd.Flags().BoolVar(&opts.autoPhraseAnchors, "auto-phrase-anchors", false, "Generate and use local phrase anchors for this translation")
	cmd.Flags().StringVar(&opts.savePhraseAnchorsPath, "save-phrase-anchors", "", "Path to save the generated phrase anchors JSON")
	cmd.Flags().StringVar(&opts.phraseAnchorsFilePath, "phrase-anchors-file", "", "Path to an existing phrase anchors JSON file to use")
	cmd.Flags().StringVar(&opts.phraseAnchorsArtifactsDir, "phrase-anchors-artifacts", "", "Directory for phrase anchors debug artifacts")
	cmd.Flags().IntVar(&opts.phraseAnchorThesisRounds, "phrase-anchor-thesis-rounds", phraseanchor.DefaultThesisRounds, "Number of phrase anchor candidate discovery rounds")
	cmd.Flags().IntVar(&opts.phraseAnchorVotes, "phrase-anchor-votes", phraseanchor.DefaultSynthesisVotes, "Number of phrase anchor A/B synthesis votes")
	cmd.Flags().IntVar(&opts.phraseAnchorQuoteFilterBatchSize, "phrase-anchor-quote-filter-batch-size", phraseanchor.DefaultQuoteFilterBatchSize, "Batch size for phrase anchor quote-kind filtering")
	cmd.Flags().IntVar(&opts.phraseAnchorProperFilterRuns, "phrase-anchor-proper-filter-runs", phraseanchor.DefaultProperFilterRuns, "Number of phrase anchor source-name filter runs")
	cmd.Flags().IntVar(&opts.phraseAnchorProperFilterWindowChunks, "phrase-anchor-proper-filter-window-chunks", phraseanchor.DefaultProperFilterWindowChunks, "Number of translation chunks per phrase anchor source-name filter window")
	cmd.Flags().BoolVar(&opts.postPolish, "post-polish", false, "Run experimental local post-translation polish after successful translation")
	cmd.Flags().StringVar(&opts.postPolishProfile, "post-polish-profile", string(postpolish.ProfileSegmentLocal), "Post-polish profile: segment-local, chunk-flow, or legacy")
	cmd.Flags().StringVar(&opts.savePolishCorrectionsPath, "save-polish-corrections", "", "Path to save accepted/rejected post-polish corrections JSON")
	cmd.Flags().StringVar(&opts.polishArtifactsDir, "polish-artifacts", "", "Directory for post-polish debug artifacts")
	cmd.Flags().IntVar(&opts.polishBroadChunkSize, "polish-broad-chunk-size", postpolish.DefaultBroadChunkSize, "Segments per broad post-polish request")
	cmd.Flags().IntVar(&opts.polishRepairChunkSize, "polish-repair-chunk-size", postpolish.DefaultRepairChunkSize, "Segments per repair post-polish request")
	cmd.Flags().IntVar(&opts.polishMaxTokens, "polish-max-tokens", 0, "Maximum generated tokens per post-polish request (default: profile-specific)")
	cmd.Flags().IntVar(&opts.polishChunkSize, "polish-chunk-size", postpolish.DefaultV2ChunkSize, "Target chunk size for v2 post-polish profiles")
	cmd.Flags().IntVar(&opts.polishMinChunkSize, "polish-min-chunk-size", postpolish.DefaultV2MinChunkSize, "Minimum chunk size for v2 sentence-aware post-polish planning")
	cmd.Flags().IntVar(&opts.polishMaxChunkSize, "polish-max-chunk-size", postpolish.DefaultV2MaxChunkSize, "Maximum chunk size for v2 sentence-aware post-polish planning")
	cmd.Flags().BoolVar(&opts.noPolishSentenceAwareChunks, "no-polish-sentence-aware-chunks", false, "Use fixed-size chunks for v2 post-polish profiles")
	cmd.Flags().StringVar(&opts.polishChunkBoundaryPlanner, "polish-chunk-boundary-planner", pipeline.ChunkBoundaryPlannerLocalLLM, "Post-polish boundary planner: local-llm, deterministic, or off")
	cmd.Flags().BoolVar(&opts.repairResidue, "repair-residue", false, "Detect and repair selected source-script residue after translation")
	cmd.Flags().StringVar(&opts.residueScripts, "residue-scripts", "", "Unicode scripts to scan for source residue, comma-separated, or auto")
	cmd.Flags().StringVar(&opts.saveResidueCandidatesPath, "save-residue-candidates", "", "Path to save detected residue candidates JSON")
	cmd.Flags().StringVar(&opts.residueReportPath, "residue-report", "", "Path to save a residue repair Markdown report")
	cmd.Flags().BoolVar(&opts.noPreprocess, "no-preprocess", false, "Disable all preprocessing (bracket removal, symbol filtering)")
	cmd.Flags().BoolVar(&opts.noLangPreprocess, "no-lang-preprocess", false, "Disable language-specific preprocessing only")
	cmd.Flags().BoolVar(&opts.noPostprocess, "no-postprocess", false, "Disable all post-processing (punctuation, timing correction)")
	cmd.Flags().BoolVar(&opts.noLangPostprocess, "no-lang-postprocess", false, "Disable language-specific post-processing only")
	cmd.Flags().StringVar(&opts.sourceLangCode, "source", "ja", "Source language code (default: ja)")
	cmd.Flags().StringVar(&opts.targetLangCode, "target", "ko", "Target language code (default: ko)")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
}

func runTranslate(cmd *cobra.Command, args []string, opts *translateOptions) error {
	if len(args) < 2 {
		return fmt.Errorf("input and output files are required")
	}
	if len(args) > 2 {
		fmt.Fprintf(os.Stderr, "Warning: expected 2 arguments but got %d. Did you forget quotes around file paths?\n", len(args))
		fmt.Fprintf(os.Stderr, "  Using input: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "  Using output: %s\n", args[1])
	}
	if err := validateSubtitlePathExtensions(args[0], args[1]); err != nil {
		return err
	}

	logLevel := logger.LevelInfo
	if opts.debug {
		logLevel = logger.LevelDebug
	}
	var logFileW io.Writer
	if opts.logFilePath != "" {
		if err := files.RejectSymlinkPath(opts.logFilePath); err != nil {
			return err
		}
		f, err := os.OpenFile(opts.logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		cleanup.Register(f.Close)
		logFileW = f
	}
	logger.Init(logLevel, logFileW)

	startTime := time.Now()

	var nameMapping map[string]string
	var err error
	if opts.namesPath != "" {
		nameMapping, err = loadNamesMapping(opts.namesPath, opts.sourceLangCode, opts.targetLangCode)
		if err != nil {
			return err
		}
	}
	launchCfg, err := buildLlamaLaunchConfig(cmd, opts.llama, opts.modelName, opts.baseURL)
	if err != nil {
		return err
	}
	if err := validatePostPolishProfileFlags(cmd, opts.postPolishProfile, opts.postPolish); err != nil {
		return err
	}

	cfg := pipeline.Config{
		InputPath:                            args[0],
		OutputPath:                           args[1],
		LogPath:                              opts.logFilePath,
		BaseURL:                              launchCfg.BaseURL,
		Model:                                launchCfg.ModelAlias,
		LlamaServer:                          launchCfg,
		MaxTokens:                            opts.maxTokens,
		TranslationTimeout:                   opts.translationTimeout,
		ChunkSize:                            opts.chunkSize,
		ContextSize:                          opts.contextSize,
		Concurrency:                          opts.concurrency,
		SentenceAwareChunks:                  !opts.noSentenceAwareChunks && opts.chunkBoundaryPlanner != pipeline.ChunkBoundaryPlannerOff,
		MinChunkSize:                         opts.minChunkSize,
		MaxChunkSize:                         opts.maxChunkSize,
		ChunkBoundaryPlanner:                 opts.chunkBoundaryPlanner,
		NoPreprocess:                         opts.noPreprocess,
		NoPostprocess:                        opts.noPostprocess,
		NoLangPreprocess:                     opts.noLangPreprocess,
		NoLangPostprocess:                    opts.noLangPostprocess,
		Overwrite:                            opts.yes,
		SourceLang:                           opts.sourceLangCode,
		TargetLang:                           opts.targetLangCode,
		NamesMapping:                         nameMapping,
		NamesPath:                            opts.namesPath,
		AutoGlossary:                         opts.autoGlossary,
		SaveGlossaryPath:                     opts.saveGlossaryPath,
		GlossaryPath:                         opts.glossaryFilePath,
		GlossaryArtifactsDir:                 opts.glossaryArtifactsDir,
		GlossaryRuns:                         opts.glossaryRuns,
		GlossaryWindowChunks:                 opts.glossaryWindowChunks,
		AutoPhraseAnchors:                    opts.autoPhraseAnchors,
		SavePhraseAnchorsPath:                opts.savePhraseAnchorsPath,
		PhraseAnchorsPath:                    opts.phraseAnchorsFilePath,
		PhraseAnchorsArtifactsDir:            opts.phraseAnchorsArtifactsDir,
		PhraseAnchorThesisRounds:             opts.phraseAnchorThesisRounds,
		PhraseAnchorVotes:                    opts.phraseAnchorVotes,
		PhraseAnchorQuoteFilterBatchSize:     opts.phraseAnchorQuoteFilterBatchSize,
		PhraseAnchorProperFilterRuns:         opts.phraseAnchorProperFilterRuns,
		PhraseAnchorProperFilterWindowChunks: opts.phraseAnchorProperFilterWindowChunks,
		PostPolish:                           opts.postPolish,
		PostPolishProfile:                    opts.postPolishProfile,
		SavePolishCorrectionsPath:            opts.savePolishCorrectionsPath,
		PolishArtifactsDir:                   opts.polishArtifactsDir,
		PolishBroadChunkSize:                 opts.polishBroadChunkSize,
		PolishRepairChunkSize:                opts.polishRepairChunkSize,
		PolishMaxTokens:                      opts.polishMaxTokens,
		PolishChunkSize:                      opts.polishChunkSize,
		PolishMinChunkSize:                   opts.polishMinChunkSize,
		PolishMaxChunkSize:                   opts.polishMaxChunkSize,
		PolishSentenceAwareChunks:            !opts.noPolishSentenceAwareChunks && opts.polishChunkBoundaryPlanner != pipeline.ChunkBoundaryPlannerOff,
		PolishChunkBoundaryPlanner:           opts.polishChunkBoundaryPlanner,
		RepairResidue:                        opts.repairResidue,
		ResidueScripts:                       opts.residueScripts,
		SaveResidueCandidatesPath:            opts.saveResidueCandidatesPath,
		ResidueReportPath:                    opts.residueReportPath,
		OnProgress: func(p translator.TranslationProgress) {
			switch p.State {
			case translator.StateCompleted:
				logger.Info("Chunk completed", "index", p.ChunkIndex, "total", p.TotalChunks)
			case translator.StateInProgress:
				logger.Warn("Chunk retry", "index", p.ChunkIndex, "attempt", p.Attempt, "error", p.Error)
			}
		},
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
	result, err := pipeline.RunTranslation(ctx, cfg)

	// Always print stats (even on partial success)
	printUsageStats(&result.Usage, time.Since(startTime), launchCfg.ModelAlias)

	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Translation canceled", "error", err)
			return nil
		}
		return err
	}

	return translationStatusError(result)
}

func translationStatusError(result pipeline.TranslationResult) error {
	switch result.Status {
	case pipeline.TranslationStatusSuccess:
		return nil
	case pipeline.TranslationStatusSkipped:
		return nil
	case pipeline.TranslationStatusPartialSuccess, pipeline.TranslationStatusFailure:
		if result.RecoveryLogPath != "" {
			return fmt.Errorf("translation finished with status: %s (recovery log: %s)", result.Status, result.RecoveryLogPath)
		}
		return fmt.Errorf("translation finished with status: %s", result.Status)
	default:
		return fmt.Errorf("translation finished with unknown status: %q", result.Status)
	}
}

var supportedSubtitleExtensions = map[string]struct{}{
	".srt":  {},
	".vtt":  {},
	".ssa":  {},
	".ass":  {},
	".ttml": {},
	".stl":  {},
}

const supportedSubtitleExtensionsLabel = ".srt, .vtt, .ssa, .ass, .ttml, .stl"

func validateSubtitlePathExtensions(inputPath, outputPath string) error {
	if err := validateSubtitleExtension("input", inputPath); err != nil {
		return err
	}
	if err := validateSubtitleExtension("output", outputPath); err != nil {
		return err
	}
	return nil
}

func validateSubtitleExtension(kind, path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := supportedSubtitleExtensions[ext]; ok {
		return nil
	}
	if ext == "" {
		ext = "(none)"
	}
	return fmt.Errorf("unsupported %s extension %q (supported: %s)", kind, ext, supportedSubtitleExtensionsLabel)
}
