package main

import (
	"fmt"

	"github.com/oukeidos/focst-local/internal/residue"
	"github.com/spf13/cobra"
)

func newScriptsCmd() *cobra.Command {
	var examples bool
	cmd := &cobra.Command{
		Use:   "scripts",
		Short: "List Unicode scripts usable for source residue detection",
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Unicode version: %s\n", residue.UnicodeVersion())
			fmt.Fprintln(out, "Input names are case-insensitive; hyphen and space are accepted as underscore.")
			fmt.Fprintln(out, "Example: old-hungarian, old_hungarian, and Old_Hungarian are equivalent.")
			fmt.Fprintln(out)
			if examples {
				fmt.Fprintln(out, "Example scripts:")
				for _, name := range residue.ExampleScripts() {
					fmt.Fprintf(out, "  %s\n", name)
				}
				return
			}
			fmt.Fprintln(out, "Supported scripts:")
			for _, name := range residue.SupportedScripts() {
				fmt.Fprintf(out, "  %s\n", name)
			}
		},
	}
	cmd.Flags().BoolVar(&examples, "examples", false, "Show a short practical script list")
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	return cmd
}
