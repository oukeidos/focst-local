package main

import (
	"fmt"

	"github.com/oukeidos/focst-local/internal/licenses"
	"github.com/spf13/cobra"
)

func newDisclaimerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disclaimer",
		Short: "Show the full disclaimer",
		RunE: func(cmd *cobra.Command, args []string) error {
			text := licenses.DisclaimerText()
			if text == "" {
				return fmt.Errorf("embedded DISCLAIMER is empty; regenerate embedded disclaimer")
			}
			_, err := cmd.OutOrStdout().Write([]byte(text))
			return err
		},
		SilenceUsage: true,
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}
