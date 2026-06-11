package pipeline

import (
	"context"
	"fmt"

	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/logger"
)

func ensureLlamaServer(ctx context.Context, cfg Config, baseURL, model string) (llamaserver.ManagedServer, func(context.Context) error, error) {
	launch := cfg.LlamaServer
	launch.BaseURL = baseURL
	launch.ModelAlias = model
	manager := cfg.LlamaManager
	if manager == nil {
		manager = llamaserver.NewManager()
	}
	server, cleanup, err := manager.Ensure(ctx, launch)
	if err != nil {
		return llamaserver.ManagedServer{}, nil, fmt.Errorf("local LLM is not ready: %w", err)
	}
	if cleanup == nil {
		cleanup = func(context.Context) error { return nil }
	}
	return server, cleanup, nil
}

func cleanupLlamaServer(cleanup func(context.Context) error) {
	if cleanup == nil {
		return
	}
	if err := cleanup(context.Background()); err != nil {
		logger.Warn("Failed to clean up managed llama.cpp server", "error", err)
	}
}
