package llamaserver

import (
	"fmt"
	"strconv"
)

func BuildArgs(cfg LaunchConfig) ([]string, error) {
	cfg = Normalize(cfg)
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}
	if cfg.ModelAlias == "" {
		return nil, fmt.Errorf("model alias is required")
	}
	args := []string{
		"--model", cfg.ModelPath,
		"--alias", cfg.ModelAlias,
		"--host", cfg.Host,
		"--port", strconv.Itoa(cfg.Port),
		"--ctx-size", strconv.Itoa(cfg.CtxSize),
		"--parallel", strconv.Itoa(cfg.Parallel),
	}
	if cfg.LogFile != "" {
		args = append(args, "--log-file", cfg.LogFile)
	}
	if cfg.Threads > 0 {
		args = append(args, "--threads", strconv.Itoa(cfg.Threads))
	}
	if cfg.CacheRAM != "" {
		args = append(args, "--cache-ram", cfg.CacheRAM)
	}
	args = append(args, cfg.ExtraArgs...)
	return args, nil
}
