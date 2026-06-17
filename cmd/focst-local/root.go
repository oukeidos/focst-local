package main

import (
	"fmt"
	"os"

	"github.com/oukeidos/focst-local/internal/cleanup"
	"github.com/oukeidos/focst-local/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func execute() {
	cmd := newRootCmd()
	err := cmd.Execute()
	if cleanupErr := cleanup.RunAll(); cleanupErr != nil {
		fmt.Fprintln(os.Stderr, cleanupErr)
		if err == nil {
			err = cleanupErr
		}
	}
	if err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	translateOpts := translateOptions{}

	cmd := &cobra.Command{
		Use:   "focst-local",
		Short: "Local LLM subtitle translator",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if hasAnyFlagSet(cmd) {
					_ = cmd.Usage()
					return fmt.Errorf("input and output files are required")
				}
				return cmd.Help()
			}
			if isSubcommand(cmd, args[0]) {
				_ = cmd.Usage()
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return runTranslate(cmd, args, &translateOpts)
		},
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
	}

	cmd.Version = version.Info()
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.SetUsageTemplate(rootUsageTemplate)

	addTranslateFlags(cmd, &translateOpts)

	cmd.AddCommand(
		newAboutCmd(),
		newDisclaimerCmd(),
		newTranslateCmd(),
		newRepairCmd(),
		newNamesCmd(),
		newListCmd(),
		newEnvCmd(),
		newConfigCmd(),
		newLicensesCmd(),
		newGlossaryCmd(),
		newPhraseAnchorsCmd(),
		newPolishCmd(),
	)

	cmd.InitDefaultCompletionCmd()
	for _, sub := range cmd.Commands() {
		if sub.Name() == "completion" {
			sub.Short = "focst-local completion scripts"
			sub.SetUsageTemplate(subcommandUsageTemplate)
			break
		}
	}

	return cmd
}

func hasAnyFlagSet(cmd *cobra.Command) bool {
	changed := false
	cmd.Flags().Visit(func(_ *pflag.Flag) {
		changed = true
	})
	return changed
}

func isSubcommand(cmd *cobra.Command, name string) bool {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}
