package files

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRejectSymlinkPath_Target(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink not permitted on Windows")
	}
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(tmp, "out.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := RejectSymlinkPath(link); err == nil {
		t.Fatalf("expected symlink rejection")
	}
}

func TestRejectSymlinkPath_ParentDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink not permitted on Windows")
	}
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	if err := os.MkdirAll(realDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkDir := filepath.Join(tmp, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	path := filepath.Join(linkDir, "out.txt")

	if err := RejectSymlinkPath(path); err == nil {
		t.Fatalf("expected symlinked directory rejection")
	}
}

func TestRejectSymlinkPath_AncestorDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink not permitted on Windows")
	}
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real", "nested")
	if err := os.MkdirAll(realDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkDir := filepath.Join(tmp, "link")
	if err := os.Symlink(filepath.Join(tmp, "real"), linkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	path := filepath.Join(linkDir, "nested", "out.txt")

	if err := RejectSymlinkPath(path); err == nil {
		t.Fatalf("expected ancestor symlink rejection")
	}
}

func TestAtomicWriteRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink not permitted on Windows")
	}
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(tmp, "out.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := AtomicWrite(link, []byte("new"), 0600); err == nil {
		t.Fatalf("expected AtomicWrite to reject symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("target modified via symlink: %s", string(data))
	}
}
