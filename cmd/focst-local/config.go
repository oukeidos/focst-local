package main

import (
	"encoding/json"
	"fmt"

	"github.com/oukeidos/focst-local/internal/userconfig"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage focst-local user config",
	}
	cmd.SetUsageTemplate(subcommandUsageTemplate)
	cmd.AddCommand(
		newConfigPathCmd(),
		newConfigShowCmd(),
		newConfigSetCmd(),
		newConfigUnsetCmd(),
		newConfigAddArgCmd(),
		newConfigClearArgsCmd(),
	)
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the user config path",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := userconfig.DefaultPath()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the current user config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := userconfig.LoadDefault()
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a user config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := userconfig.LoadDefault()
			if err != nil {
				return err
			}
			cfg, err = userconfig.Set(cfg, args[0], args[1])
			if err != nil {
				return err
			}
			return userconfig.Save(path, cfg)
		},
	}
}

func newConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a user config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := userconfig.LoadDefault()
			if err != nil {
				return err
			}
			cfg, err = userconfig.Unset(cfg, args[0])
			if err != nil {
				return err
			}
			return userconfig.Save(path, cfg)
		},
	}
}

func newConfigAddArgCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "add-arg <arg>",
		Short:              "Append one llama.cpp argument token",
		DisableFlagParsing: true,
		Args:               cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := userconfig.LoadDefault()
			if err != nil {
				return err
			}
			cfg = userconfig.AddArg(cfg, args[0])
			return userconfig.Save(path, cfg)
		},
	}
}

func newConfigClearArgsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-args",
		Short: "Remove all stored llama.cpp argument tokens",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, path, err := userconfig.LoadDefault()
			if err != nil {
				return err
			}
			cfg = userconfig.ClearArgs(cfg)
			return userconfig.Save(path, cfg)
		},
	}
}
