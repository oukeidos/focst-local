package userconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oukeidos/focst-local/internal/files"
)

const (
	EnvLlamaServerBin = "FOCST_LLAMA_SERVER_BIN"
	EnvModelPath      = "FOCST_LLAMA_MODEL_PATH"
	EnvCtxSize        = "FOCST_LLAMA_CTX_SIZE"
	EnvParallel       = "FOCST_LLAMA_PARALLEL"
)

// Config stores user-specific local llama.cpp defaults.
type Config struct {
	LlamaServerBin string   `json:"llama_server_bin,omitempty"`
	ModelPath      string   `json:"model_path,omitempty"`
	Model          string   `json:"model,omitempty"`
	CtxSize        int      `json:"ctx_size,omitempty"`
	Parallel       int      `json:"parallel,omitempty"`
	ExtraArgs      []string `json:"extra_args,omitempty"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "focst-local", "config.json"), nil
}

func LoadDefault() (Config, string, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := Load(path)
	return cfg, path, err
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return Config{}, nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := files.AtomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

func Set(cfg Config, key, value string) (Config, error) {
	key = normalizeKey(key)
	switch key {
	case "llama_server_bin":
		cfg.LlamaServerBin = strings.TrimSpace(value)
	case "model_path":
		cfg.ModelPath = strings.TrimSpace(value)
	case "model":
		cfg.Model = strings.TrimSpace(value)
	case "ctx_size":
		n, err := parsePositiveInt(key, value)
		if err != nil {
			return Config{}, err
		}
		cfg.CtxSize = n
	case "parallel":
		n, err := parsePositiveInt(key, value)
		if err != nil {
			return Config{}, err
		}
		cfg.Parallel = n
	default:
		return Config{}, fmt.Errorf("unknown config key %q", key)
	}
	return cfg, nil
}

func Unset(cfg Config, key string) (Config, error) {
	key = normalizeKey(key)
	switch key {
	case "llama_server_bin":
		cfg.LlamaServerBin = ""
	case "model_path":
		cfg.ModelPath = ""
	case "model":
		cfg.Model = ""
	case "ctx_size":
		cfg.CtxSize = 0
	case "parallel":
		cfg.Parallel = 0
	case "extra_args":
		cfg.ExtraArgs = nil
	default:
		return Config{}, fmt.Errorf("unknown config key %q", key)
	}
	return cfg, nil
}

func AddArg(cfg Config, arg string) Config {
	cfg.ExtraArgs = append(cfg.ExtraArgs, arg)
	return cfg
}

func ClearArgs(cfg Config) Config {
	cfg.ExtraArgs = nil
	return cfg
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, "-", "_")
	return key
}

func parsePositiveInt(key, value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, value)
	}
	return n, nil
}
