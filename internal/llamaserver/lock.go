package llamaserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/oukeidos/focst-local/internal/files"
)

type LockFile struct {
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	BaseURL    string    `json:"base_url"`
	Model      string    `json:"model"`
	ModelPath  string    `json:"model_path"`
	ServerBin  string    `json:"server_bin"`
	CtxSize    int       `json:"ctx_size"`
	Parallel   int       `json:"parallel"`
	Args       []string  `json:"args"`
	LogFile    string    `json:"log_file"`
	Executable string    `json:"executable"`
}

func LockPath(host string, port int) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	name := host + "-" + strconv.Itoa(port) + ".json"
	return filepath.Join(dir, "focst-local", "llama-server", name), nil
}

func SaveLock(path string, lock LockFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return files.AtomicWrite(path, data, DefaultLockFilePerm)
}

func LoadLock(path string) (LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LockFile{}, err
	}
	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockFile{}, err
	}
	return lock, nil
}

func RemoveLock(path string) {
	_ = os.Remove(path)
}
