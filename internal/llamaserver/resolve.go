package llamaserver

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oukeidos/focst-local/internal/userconfig"
)

func ResolveConfig(cfg LaunchConfig, userCfg userconfig.Config) (LaunchConfig, error) {
	cfg = Normalize(cfg)
	if cfg.Mode != ModeExternal && cfg.Mode != ModeStart {
		return LaunchConfig{}, fmt.Errorf("invalid llama server mode %q", cfg.Mode)
	}
	if cfg.Mode == ModeStart {
		serverBin, err := resolveServerBin(cfg.ServerBin, userCfg)
		if err != nil {
			return LaunchConfig{}, err
		}
		cfg.ServerBin = serverBin
		modelPath, err := resolveModelPath(cfg.ModelPath, userCfg)
		if err != nil {
			return LaunchConfig{}, err
		}
		cfg.ModelPath = modelPath
	}
	return cfg, nil
}

func resolveServerBin(value string, cfg userconfig.Config) (string, error) {
	if strings.TrimSpace(value) != "" {
		return cleanExecutable(value, "--llama-server-bin")
	}
	if env := strings.TrimSpace(os.Getenv(userconfig.EnvLlamaServerBin)); env != "" {
		return cleanExecutable(env, userconfig.EnvLlamaServerBin)
	}
	if strings.TrimSpace(cfg.LlamaServerBin) != "" {
		return cleanExecutable(cfg.LlamaServerBin, "user config llama_server_bin")
	}
	path, err := exec.LookPath("llama-server")
	if err == nil {
		return path, nil
	}
	return "", fmt.Errorf("llama-server binary not found; set --llama-server-bin, %s, user config, or add llama-server to PATH", userconfig.EnvLlamaServerBin)
}

func resolveModelPath(value string, cfg userconfig.Config) (string, error) {
	if strings.TrimSpace(value) != "" {
		return cleanFile(value, "--model-path")
	}
	if env := strings.TrimSpace(os.Getenv(userconfig.EnvModelPath)); env != "" {
		return cleanFile(env, userconfig.EnvModelPath)
	}
	if strings.TrimSpace(cfg.ModelPath) != "" {
		return cleanFile(cfg.ModelPath, "user config model_path")
	}
	return "", fmt.Errorf("model file path is required in start mode; set --model-path, %s, or user config model_path", userconfig.EnvModelPath)
}

func ResolveIntFromEnv(name string) (int, bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, true, fmt.Errorf("%s must be a positive integer, got %q", name, value)
	}
	return n, true, nil
}

func cleanExecutable(path, source string) (string, error) {
	cleaned, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", source, err)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%s not found: %s", source, cleaned)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, expected executable: %s", source, cleaned)
	}
	if info.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable: %s", source, cleaned)
	}
	return cleaned, nil
}

func cleanFile(path, source string) (string, error) {
	cleaned, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", source, err)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%s not found: %s", source, cleaned)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, expected model file: %s", source, cleaned)
	}
	return cleaned, nil
}

func BaseURL(host string, port int) string {
	if strings.TrimSpace(host) == "" {
		host = DefaultHost
	}
	if port <= 0 {
		port = DefaultPort
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/v1"
}
