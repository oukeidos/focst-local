package main

import (
	"fmt"
	"os"
	"time"

	"github.com/oukeidos/focst-local/internal/files"
	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/names"
	"github.com/oukeidos/focst-local/internal/openai"
	"github.com/oukeidos/focst-local/internal/prompt"
	"github.com/spf13/cobra"
)

type namesOptions struct {
	workType   string
	title      string
	year       string
	sourceName string
	targetName string
	maxTokens  int
	allowEnv   bool
	envOnly    bool
	yes        bool
	debug      bool
}

func newNamesCmd() *cobra.Command {
	opts := namesOptions{}
	cmd := &cobra.Command{
		Use:   "names [options] <output.json>",
		Short: "Extract character name mappings using GPT-5.2",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.title == "" {
				_ = cmd.Usage()
				return fmt.Errorf("-title is required")
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return fmt.Errorf("output path is required")
			}
			return runNames(cmd, args, &opts)
		},
		SilenceUsage: true,
	}

	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.Flags().StringVar(&opts.workType, "type", "movie", "Type of work (movie, show, etc.)")
	cmd.Flags().StringVar(&opts.title, "title", "", "Title of the work")
	cmd.Flags().StringVar(&opts.year, "year", "", "Release year")
	cmd.Flags().StringVar(&opts.sourceName, "source", "Japanese", "Source language name (e.g. Japanese)")
	cmd.Flags().StringVar(&opts.targetName, "target", "Korean", "Target language name (e.g. Korean)")
	cmd.Flags().IntVar(&opts.maxTokens, "max-tokens", 16384, "Max output tokens including reasoning")
	cmd.Flags().BoolVar(&opts.allowEnv, "allow-env", false, "Allow reading API key from environment variables")
	cmd.Flags().BoolVar(&opts.envOnly, "env-only", false, "Use only environment variables for API keys")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Overwrite output file without asking")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging")
	return cmd
}

func runNames(cmd *cobra.Command, args []string, opts *namesOptions) error {
	startTime := time.Now()
	outputPath := args[0]

	allowOverwrite := opts.yes
	if !allowOverwrite {
		if _, err := os.Stat(outputPath); err == nil {
			confirmed, err := prompt.DefaultConfirmer().ConfirmOverwrite(outputPath, allowOverwrite)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Println("Aborted.")
				return nil
			}
			allowOverwrite = true
		}
	}
	originalOutputPath := outputPath
	pathChanged := false
	if !allowOverwrite {
		safePath, changed, err := files.SafePath(outputPath)
		if err != nil {
			return fmt.Errorf("failed to resolve output path: %w", err)
		}
		if changed {
			outputPath = safePath
			pathChanged = true
		}
	}
	if err := files.RejectSymlinkPath(outputPath); err != nil {
		return err
	}

	logLevel := logger.LevelInfo
	if opts.debug {
		logLevel = logger.LevelDebug
	}
	logger.Init(logLevel, nil)
	if pathChanged {
		logger.Warn("Output path adjusted to avoid overwrite", "original", originalOutputPath, "effective", outputPath)
	}

	const openAIMaxTokens = 128000
	maxTokensVal := opts.maxTokens
	if maxTokensVal > openAIMaxTokens {
		logger.Warn("Max tokens clamped", "requested", maxTokensVal, "effective", openAIMaxTokens)
		maxTokensVal = openAIMaxTokens
	}

	key, source, err := resolveAPIKey("openai", opts.allowEnv, opts.envOnly)
	if err != nil {
		return err
	}
	logger.Info("Using API Key", "service", "openai", "source", source)

	sourceCode, err := resolveLanguageCode(opts.sourceName)
	if err != nil {
		return err
	}
	targetCode, err := resolveLanguageCode(opts.targetName)
	if err != nil {
		return err
	}

	client := openai.NewClient(key, "gpt-5.2")
	extractor := names.NewExtractor(client)

	logger.Info("Extracting character names", "title", opts.title, "type", opts.workType)
	ctx, stop := signalContext()
	defer stop()
	mappings, usage, err := extractor.Extract(ctx, opts.workType, opts.title, opts.year, maxTokensVal, sourceCode, targetCode)
	if err != nil {
		if ctx.Err() != nil {
			logger.Warn("Name extraction canceled", "error", err)
			return nil
		}
		return err
	}

	data, err := names.EncodeMappings(mappings, sourceCode, targetCode)
	if err != nil {
		return err
	}

	if err := files.AtomicWrite(outputPath, data, 0600); err != nil {
		return err
	}

	logger.Info("Success", "count", len(mappings), "path", outputPath)

	fmt.Println("\n--- Execution Stats ---")
	fmt.Printf("Time: %s\n", time.Since(startTime))
	fmt.Printf("Token Usage: In=%d, Out=%d, Total=%d\n", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	if usage.WebSearchCalls > 0 {
		fmt.Printf("Web Search Calls: %d\n", usage.WebSearchCalls)
	}

	cost := estimateOpenAICost(client.GetModelID(), usage)
	fmt.Printf("Estimated Cost: $%.5f\n", cost)
	return nil
}
