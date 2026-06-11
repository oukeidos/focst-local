package main

import (
	"fmt"
	"strings"

	"github.com/oukeidos/focst-local/internal/auth"
	"github.com/spf13/cobra"
)

type envOptions struct {
	service string
}

func newEnvCmd() *cobra.Command {
	opts := envOptions{}
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage API keys in OS Keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvStatus(cmd, &opts)
		},
	}

	cmd.SetUsageTemplate(envUsageTemplate)
	cmd.PersistentFlags().StringVar(&opts.service, "service", "openai", "Service to manage (openai)")

	cmd.AddCommand(
		newEnvSetupCmd(&opts),
		newEnvDeleteCmd(&opts),
		newEnvStatusCmd(&opts),
	)
	return cmd
}

func newEnvSetupCmd(opts *envOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Save API key to keychain (prompt only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvSetup(cmd, opts)
		},
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}

func newEnvDeleteCmd(opts *envOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete key from keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvDelete(cmd, opts)
		},
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}

func newEnvStatusCmd(opts *envOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show key status (default if no action given)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvStatus(cmd, opts)
		},
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}

func runEnvSetup(cmd *cobra.Command, opts *envOptions) error {
	svc := strings.ToLower(opts.service)
	if svc != "openai" {
		return fmt.Errorf("invalid service. Must be 'openai'")
	}

	svcName := "OpenAI"
	promptKey, err := auth.PromptForAPIKey(fmt.Sprintf("%s API Key: ", svcName))
	if err != nil {
		return fmt.Errorf("error reading key: %w", err)
	}
	key := strings.TrimSpace(promptKey)
	if key == "" {
		return fmt.Errorf("API key is required for setup")
	}
	if err := auth.SaveKey(svc, key); err != nil {
		return fmt.Errorf("error saving key: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved %s API key to keychain.\n", svc)
	return nil
}

func runEnvDelete(cmd *cobra.Command, opts *envOptions) error {
	svc := strings.ToLower(opts.service)
	if svc != "openai" {
		return fmt.Errorf("invalid service. Must be 'openai'")
	}
	if err := auth.DeleteKey(svc); err != nil {
		return fmt.Errorf("error deleting key: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s API key from keychain.\n", svc)
	return nil
}

func runEnvStatus(cmd *cobra.Command, opts *envOptions) error {
	svc := strings.ToLower(opts.service)
	if svc != "openai" {
		return fmt.Errorf("invalid service. Must be 'openai'")
	}

	exists := getStatus(svc)
	if exists {
		fmt.Fprintf(cmd.OutOrStdout(), "%s API Key: Found (source=Keychain)\n", svc)
		return nil
	}
	if envKey, ok := getEnvKey(svc); ok && envKey != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "%s API Key: Found (source=Environment Variable; disabled by default, use --allow-env)\n", svc)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s API Key: Not Found (keychain empty, env not set)\n", svc)
	return nil
}
