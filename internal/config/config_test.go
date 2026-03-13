package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesDefaultNetworkMountRoot(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Network.MountRoot != "/ucmount" {
		t.Fatalf("expected default mount root /ucmount, got %q", cfg.Network.MountRoot)
	}
}

func TestLoadSupportsCustomNetworkMountRoot(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := "network:\n  mount_root: /ucmount-a/\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Network.MountRoot != "/ucmount-a" {
		t.Fatalf("expected cleaned mount root /ucmount-a, got %q", cfg.Network.MountRoot)
	}
}

func TestLoadRejectsRelativeNetworkMountRoot(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := "network:\n  mount_root: ucmount-a\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected Load to fail for relative mount root")
	}

	if !strings.Contains(err.Error(), "network.mount_root") {
		t.Fatalf("expected network.mount_root validation error, got %v", err)
	}
}
