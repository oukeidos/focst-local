package main

import (
	"fmt"

	"github.com/oukeidos/focst-local/internal/postpolish"
	"github.com/spf13/cobra"
)

func validatePostPolishProfileFlags(cmd *cobra.Command, profileValue string, active bool) error {
	profileChanged := cmd.Flags().Changed("post-polish-profile")
	if !active && !profileChanged {
		return nil
	}
	profile, ok := postpolish.NormalizeProfile(profileValue)
	if !ok {
		return fmt.Errorf("invalid post-polish profile: %s", profileValue)
	}
	if active && profile != postpolish.ProfileLegacy {
		if cmd.Flags().Changed("polish-broad-chunk-size") || cmd.Flags().Changed("polish-repair-chunk-size") {
			return fmt.Errorf("--polish-broad-chunk-size and --polish-repair-chunk-size are only supported with --post-polish-profile legacy")
		}
	}
	return nil
}
