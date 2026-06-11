package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAboutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "about",
		Short: "Show a short description and link",
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "focst-local - local LLM subtitle translator")
			fmt.Fprintln(out, "https://github.com/oukeidos/focst-local")
		},
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}
