package llamaserver

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oukeidos/focst-local/internal/logger"
	"github.com/oukeidos/focst-local/internal/userconfig"
)

type DefaultManager struct{}

func NewManager() *DefaultManager {
	return &DefaultManager{}
}

func (m *DefaultManager) Ensure(ctx context.Context, cfg LaunchConfig) (ManagedServer, func(context.Context) error, error) {
	userCfg, _, err := userconfig.LoadDefault()
	if err != nil {
		return ManagedServer{}, nil, err
	}
	cfg, err = ResolveConfig(cfg, userCfg)
	if err != nil {
		return ManagedServer{}, nil, err
	}
	switch cfg.Mode {
	case ModeExternal:
		return m.ensureExternal(ctx, cfg)
	case ModeStart:
		return m.ensureStarted(ctx, cfg)
	default:
		return ManagedServer{}, nil, fmt.Errorf("invalid llama server mode %q", cfg.Mode)
	}
}

func (m *DefaultManager) Status(ctx context.Context, baseURL string) (Status, error) {
	return Probe(ctx, baseURL)
}

func (m *DefaultManager) Stop(ctx context.Context, lock LockFile) error {
	if lock.PID <= 0 {
		return fmt.Errorf("lock has no pid")
	}
	proc, err := os.FindProcess(lock.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		done <- err
	}()
	select {
	case <-ctx.Done():
		_ = proc.Kill()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (m *DefaultManager) ensureExternal(ctx context.Context, cfg LaunchConfig) (ManagedServer, func(context.Context) error, error) {
	logger.Info("Probing llama.cpp server",
		"event", "llama_server_probe_started",
		"mode", cfg.Mode,
		"base_url", cfg.BaseURL,
		"model", cfg.ModelAlias,
	)
	status, err := Probe(ctx, cfg.BaseURL)
	if err != nil {
		return ManagedServer{}, nil, fmt.Errorf("local LLM is not ready: %w", err)
	}
	if err := validateStatus(status, cfg.ModelAlias); err != nil {
		logger.Error("llama.cpp server incompatible",
			"event", "llama_server_incompatible",
			"mode", cfg.Mode,
			"base_url", cfg.BaseURL,
			"model", cfg.ModelAlias,
			"available_models", strings.Join(status.ModelLabels(), ", "),
		)
		return ManagedServer{}, nil, err
	}
	warnContext(status, cfg)
	logger.Info("llama.cpp server ready",
		"event", "llama_server_probe_completed",
		"mode", cfg.Mode,
		"owned", false,
		"base_url", cfg.BaseURL,
		"model", cfg.ModelAlias,
		"n_ctx", status.NCtx,
	)
	return ManagedServer{BaseURL: cfg.BaseURL, Owned: false, Config: cfg}, func(context.Context) error { return nil }, nil
}

func (m *DefaultManager) ensureStarted(ctx context.Context, cfg LaunchConfig) (ManagedServer, func(context.Context) error, error) {
	if err := ensurePortFree(cfg.Host, cfg.Port); err != nil {
		return ManagedServer{}, nil, err
	}
	if cfg.LogFile == "" {
		logFile, err := defaultLogFile(cfg)
		if err != nil {
			return ManagedServer{}, nil, err
		}
		cfg.LogFile = logFile
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0700); err != nil {
		return ManagedServer{}, nil, fmt.Errorf("failed to create llama log directory: %w", err)
	}
	logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, DefaultLogFilePerm)
	if err != nil {
		return ManagedServer{}, nil, fmt.Errorf("failed to open llama log file: %w", err)
	}

	args, err := BuildArgs(cfg)
	if err != nil {
		_ = logFile.Close()
		return ManagedServer{}, nil, err
	}
	logger.Info("Starting llama.cpp server",
		"event", "llama_server_starting",
		"mode", cfg.Mode,
		"base_url", cfg.BaseURL,
		"model", cfg.ModelAlias,
		"model_path", cfg.ModelPath,
		"server_bin", cfg.ServerBin,
		"ctx_size", cfg.CtxSize,
		"parallel", cfg.Parallel,
		"extra_args", cfg.ExtraArgs,
		"args", args,
		"log_file", cfg.LogFile,
	)

	cmd := exec.CommandContext(ctx, cfg.ServerBin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return ManagedServer{}, nil, fmt.Errorf("failed to start llama-server: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		_ = logFile.Close()
	}()

	lockPath, err := LockPath(cfg.Host, cfg.Port)
	if err == nil {
		_ = SaveLock(lockPath, LockFile{
			PID:        cmd.Process.Pid,
			StartedAt:  time.Now(),
			BaseURL:    cfg.BaseURL,
			Model:      cfg.ModelAlias,
			ModelPath:  cfg.ModelPath,
			ServerBin:  cfg.ServerBin,
			CtxSize:    cfg.CtxSize,
			Parallel:   cfg.Parallel,
			Args:       append([]string(nil), args...),
			LogFile:    cfg.LogFile,
			Executable: cfg.ServerBin,
		})
	}

	started := time.Now()
	status, err := waitReady(ctx, cfg, waitCh)
	if err != nil {
		_ = stopProcess(context.Background(), cmd.Process, waitCh)
		if lockPath != "" {
			RemoveLock(lockPath)
		}
		return ManagedServer{}, nil, err
	}
	warnContext(status, cfg)
	logger.Info("llama.cpp server ready",
		"event", "llama_server_ready",
		"mode", cfg.Mode,
		"owned", true,
		"pid", cmd.Process.Pid,
		"base_url", cfg.BaseURL,
		"model", cfg.ModelAlias,
		"model_path", cfg.ModelPath,
		"server_bin", cfg.ServerBin,
		"ctx_size", cfg.CtxSize,
		"parallel", cfg.Parallel,
		"args", args,
		"log_file", cfg.LogFile,
		"load_seconds", time.Since(started).Seconds(),
		"n_ctx", status.NCtx,
	)

	cleanup := func(cleanupCtx context.Context) error {
		if cfg.KeepAlive {
			return nil
		}
		logger.Info("Stopping llama.cpp server",
			"event", "llama_server_stopping",
			"pid", cmd.Process.Pid,
			"base_url", cfg.BaseURL,
			"model", cfg.ModelAlias,
		)
		err := stopProcess(cleanupCtx, cmd.Process, waitCh)
		if lockPath != "" {
			RemoveLock(lockPath)
		}
		logger.Info("llama.cpp server stopped",
			"event", "llama_server_stopped",
			"pid", cmd.Process.Pid,
			"base_url", cfg.BaseURL,
			"error", err,
		)
		return err
	}

	return ManagedServer{
		BaseURL: cfg.BaseURL,
		PID:     cmd.Process.Pid,
		Owned:   true,
		Config:  cfg,
		LogFile: cfg.LogFile,
	}, cleanup, nil
}

func waitReady(ctx context.Context, cfg LaunchConfig, waitCh <-chan error) (Status, error) {
	deadline := time.NewTimer(cfg.LoadTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case err := <-waitCh:
			if err == nil {
				return Status{}, fmt.Errorf("llama-server exited before becoming ready")
			}
			return Status{}, fmt.Errorf("llama-server exited before becoming ready: %w (log: %s)", err, cfg.LogFile)
		case <-deadline.C:
			if lastErr != nil {
				return Status{}, fmt.Errorf("timed out waiting for llama-server: %w (log: %s)", lastErr, cfg.LogFile)
			}
			return Status{}, fmt.Errorf("timed out waiting for llama-server (log: %s)", cfg.LogFile)
		case <-ticker.C:
			status, err := Probe(ctx, cfg.BaseURL)
			if err != nil {
				lastErr = err
				continue
			}
			if err := validateStatus(status, cfg.ModelAlias); err != nil {
				lastErr = err
				continue
			}
			return status, nil
		}
	}
}

func validateStatus(status Status, modelAlias string) error {
	if !status.HasModel(modelAlias) {
		return fmt.Errorf("llama server does not expose requested model %q (available: %s)", modelAlias, strings.Join(status.ModelLabels(), ", "))
	}
	return nil
}

func warnContext(status Status, cfg LaunchConfig) {
	if status.NCtx == 0 {
		logger.Warn("llama.cpp context could not be verified",
			"event", "llama_server_context_warning",
			"base_url", cfg.BaseURL,
			"model", cfg.ModelAlias,
		)
		return
	}
	if status.NCtx < DefaultContextWarn {
		logger.Warn("llama.cpp context is below the recommended default",
			"event", "llama_server_context_warning",
			"base_url", cfg.BaseURL,
			"model", cfg.ModelAlias,
			"n_ctx", status.NCtx,
			"recommended_ctx_size", DefaultContextWarn,
		)
	}
}

func ensurePortFree(host string, port int) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("llama server port is occupied or unavailable: %s: %w", address, err)
	}
	return ln.Close()
}

func defaultLogFile(cfg LaunchConfig) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	model := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(cfg.ModelAlias)
	name := fmt.Sprintf("%s-%s.log", model, strconv.Itoa(cfg.Port))
	return filepath.Join(dir, "focst-local", "llama-server", name), nil
}

func stopProcess(ctx context.Context, proc *os.Process, waitCh <-chan error) error {
	if proc == nil {
		return nil
	}
	_ = proc.Signal(os.Interrupt)
	select {
	case err := <-waitCh:
		return err
	case <-time.After(5 * time.Second):
		_ = proc.Kill()
		select {
		case err := <-waitCh:
			return err
		case <-time.After(2 * time.Second):
			return fmt.Errorf("timed out waiting for llama-server process to stop")
		}
	case <-ctx.Done():
		_ = proc.Kill()
		return ctx.Err()
	}
}
