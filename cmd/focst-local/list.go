package main

import (
	"fmt"

	"github.com/oukeidos/focst-local/internal/language"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported languages",
		Run: func(cmd *cobra.Command, args []string) {
			langs := language.GetSupportedLanguages()
			fmt.Fprintln(cmd.OutOrStdout(), "Supported Languages:")
			for _, l := range langs {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-35s [%s]\n", l.Name, l.ID)
			}
		},
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}
