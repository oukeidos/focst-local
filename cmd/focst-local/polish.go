package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/oukeidos/focst-local/internal/chunker"
	"github.com/oukeidos/focst-local/internal/cleanup"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/glossary"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/pipeline"
	"github.com/oukeidos/focst-local/internal/postpolish"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/spf13/cobra"
)

type polishOptions struct {
	modelName                   string
	baseURL                     string
	translationTimeout          time.Duration
	sourceLangCode              string
	targetLangCode              string
	namesPath                   string
	glossaryFilePath            string
	postPolishProfile           string
	savePolishCorrectionsPath   string
	polishArtifactsDir          string
	polishBroadChunkSize        int
	polishRepairChunkSize       int
	polishMaxTokens             int
	polishChunkSize             int
	polishMinChunkSize          int
	polishMaxChunkSize          int
	noPolishSentenceAwareChunks bool
	polishChunkBoundaryPlanner  string
	logFilePath                 string
	yes                         bool
	noPreprocess                bool
	noLangPreprocess            bool
	debug                       bool
	llama                       llamaServerOptions
}

func newPolishCmd() *cobra.Command {
	opts := polishOptions{}
	cmd := &cobra.Command{
		Use:   "polish <source.srt> <translated.srt> <polished.srt>",
		Short: "Apply experimental local post-translation polish to an existing translation",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 3 {
				_ = cmd.Usage()
				return fmt.Errorf("source, translated input, and polished output files are required")
			}
			return runPolish(cmd, args, &opts)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addPolishFlags(cmd, &opts)
	return cmd
}

func addPolishFlags(cmd *cobra.Command, opts *polishOptions) {
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().StringVar(&opts.modelName, "model", localllm.DefaultModel, "Local model name")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per polish request; 0 disables the timeout")
	cmd.Flags().StringVar(&opts.sourceLangCode, "source", "ja", "Source language code (default: ja)")
	cmd.Flags().StringVar(&opts.targetLangCode, "target", "ko", "Target language code (default: ko)")
	cmd.Flags().StringVar(&opts.namesPath, "names", "", "Path to character name mapping JSON file")
	cmd.Flags().StringVar(&opts.glossaryFilePath, "glossary-file", "", "Path to an existing local glossary JSON file to use as a polish guard")
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
	cmd.Flags().StringVar(&opts.logFilePath, "log-file", "", "Path to save machine-readable JSONL logs")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite output file without asking")
	cmd.Flags().BoolVar(&opts.noPreprocess, "no-preprocess", false, "Disable all preprocessing")
	cmd.Flags().BoolVar(&opts.noLangPreprocess, "no-lang-preprocess", false, "Disable language-specific preprocessing only")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
}

func runPolish(cmd *cobra.Command, args []string, opts *polishOptions) error {
	if err := validateSubtitlePathExtensions(args[1], args[2]); err != nil {
		return err
	}
	if err := validateSubtitleExtension("source", args[0]); err != nil {
		return err
	}
	if err := validatePostPolishProfileFlags(cmd, opts.postPolishProfile, true); err != nil {
		return err
	}
	for _, path := range []string{args[2], opts.logFilePath, opts.glossaryFilePath, opts.savePolishCorrectionsPath, opts.polishArtifactsDir} {
		if path == "" {
			continue
		}
		if err := files.RejectSymlinkPath(path); err != nil {
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

	srcLang, ok := language.GetLanguage(opts.sourceLangCode)
	if !ok {
		return fmt.Errorf("unsupported source language: %s", opts.sourceLangCode)
	}
	tgtLang, ok := language.GetLanguage(opts.targetLangCode)
	if !ok {
		return fmt.Errorf("unsupported target language: %s", opts.targetLangCode)
	}
	sourceSegments, err := srt.Load(args[0])
	if err != nil {
		return fmt.Errorf("failed to load source subtitle file: %w", err)
	}
	if err := srt.Validate(sourceSegments); err != nil {
		return fmt.Errorf("invalid source subtitle file: %w", err)
	}
	if !opts.noPreprocess {
		sourceSegments, _ = srt.PreprocessForPathWithMappingOptions(sourceSegments, srcLang.Code, args[0], !opts.noLangPreprocess)
	}
	translatedSegments, err := srt.Load(args[1])
	if err != nil {
		return fmt.Errorf("failed to load translated subtitle file: %w", err)
	}
	if err := srt.Validate(translatedSegments); err != nil {
		return fmt.Errorf("invalid translated subtitle file: %w", err)
	}

	protected := map[string]string{}
	if opts.glossaryFilePath != "" {
		artifact, err := glossary.LoadArtifact(opts.glossaryFilePath)
		if err != nil {
			return err
		}
		protected = mergePolishMappings(protected, glossary.Mapping(artifact.Entries))
	}
	if opts.namesPath != "" {
		nameMapping, err := loadNamesMapping(opts.namesPath, opts.sourceLangCode, opts.targetLangCode)
		if err != nil {
			return err
		}
		protected = mergePolishMappings(protected, nameMapping)
	}

	shouldOverwrite := opts.yes
	if _, err := os.Stat(args[2]); err == nil {
		confirmed, err := prompt.DefaultConfirmer().ConfirmOverwrite(args[2], opts.yes)
		if err != nil {
			return err
		}
		shouldOverwrite = confirmed
		if !shouldOverwrite {
			logger.Info("Output file exists. Aborted by user.", "path", args[2])
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat output path: %w", err)
	}

	launchCfg, err := buildLlamaLaunchConfig(cmd, opts.llama, opts.modelName, opts.baseURL)
	if err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	server, cleanupServer, err := llamaserver.NewManager().Ensure(ctx, launchCfg)
	if err != nil {
		return fmt.Errorf("local LLM is not ready: %w", err)
	}
	defer func() {
		if cleanupServer != nil {
			if err := cleanupServer(ctx); err != nil {
				logger.Warn("Failed to clean up managed llama.cpp server", "error", err)
			}
		}
	}()
	client := localllm.NewClient(server.BaseURL, launchCfg.ModelAlias)
	client.SetTranslationTimeout(opts.translationTimeout)

	profile, _ := postpolish.NormalizeProfile(opts.postPolishProfile)
	var polishPlan chunker.ChunkPlan
	if postpolish.NeedsChunkPlan(profile) {
		if opts.polishChunkSize <= 0 {
			return fmt.Errorf("polish chunk size must be greater than 0")
		}
		if opts.polishMinChunkSize <= 0 || opts.polishMaxChunkSize <= 0 || opts.polishMinChunkSize > opts.polishMaxChunkSize {
			return fmt.Errorf("invalid polish chunk range: %d..%d", opts.polishMinChunkSize, opts.polishMaxChunkSize)
		}
		if opts.polishChunkSize < opts.polishMinChunkSize || opts.polishChunkSize > opts.polishMaxChunkSize {
			return fmt.Errorf("polish chunk size must be inside polish sentence-aware range, got %d range=%d..%d", opts.polishChunkSize, opts.polishMinChunkSize, opts.polishMaxChunkSize)
		}
		switch opts.polishChunkBoundaryPlanner {
		case pipeline.ChunkBoundaryPlannerOff, pipeline.ChunkBoundaryPlannerDeterministic, pipeline.ChunkBoundaryPlannerLocalLLM:
		default:
			return fmt.Errorf("invalid polish chunk boundary planner: %s", opts.polishChunkBoundaryPlanner)
		}
		planOptions := chunker.PlanOptions{
			NominalSize:         opts.polishChunkSize,
			MinSize:             opts.polishMinChunkSize,
			MaxSize:             opts.polishMaxChunkSize,
			ContextSize:         0,
			EnableSentenceAware: !opts.noPolishSentenceAwareChunks && opts.polishChunkBoundaryPlanner != pipeline.ChunkBoundaryPlannerOff,
		}
		var boundaryPlanner chunker.BoundaryPlanner
		if planOptions.EnableSentenceAware && opts.polishChunkBoundaryPlanner == pipeline.ChunkBoundaryPlannerLocalLLM {
			boundaryPlanner = client
		}
		_, plan, err := chunker.PlanChunks(ctx, sourceSegments, planOptions, boundaryPlanner)
		if err != nil {
			return fmt.Errorf("failed to plan post-polish chunks: %w", err)
		}
		polishPlan = plan
	}

	started := time.Now()
	result, err := postpolish.Run(ctx, client, sourceSegments, translatedSegments, postpolish.Config{
		SourceLanguage:      srcLang,
		TargetLanguage:      tgtLang,
		Model:               launchCfg.ModelAlias,
		BaseURL:             server.BaseURL,
		Profile:             profile,
		ChunkPlan:           polishPlan,
		ChunkSize:           opts.polishChunkSize,
		BroadChunkSize:      opts.polishBroadChunkSize,
		RepairChunkSize:     opts.polishRepairChunkSize,
		MaxTokens:           opts.polishMaxTokens,
		ArtifactDir:         opts.polishArtifactsDir,
		ProtectedRenderings: protected,
	})
	printUsageStats(&result.Usage, time.Since(started), launchCfg.ModelAlias)
	if err != nil {
		return err
	}
	outSegments := postpolish.Apply(translatedSegments, result.Accepted)
	if err := srt.Save(args[2], outSegments); err != nil {
		return fmt.Errorf("failed to save polished output file: %w", err)
	}
	if opts.savePolishCorrectionsPath != "" {
		if err := postpolish.SaveArtifact(opts.savePolishCorrectionsPath, result.Artifact); err != nil {
			return err
		}
	}
	logger.Info("Saved polished output", "path", args[2], "accepted", len(result.Accepted), "rejected", len(result.Rejected))
	return nil
}

func mergePolishMappings(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for source, target := range base {
		out[source] = target
	}
	for source, target := range override {
		out[source] = target
	}
	return out
}
