package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPathPrefersLocalConfig(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	localConfig := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(localConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	exampleDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	exampleConfig := filepath.Join(exampleDir, "config.example.yaml")
	if err := os.WriteFile(exampleConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "config.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "config.yaml")
	}
}

func TestDefaultPathFallsBackToExample(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	exampleDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	exampleConfig := filepath.Join(exampleDir, "config.example.yaml")
	if err := os.WriteFile(exampleConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "configs/config.example.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "configs/config.example.yaml")
	}
}

func TestDefaultPathReturnsLocalNameWhenNoConfigExists(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "config.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "config.yaml")
	}
}
