package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "id_rsa")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		SSH: SSHConfig{
			Host:           "10.0.0.1",
			Port:           22,
			User:           "monitor",
			KeyFile:        keyFile,
			InsecureHostKey: true,
			DialTimeout:    10 * time.Second,
			CommandTimeout: 30 * time.Second,
		},
		Cache: CacheConfig{
			Dir:            tmpDir,
			MetricsTTL:     5 * time.Minute,
			DiscoveryTTL:   4 * time.Hour,
			PerformanceTTL: 60 * time.Second,
		},
		Limits: LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:     500,
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidationFailures(t *testing.T) {
	cfg := &Config{}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error on empty config, got nil")
	}

	// Missing auth
	cfg.SSH.Host = "10.0.0.1"
	cfg.SSH.User = "monitor"
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error when no auth configured")
	}

	// Both keyfile and password
	cfg.SSH.Password = "secret"
	cfg.SSH.KeyFile = "/nonexistent/key"
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error when both keyfile and password set")
	}
}

func TestCacheFileSanitization(t *testing.T) {
	cfg := &Config{Cache: CacheConfig{Dir: "/var/tmp"}}
	got := cfg.CacheFile("10.0.0.1:8080/foo")
	expected := filepath.Join("/var/tmp", "flashsystem_10.0.0.1_8080_foo.cache.json")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
