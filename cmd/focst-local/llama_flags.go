package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/userconfig"
	"github.com/spf13/cobra"
)

type llamaServerOptions struct {
	mode        string
	serverBin   string
	modelPath   string
	host        string
	port        int
	ctxSize     int
	parallel    int
	threads     int
	cacheRAM    string
	extraArgs   []string
	keepAlive   bool
	loadTimeout time.Duration
	logFile     string
}

func addLlamaServerFlags(cmd *cobra.Command, opts *llamaServerOptions) {
	cmd.Flags().StringVar(&opts.mode, "llama-server-mode", string(llamaserver.ModeExternal), "llama.cpp server mode: external or start")
	cmd.Flags().StringVar(&opts.serverBin, "llama-server-bin", "", "Path to llama-server executable for start mode")
	cmd.Flags().StringVar(&opts.modelPath, "model-path", "", "Path to GGUF model file for start mode")
	cmd.Flags().StringVar(&opts.host, "llama-host", llamaserver.DefaultHost, "Host for managed llama-server")
	cmd.Flags().IntVar(&opts.port, "llama-port", llamaserver.DefaultPort, "Port for managed llama-server")
	cmd.Flags().IntVar(&opts.ctxSize, "ctx-size", 0, "llama.cpp context size for start mode (default: config/env/16384)")
	cmd.Flags().IntVar(&opts.parallel, "parallel", 0, "llama.cpp parallel slots for start mode (default: config/env/1)")
	cmd.Flags().IntVar(&opts.threads, "threads", 0, "llama.cpp worker threads for start mode")
	cmd.Flags().StringVar(&opts.cacheRAM, "cache-ram", "", "llama.cpp prompt cache RAM setting for start mode")
	cmd.Flags().StringArrayVar(&opts.extraArgs, "llama-arg", nil, "Extra llama.cpp argument token for start mode; repeat for multiple tokens")
	cmd.Flags().BoolVar(&opts.keepAlive, "keep-llama-server", false, "Keep managed llama-server running after command completion")
	cmd.Flags().DurationVar(&opts.loadTimeout, "llama-load-timeout", llamaserver.DefaultLoadTimeout, "Timeout for managed llama-server startup")
	cmd.Flags().StringVar(&opts.logFile, "llama-log-file", "", "Path to managed llama-server log file")
}

func buildLlamaLaunchConfig(cmd *cobra.Command, opts llamaServerOptions, modelAlias, baseURL string) (llamaserver.LaunchConfig, error) {
	userCfg, _, err := userconfig.LoadDefault()
	if err != nil {
		return llamaserver.LaunchConfig{}, err
	}
	if !flagChanged(cmd, "model") && userCfg.Model != "" {
		modelAlias = userCfg.Model
	}
	ctxSize, err := resolvePositiveIntOption(cmd, "ctx-size", opts.ctxSize, userconfig.EnvCtxSize, userCfg.CtxSize, llamaserver.DefaultCtxSize)
	if err != nil {
		return llamaserver.LaunchConfig{}, err
	}
	parallel, err := resolvePositiveIntOption(cmd, "parallel", opts.parallel, userconfig.EnvParallel, userCfg.Parallel, llamaserver.DefaultParallel)
	if err != nil {
		return llamaserver.LaunchConfig{}, err
	}
	extraArgs := opts.extraArgs
	if !flagChanged(cmd, "llama-arg") && len(userCfg.ExtraArgs) > 0 {
		extraArgs = append([]string(nil), userCfg.ExtraArgs...)
	}
	mode := llamaserver.Mode(opts.mode)
	if mode == "" {
		mode = llamaserver.ModeExternal
	}
	if mode == llamaserver.ModeStart {
		baseURL = llamaserver.BaseURL(opts.host, opts.port)
	}
	return llamaserver.LaunchConfig{
		Mode:        mode,
		ServerBin:   opts.serverBin,
		ModelPath:   opts.modelPath,
		ModelAlias:  modelAlias,
		Host:        opts.host,
		Port:        opts.port,
		BaseURL:     baseURL,
		CtxSize:     ctxSize,
		Parallel:    parallel,
		Threads:     opts.threads,
		CacheRAM:    opts.cacheRAM,
		ExtraArgs:   extraArgs,
		LogFile:     opts.logFile,
		LoadTimeout: opts.loadTimeout,
		KeepAlive:   opts.keepAlive,
	}, nil
}

func resolvePositiveIntOption(cmd *cobra.Command, flagName string, flagValue int, envName string, configValue int, defaultValue int) (int, error) {
	if flagChanged(cmd, flagName) {
		if flagValue <= 0 {
			return 0, fmt.Errorf("--%s must be a positive integer", flagName)
		}
		return flagValue, nil
	}
	if value := os.Getenv(envName); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer, got %q", envName, value)
		}
		return n, nil
	}
	if configValue > 0 {
		return configValue, nil
	}
	return defaultValue, nil
}

func flagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}
