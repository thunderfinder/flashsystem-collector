package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.SSH.Port != 22 {
		t.Errorf("expected default SSH.Port 22, got %d", cfg.SSH.Port)
	}
	if cfg.SSH.DialTimeout != 10*time.Second {
		t.Errorf("expected default DialTimeout 10s, got %s", cfg.SSH.DialTimeout)
	}
	if cfg.SSH.CommandTimeout != 30*time.Second {
		t.Errorf("expected default CommandTimeout 30s, got %s", cfg.SSH.CommandTimeout)
	}
	if cfg.SSH.InsecureHostKey {
		t.Errorf("expected default InsecureHostKey false, got true")
	}
	if cfg.Cache.Dir != "/var/tmp" {
		t.Errorf("expected default Cache.Dir /var/tmp, got %s", cfg.Cache.Dir)
	}
	if cfg.Cache.MetricsTTL != 300*time.Second {
		t.Errorf("expected default MetricsTTL 300s, got %s", cfg.Cache.MetricsTTL)
	}
	if cfg.Cache.DiscoveryTTL != 14400*time.Second {
		t.Errorf("expected default DiscoveryTTL 14400s, got %s", cfg.Cache.DiscoveryTTL)
	}
	if cfg.Cache.PerformanceTTL != 60*time.Second {
		t.Errorf("expected default PerformanceTTL 60s, got %s", cfg.Cache.PerformanceTTL)
	}
	if cfg.Limits.MaxJSONBytes != 1024*1024 {
		t.Errorf("expected default MaxJSONBytes 1MB, got %d", cfg.Limits.MaxJSONBytes)
	}
	if cfg.Limits.MaxVolumes != 500 {
		t.Errorf("expected default MaxVolumes 500, got %d", cfg.Limits.MaxVolumes)
	}
	if cfg.Limits.MaxDrives != 500 {
		t.Errorf("expected default MaxDrives 500, got %d", cfg.Limits.MaxDrives)
	}
	if cfg.Logging.Verbose {
		t.Errorf("expected default Verbose false, got true")
	}
}

func TestApplyEnv(t *testing.T) {
	// Limpiar variables de entorno tras el test
	envKeys := []string{
		"FS_HOST", "FS_PORT", "FS_USER", "FS_PASSWORD", "FS_KEYFILE",
		"FS_INSECURE", "FS_KNOWN_HOSTS", "FS_CACHE_DIR", "FS_VERBOSE",
		"FS_DIAL_TIMEOUT", "FS_CMD_TIMEOUT", "FS_METRICS_TTL",
		"FS_DISCOVERY_TTL", "FS_PERFORMANCE_TTL", "FS_MAX_JSON_BYTES",
		"FS_MAX_VOLUMES", "FS_MAX_DRIVES",
	}
	for _, k := range envKeys {
		orig := os.Getenv(k)
		defer func(k, orig string) {
			if orig == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, orig)
			}
		}(k, orig)
	}

	os.Setenv("FS_HOST", "192.168.1.100")
	os.Setenv("FS_PORT", "2222")
	os.Setenv("FS_USER", "testuser")
	os.Setenv("FS_PASSWORD", "secret")
	os.Setenv("FS_INSECURE", "true")
	os.Setenv("FS_CACHE_DIR", "/tmp")
	os.Setenv("FS_VERBOSE", "1")
	os.Setenv("FS_DIAL_TIMEOUT", "5s")
	os.Setenv("FS_CMD_TIMEOUT", "15s")
	os.Setenv("FS_METRICS_TTL", "2m")
	os.Setenv("FS_DISCOVERY_TTL", "1h")
	os.Setenv("FS_PERFORMANCE_TTL", "30s")
	os.Setenv("FS_MAX_JSON_BYTES", "2097152")
	os.Setenv("FS_MAX_VOLUMES", "100")
	os.Setenv("FS_MAX_DRIVES", "200")

	cfg := defaultConfig()
	applyEnv(&cfg)

	if cfg.SSH.Host != "192.168.1.100" {
		t.Errorf("expected Host 192.168.1.100, got %s", cfg.SSH.Host)
	}
	if cfg.SSH.Port != 2222 {
		t.Errorf("expected Port 2222, got %d", cfg.SSH.Port)
	}
	if cfg.SSH.User != "testuser" {
		t.Errorf("expected User testuser, got %s", cfg.SSH.User)
	}
	if cfg.SSH.Password != "secret" {
		t.Errorf("expected Password secret, got %s", cfg.SSH.Password)
	}
	if !cfg.SSH.InsecureHostKey {
		t.Errorf("expected InsecureHostKey true, got false")
	}
	if cfg.Cache.Dir != "/tmp" {
		t.Errorf("expected Cache.Dir /tmp, got %s", cfg.Cache.Dir)
	}
	if !cfg.Logging.Verbose {
		t.Errorf("expected Verbose true, got false")
	}
	if cfg.SSH.DialTimeout != 5*time.Second {
		t.Errorf("expected DialTimeout 5s, got %s", cfg.SSH.DialTimeout)
	}
	if cfg.SSH.CommandTimeout != 15*time.Second {
		t.Errorf("expected CommandTimeout 15s, got %s", cfg.SSH.CommandTimeout)
	}
	if cfg.Cache.MetricsTTL != 2*time.Minute {
		t.Errorf("expected MetricsTTL 2m, got %s", cfg.Cache.MetricsTTL)
	}
	if cfg.Cache.DiscoveryTTL != 1*time.Hour {
		t.Errorf("expected DiscoveryTTL 1h, got %s", cfg.Cache.DiscoveryTTL)
	}
	if cfg.Cache.PerformanceTTL != 30*time.Second {
		t.Errorf("expected PerformanceTTL 30s, got %s", cfg.Cache.PerformanceTTL)
	}
	if cfg.Limits.MaxJSONBytes != 2097152 {
		t.Errorf("expected MaxJSONBytes 2097152, got %d", cfg.Limits.MaxJSONBytes)
	}
	if cfg.Limits.MaxVolumes != 100 {
		t.Errorf("expected MaxVolumes 100, got %d", cfg.Limits.MaxVolumes)
	}
	if cfg.Limits.MaxDrives != 200 {
		t.Errorf("expected MaxDrives 200, got %d", cfg.Limits.MaxDrives)
	}
}

