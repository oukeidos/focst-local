package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/oukeidos/focst-local/internal/cleanup"
	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/glossary"
	"github.com/oukeidos/focst-local/internal/language"
	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/localllm"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/oukeidos/focst-local/internal/residue"
	"github.com/oukeidos/focst-local/internal/srt"
	"github.com/spf13/cobra"
)

type residueOptions struct {
	sourceLangCode        string
	targetLangCode        string
	residueScripts        string
	saveCandidatesPath    string
	residueReportPath     string
	residueCandidatesPath string
	modelName             string
	baseURL               string
	maxTokens             int
	translationTimeout    time.Duration
	namesPath             string
	glossaryFilePath      string
	logFilePath           string
	yes                   bool
	noPreprocess          bool
	noLangPreprocess      bool
	debug                 bool
	llama                 llamaServerOptions
}

func newResidueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "residue",
		Short: "Detect or repair untranslated source-script residue in translated subtitles",
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.AddCommand(newResidueDetectCmd(), newResidueRepairCmd())
	return cmd
}

func newResidueDetectCmd() *cobra.Command {
	opts := residueOptions{}
	cmd := &cobra.Command{
		Use:   "detect <source.srt> <translated.srt>",
		Short: "Detect source-script residue without calling a model",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				_ = cmd.Usage()
				return fmt.Errorf("source and translated files are required")
			}
			return runResidueDetect(cmd, args, &opts)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addResidueDetectFlags(cmd, &opts)
	return cmd
}

