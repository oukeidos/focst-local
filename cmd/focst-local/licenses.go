package main

import (
	"fmt"

	"github.com/oukeidos/focst-local/internal/licenses"
	"github.com/spf13/cobra"
)

func newLicensesCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "licenses",
		Short: "Show third-party license notices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLicenses(cmd, full)
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.Flags().BoolVar(&full, "full", false, "Print full license texts")
	return cmd
}

func runLicenses(cmd *cobra.Command, full bool) error {
	if full {
		return printFullLicenses(cmd)
	}
	return printNotices(cmd)
}

func printNotices(cmd *cobra.Command) error {
	text := licenses.NoticesText()
	if text == "" {
		return fmt.Errorf("embedded THIRD_PARTY_NOTICES is empty; regenerate embedded license notices")
	}
	_, err := cmd.OutOrStdout().Write([]byte(text))
	return err
}

func printFullLicenses(cmd *cobra.Command) error {
	text := licenses.FullText()
	if text == "" {
		return fmt.Errorf("embedded THIRD_PARTY_LICENSES_FULL is empty; regenerate embedded license notices")
	}
	_, err := cmd.OutOrStdout().Write([]byte(text))
	return err
}
