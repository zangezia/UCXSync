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

func TestLoadSupportsCustomNetworkMountOptions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := strings.Join([]string{
		"network:",
		"  mount_root: /ucmount-a",
		"  mount_options:",
		"    - nounix",
		"    - '  noserverino  '",
		"    - actimeo=1",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := []string{"nounix", "noserverino", "actimeo=1"}
	if strings.Join(cfg.Network.MountOptions, ",") != strings.Join(want, ",") {
		t.Fatalf("expected cleaned mount options %v, got %v", want, cfg.Network.MountOptions)
	}
}

func TestLoadRejectsEmptyNetworkMountOption(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := "network:\n  mount_root: /ucmount-a\n  mount_options:\n    - nounix\n    - '   '\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected Load to fail for empty network mount option")
	}

	if !strings.Contains(err.Error(), "network.mount_options") {
		t.Fatalf("expected network.mount_options validation error, got %v", err)
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

func TestLoadSupportsDashboardInstances(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := strings.Join([]string{
		"web:",
		"  dashboard:",
		"    instances:",
		"      - id: a",
		"        name: Instance A",
		"        url: http://127.0.0.1:8080/",
		"      - id: b",
		"        url: http://127.0.0.1:8081",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := len(cfg.Web.Dashboard.Instances); got != 2 {
		t.Fatalf("expected 2 dashboard instances, got %d", got)
	}

	if cfg.Web.Dashboard.Instances[0].URL != "http://127.0.0.1:8080" {
		t.Fatalf("expected trailing slash to be trimmed, got %q", cfg.Web.Dashboard.Instances[0].URL)
	}

	if cfg.Web.Dashboard.Instances[1].Name != "b" {
		t.Fatalf("expected empty dashboard name to default to id, got %q", cfg.Web.Dashboard.Instances[1].Name)
	}
}

func TestLoadRejectsDashboardInstanceWithoutHTTPURL(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := strings.Join([]string{
		"web:",
		"  dashboard:",
		"    instances:",
		"      - id: a",
		"        url: 127.0.0.1:8080",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected Load to fail for invalid dashboard URL")
	}

	if !strings.Contains(err.Error(), "web.dashboard.instances") {
		t.Fatalf("expected dashboard validation error, got %v", err)
	}
}