func newResidueRepairCmd() *cobra.Command {
	opts := residueOptions{}
	cmd := &cobra.Command{
		Use:   "repair <source.srt> <translated.srt> <repaired.srt>",
		Short: "Repair detected source-script residue with a local model",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 3 {
				_ = cmd.Usage()
				return fmt.Errorf("source, translated input, and repaired output files are required")
			}
			return runResidueRepair(cmd, args, &opts)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	addResidueRepairFlags(cmd, &opts)
	return cmd
}

func addResidueDetectFlags(cmd *cobra.Command, opts *residueOptions) {
	cmd.Flags().StringVar(&opts.sourceLangCode, "source", "ja", "Source language code (default: ja)")
	cmd.Flags().StringVar(&opts.targetLangCode, "target", "ko", "Target language code (default: ko)")
	cmd.Flags().StringVar(&opts.residueScripts, "residue-scripts", "", "Unicode scripts to scan, comma-separated, or auto")
	cmd.Flags().StringVar(&opts.saveCandidatesPath, "save-residue-candidates", "", "Path to save detected residue candidates JSON")
	cmd.Flags().StringVar(&opts.residueReportPath, "residue-report", "", "Path to save a residue Markdown report")
	cmd.Flags().BoolVar(&opts.noPreprocess, "no-preprocess", false, "Disable all preprocessing")
	cmd.Flags().BoolVar(&opts.noLangPreprocess, "no-lang-preprocess", false, "Disable language-specific preprocessing only")
}

func addResidueRepairFlags(cmd *cobra.Command, opts *residueOptions) {
	addResidueDetectFlags(cmd, opts)
	cmd.Flags().StringVar(&opts.residueCandidatesPath, "residue-candidates", "", "Path to existing residue candidates JSON")
	cmd.Flags().StringVar(&opts.baseURL, "llama-base-url", localllm.DefaultBaseURL, "OpenAI-compatible llama.cpp base URL")
	cmd.Flags().StringVar(&opts.modelName, "model", localllm.DefaultModel, "Local model name")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", residue.DefaultRepairMaxTokens, "Maximum generated tokens per residue repair request")
	cmd.Flags().DurationVar(&opts.translationTimeout, "translation-timeout", localllm.DefaultTranslationTimeout, "Timeout per residue repair request; 0 disables the timeout")
	cmd.Flags().StringVar(&opts.namesPath, "names", "", "Path to character name mapping JSON file to protect during repair")
	cmd.Flags().StringVar(&opts.glossaryFilePath, "glossary-file", "", "Path to an existing local glossary JSON file to protect during repair")
	cmd.Flags().StringVar(&opts.logFilePath, "log-file", "", "Path to save machine-readable JSONL logs")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite output file without asking")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	addLlamaServerFlags(cmd, &opts.llama)
}

func runResidueDetect(cmd *cobra.Command, args []string, opts *residueOptions) error {
	if err := validateSubtitleExtension("source", args[0]); err != nil {
		return err
	}
	if err := validateSubtitleExtension("translated", args[1]); err != nil {
		return err
	}
	if opts.residueScripts == "" {
		return fmt.Errorf("--residue-scripts is required")
	}
	if err := rejectResidueOutputSymlinks(opts.saveCandidatesPath, opts.residueReportPath); err != nil {
		return err
	}
	artifact, _, _, err := detectResidueFiles(args[0], args[1], opts)
	if err != nil {
		return err
	}
	if opts.saveCandidatesPath != "" {
		if err := residue.SaveArtifact(opts.saveCandidatesPath, artifact); err != nil {
			return err
		}
	}
	if opts.residueReportPath != "" {
		if err := residue.SaveMarkdownReport(opts.residueReportPath, artifact, nil); err != nil {
			return err
		}
	}
	if opts.saveCandidatesPath == "" && opts.residueReportPath == "" {
		fmt.Fprint(cmd.OutOrStdout(), residue.MarkdownReport(artifact, nil))
	}
	return nil
}

func runResidueRepair(cmd *cobra.Command, args []string, opts *residueOptions) error {
	if err := validateSubtitleExtension("source", args[0]); err != nil {
		return err
	}
	if err := validateSubtitleExtension("translated", args[1]); err != nil {
		return err
	}
	if err := validateSubtitleExtension("repaired", args[2]); err != nil {
		return err
	}
	if opts.residueScripts != "" && opts.residueCandidatesPath != "" {
		return fmt.Errorf("--residue-scripts and --residue-candidates cannot be used together")
	}
	if opts.residueScripts == "" && opts.residueCandidatesPath == "" {
		return fmt.Errorf("one of --residue-scripts or --residue-candidates is required")
	}
	for _, path := range []string{args[2], opts.logFilePath, opts.saveCandidatesPath, opts.residueReportPath, opts.residueCandidatesPath, opts.glossaryFilePath} {
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
	artifact, sourceSegments, translatedSegments, err := loadOrDetectResidue(args[0], args[1], opts)
	if err != nil {
		return err
	}
	if opts.saveCandidatesPath != "" {
		if err := residue.SaveArtifact(opts.saveCandidatesPath, artifact); err != nil {
			return err
		}
	}
	if len(artifact.Candidates) == 0 {
		logger.Info("No residue candidates detected", "event", "residue_candidates_empty")
		confirmed, err := maybeConfirmOutput(args[2], opts.yes)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
		if err := srt.Save(args[2], translatedSegments); err != nil {
			return fmt.Errorf("failed to save repaired output file: %w", err)
		}
		if opts.residueReportPath != "" {
			if err := residue.SaveMarkdownReport(opts.residueReportPath, artifact, nil); err != nil {
				return err
			}
		}
		return nil
	}
	protected, err := loadResidueProtectedMappings(opts)
	if err != nil {
		return err
	}
	confirmed, err := maybeConfirmOutput(args[2], opts.yes)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
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
	started := time.Now()
	srcLang, ok := language.GetLanguage(opts.sourceLangCode)
	if !ok {
		return fmt.Errorf("unsupported source language: %s", opts.sourceLangCode)
	}
	tgtLang, ok := language.GetLanguage(opts.targetLangCode)
	if !ok {
		return fmt.Errorf("unsupported target language: %s", opts.targetLangCode)
	}
	result, err := residue.Repair(ctx, client, sourceSegments, translatedSegments, artifact, residue.RepairOptions{
		SourceLanguage:      srcLang,
		TargetLanguage:      tgtLang,
		Model:               launchCfg.ModelAlias,
		BaseURL:             server.BaseURL,
		MaxTokens:           opts.maxTokens,
		ProtectedRenderings: protected,
	})
	printUsageStats(&result.Usage, time.Since(started), launchCfg.ModelAlias)
	if err != nil {
		return err
	}
	if err := srt.Save(args[2], result.Segments); err != nil {
		return fmt.Errorf("failed to save repaired output file: %w", err)
	}
	if opts.residueReportPath != "" {
		if err := residue.SaveMarkdownReport(opts.residueReportPath, artifact, result.Records); err != nil {
			return err
		}
	}
	logger.Info("Saved residue-repaired output", "path", args[2], "candidates", len(artifact.Candidates), "records", len(result.Records))
	return nil
}

func detectResidueFiles(sourcePath, translatedPath string, opts *residueOptions) (residue.Artifact, []srt.Segment, []srt.Segment, error) {
	srcLang, ok := language.GetLanguage(opts.sourceLangCode)
	if !ok {
		return residue.Artifact{}, nil, nil, fmt.Errorf("unsupported source language: %s", opts.sourceLangCode)
	}
	tgtLang, ok := language.GetLanguage(opts.targetLangCode)
	if !ok {
		return residue.Artifact{}, nil, nil, fmt.Errorf("unsupported target language: %s", opts.targetLangCode)
	}
	return residue.DetectFiles(residue.DetectOptions{
		SourceLanguage:   srcLang,
		TargetLanguage:   tgtLang,
		SourcePath:       sourcePath,
		TranslatedPath:   translatedPath,
		ScriptSpec:       opts.residueScripts,
		NoPreprocess:     opts.noPreprocess,
		NoLangPreprocess: opts.noLangPreprocess,
	})
}

func loadOrDetectResidue(sourcePath, translatedPath string, opts *residueOptions) (residue.Artifact, []srt.Segment, []srt.Segment, error) {
	if opts.residueCandidatesPath == "" {
		return detectResidueFiles(sourcePath, translatedPath, opts)
	}
	artifact, err := residue.LoadArtifact(opts.residueCandidatesPath)
	if err != nil {
		return residue.Artifact{}, nil, nil, err
	}
	loadOpts := *opts
	if loadOpts.residueScripts == "" {
		loadOpts.residueScripts = artifact.Config.ScriptSpec
		if loadOpts.residueScripts == "" {
			loadOpts.residueScripts = residue.AutoScripts
		}
	}
	_, sourceSegments, translatedSegments, err := detectResidueFiles(sourcePath, translatedPath, &loadOpts)
	if err != nil {
		return residue.Artifact{}, nil, nil, err
	}
	return artifact, sourceSegments, translatedSegments, nil
}

func rejectResidueOutputSymlinks(paths ...string) error {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := files.RejectSymlinkPath(path); err != nil {
			return err
		}
	}
	return nil
}

func maybeConfirmOutput(path string, yes bool) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		confirmed, err := prompt.DefaultConfirmer().ConfirmOverwrite(path, yes)
		if err != nil {
			return false, err
		}
		if !confirmed {
			logger.Info("Output file exists. Aborted by user.", "path", path)
			return false, nil
		}
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to stat output path: %w", err)
	}
	return true, nil
}

func loadResidueProtectedMappings(opts *residueOptions) (map[string]string, error) {
	protected := map[string]string{}
	if opts.glossaryFilePath != "" {
		artifact, err := glossary.LoadArtifact(opts.glossaryFilePath)
		if err != nil {
			return nil, err
		}
		protected = mergePolishMappings(protected, glossary.Mapping(artifact.Entries))
	}
	if opts.namesPath != "" {
		nameMapping, err := loadNamesMapping(opts.namesPath, opts.sourceLangCode, opts.targetLangCode)
		if err != nil {
			return nil, err
		}
		protected = mergePolishMappings(protected, nameMapping)
	}
	return protected, nil
}
