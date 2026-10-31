package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateKeyFilePermissions(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid key file with 0600 permissions
	validKey := filepath.Join(tempDir, "valid_key")
	if err := os.WriteFile(validKey, []byte("fake-key"), 0600); err != nil {
		t.Fatalf("failed to write valid key: %v", err)
	}
	// Explicit chmod in case umask modified it
	if err := os.Chmod(validKey, 0600); err != nil {
		t.Fatalf("failed to chmod valid key: %v", err)
	}

	cfg := defaultConfig()
	cfg.SSH.Host = "10.0.0.1"
	cfg.SSH.User = "monitor"
	cfg.SSH.KeyFile = validKey
	cfg.SSH.InsecureHostKey = true
	cfg.Cache.Dir = tempDir

	if err := cfg.validate(); err != nil {
		t.Fatalf("expected valid key file with 0600 permissions to pass, got: %v", err)
	}

	if runtime.GOOS != "windows" {
		// 2. Insecure key file with 0644 permissions (group/world readable)
		insecureKey := filepath.Join(tempDir, "insecure_key")
		if err := os.WriteFile(insecureKey, []byte("fake-key"), 0644); err != nil {
			t.Fatalf("failed to write insecure key: %v", err)
		}
		if err := os.Chmod(insecureKey, 0644); err != nil {
			t.Fatalf("failed to chmod insecure key: %v", err)
		}

		cfg.SSH.KeyFile = insecureKey
		err := cfg.validate()
		if err == nil {
			t.Fatal("expected error for insecure key permissions (0644), got nil")
		}
		if !strings.Contains(err.Error(), "insecure permissions") {
			t.Fatalf("expected insecure permissions error message, got: %v", err)
		}

		// 3. Insecure key file with 0666 permissions (group/world writable)
		insecureKey2 := filepath.Join(tempDir, "insecure_key2")
		if err := os.WriteFile(insecureKey2, []byte("fake-key"), 0666); err != nil {
			t.Fatalf("failed to write insecure key2: %v", err)
		}
		if err := os.Chmod(insecureKey2, 0666); err != nil {
			t.Fatalf("failed to chmod insecure key2: %v", err)
		}

		cfg.SSH.KeyFile = insecureKey2
		err = cfg.validate()
		if err == nil {
			t.Fatal("expected error for insecure key permissions (0666), got nil")
		}
		if !strings.Contains(err.Error(), "insecure permissions") {
			t.Fatalf("expected insecure permissions error message, got: %v", err)
		}
	}

	// 4. Directory specified instead of regular file
	keyDir := filepath.Join(tempDir, "key_dir")
	if err := os.Mkdir(keyDir, 0700); err != nil {
		t.Fatalf("failed to create key dir: %v", err)
	}

	cfg.SSH.KeyFile = keyDir
	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for directory keyfile, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected 'not a regular file' error message, got: %v", err)
	}

	// 5. Non-existent file
	cfg.SSH.KeyFile = filepath.Join(tempDir, "non_existent")
	err = cfg.validate()
	if err == nil {
		t.Fatal("expected error for non-existent keyfile, got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' error message, got: %v", err)
	}
}
