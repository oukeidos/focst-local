package llamaserver

import (
	"context"
	"time"

	"github.com/oukeidos/focst-local/internal/localllm"
)

type Mode string

const (
	ModeExternal Mode = "external"
	ModeStart    Mode = "start"

	DefaultHost         = "127.0.0.1"
	DefaultPort         = 8080
	DefaultCtxSize      = 16384
	DefaultParallel     = 1
	DefaultLoadTimeout  = 120 * time.Second
	DefaultContextWarn  = 16384
	DefaultLogFilePerm  = 0600
	DefaultLockFilePerm = 0600
)

type LaunchConfig struct {
	Mode        Mode
	ServerBin   string
	ModelPath   string
	ModelAlias  string
	Host        string
	Port        int
	BaseURL     string
	CtxSize     int
	Parallel    int
	Threads     int
	CacheRAM    string
	ExtraArgs   []string
	LogFile     string
	LoadTimeout time.Duration
	KeepAlive   bool
}

type ManagedServer struct {
	BaseURL string
	PID     int
	Owned   bool
	Config  LaunchConfig
	LogFile string
}

type ModelInfo struct {
	ID      string
	Name    string
	Model   string
	Aliases []string
	NCtx    int
}

type Status struct {
	BaseURL string
	Models  []ModelInfo
	NCtx    int
}

type Manager interface {
	Ensure(ctx context.Context, cfg LaunchConfig) (ManagedServer, func(context.Context) error, error)
	Status(ctx context.Context, baseURL string) (Status, error)
	Stop(ctx context.Context, lock LockFile) error
}

func Normalize(cfg LaunchConfig) LaunchConfig {
	if cfg.Mode == "" {
		cfg.Mode = ModeExternal
	}
	if cfg.ModelAlias == "" {
		cfg.ModelAlias = localllm.DefaultModel
	}
	if cfg.Host == "" {
		cfg.Host = DefaultHost
	}
	if cfg.Port <= 0 {
		cfg.Port = DefaultPort
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURL(cfg.Host, cfg.Port)
	}
	if cfg.CtxSize <= 0 {
		cfg.CtxSize = DefaultCtxSize
	}
	if cfg.Parallel <= 0 {
		cfg.Parallel = DefaultParallel
	}
	if cfg.LoadTimeout <= 0 {
		cfg.LoadTimeout = DefaultLoadTimeout
	}
	return cfg
}