func TestValidation(t *testing.T) {
	tempDir := t.TempDir()

	validCfg := func() Config {
		c := defaultConfig()
		c.SSH.Host = "10.0.0.1"
		c.SSH.User = "admin"
		c.SSH.Password = "pass"
		c.SSH.InsecureHostKey = true
		c.Cache.Dir = tempDir
		return c
	}

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := validCfg()
		if err := cfg.validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("MissingHost", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.Host = ""
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for missing host, got nil")
		}
	})

	t.Run("MissingUser", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.User = ""
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for missing user, got nil")
		}
	})

	t.Run("InvalidPort", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.Port = 70000
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for port > 65535, got nil")
		}
	})

	t.Run("MissingAuth", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.Password = ""
		cfg.SSH.KeyFile = ""
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for missing auth, got nil")
		}
	})

	t.Run("BothAuthProvided", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.KeyFile = filepath.Join(tempDir, "key")
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error when both keyfile and password are provided, got nil")
		}
	})

	t.Run("KeyFileNotFound", func(t *testing.T) {
		cfg := validCfg()
		cfg.SSH.Password = ""
		cfg.SSH.KeyFile = filepath.Join(tempDir, "non_existent_key")
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for non-existent keyfile, got nil")
		}
	})

	t.Run("KeyFileValid", func(t *testing.T) {
		keyPath := filepath.Join(tempDir, "test_key")
		if err := os.WriteFile(keyPath, []byte("key_data"), 0600); err != nil {
			t.Fatalf("failed to create key file: %v", err)
		}
		cfg := validCfg()
		cfg.SSH.Password = ""
		cfg.SSH.KeyFile = keyPath
		if err := cfg.validate(); err != nil {
			t.Fatalf("expected valid keyfile, got error: %v", err)
		}
	})

	t.Run("PerformanceTTLTooShort", func(t *testing.T) {
		cfg := validCfg()
		cfg.Cache.PerformanceTTL = 0
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for performance-ttl < 1s, got nil")
		}
	})

	t.Run("DiscoveryTTLTooShort", func(t *testing.T) {
		cfg := validCfg()
		cfg.Cache.DiscoveryTTL = 10 * time.Second
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for discovery-ttl < 1m, got nil")
		}
	})

	t.Run("InvalidCacheDir", func(t *testing.T) {
		cfg := validCfg()
		cfg.Cache.Dir = filepath.Join(tempDir, "does_not_exist_dir")
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for non-existent cache directory, got nil")
		}
	})
}

func TestCacheFile(t *testing.T) {
	cfg := defaultConfig()
	cfg.Cache.Dir = "/var/tmp"

	tests := []struct {
		host     string
		expected string
	}{
		{"10.10.10.50", "/var/tmp/flashsystem_10.10.10.50.cache.json"},
		{"storage:primary", "/var/tmp/flashsystem_storage_primary.cache.json"},
		{"host/sub/name", "/var/tmp/flashsystem_host_sub_name.cache.json"},
		{"host name with spaces", "/var/tmp/flashsystem_host_name_with_spaces.cache.json"},
	}

	for _, tt := range tests {
		got := cfg.CacheFile(tt.host)
		if got != tt.expected {
			t.Errorf("CacheFile(%q) = %q, expected %q", tt.host, got, tt.expected)
		}
	}
}

func TestParseDurationEnv(t *testing.T) {
	if d := parseDurationEnv("NON_EXISTENT_VAR"); d != 0 {
		t.Errorf("expected 0 for unset var, got %s", d)
	}

	t.Setenv("TEST_DUR_VALID", "15m")
	if d := parseDurationEnv("TEST_DUR_VALID"); d != 15*time.Minute {
		t.Errorf("expected 15m, got %s", d)
	}

	t.Setenv("TEST_DUR_INVALID", "invalid_duration")
	if d := parseDurationEnv("TEST_DUR_INVALID"); d != 0 {
		t.Errorf("expected 0 for invalid duration, got %s", d)
	}
}

func TestLoadWithFlags(t *testing.T) {
	// Guardar os.Args original y CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}()

	tempDir := t.TempDir()
	os.Args = []string{
		"cmd",
		"-host=192.168.10.1",
		"-user=monitor",
		"-password=testpass",
		"-insecure=true",
		"-cache-dir=" + tempDir,
		"-performance-ttl=45s",
	}

	// Reinicializar CommandLine para testing de flag.Parse()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.SSH.Host != "192.168.10.1" {
		t.Errorf("expected Host 192.168.10.1, got %s", cfg.SSH.Host)
	}
	if cfg.Cache.PerformanceTTL != 45*time.Second {
		t.Errorf("expected PerformanceTTL 45s, got %s", cfg.Cache.PerformanceTTL)
	}
}
