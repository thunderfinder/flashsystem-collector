package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// generateTestPrivateKeyPEM creates a valid ED25519 private key PEM for tests.
func generateTestPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	privBytes, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	return pem.EncodeToMemory(privBytes)
}

func TestBuildHostKeyCallback(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("InsecureHostKey", func(t *testing.T) {
		cfg := ClientConfig{
			InsecureHostKey: true,
		}
		cb, err := buildHostKeyCallback(cfg)
		if err != nil {
			t.Fatalf("expected no error for InsecureHostKey, got: %v", err)
		}
		if cb == nil {
			t.Fatal("expected non-nil HostKeyCallback for InsecureHostKey")
		}
	})

	t.Run("ValidKnownHostsFile", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate ed25519 key: %v", err)
		}
		sshPubKey, err := ssh.NewPublicKey(pubKey)
		if err != nil {
			t.Fatalf("failed to create ssh public key: %v", err)
		}
		knownHostsLine := knownhosts.Line([]string{"10.0.0.1"}, sshPubKey) + "\n"

		khPath := filepath.Join(tempDir, "valid_known_hosts")
		if err := os.WriteFile(khPath, []byte(knownHostsLine), 0644); err != nil {
			t.Fatalf("failed to write known_hosts: %v", err)
		}
		if err := os.Chmod(khPath, 0644); err != nil {
			t.Fatalf("failed to chmod known_hosts: %v", err)
		}

		cfg := ClientConfig{
			KnownHostsFile: khPath,
		}
		cb, err := buildHostKeyCallback(cfg)
		if err != nil {
			t.Fatalf("expected valid known_hosts to succeed, got: %v", err)
		}
		if cb == nil {
			t.Fatal("expected non-nil callback")
		}
	})

	t.Run("NonExistentKnownHostsFile", func(t *testing.T) {
		cfg := ClientConfig{
			KnownHostsFile: filepath.Join(tempDir, "does_not_exist"),
		}
		_, err := buildHostKeyCallback(cfg)
		if err == nil {
			t.Fatal("expected error for non-existent known_hosts file, got nil")
		}
		if !strings.Contains(err.Error(), "invalid known_hosts file") {
			t.Fatalf("expected 'invalid known_hosts file' in error, got: %v", err)
		}
	})

	t.Run("DirectoryKnownHostsFile", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "kh_dir")
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		cfg := ClientConfig{
			KnownHostsFile: dirPath,
		}
		_, err := buildHostKeyCallback(cfg)
		if err == nil {
			t.Fatal("expected error for directory known_hosts, got nil")
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("expected 'not a regular file' in error, got: %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("InsecurePermissionsKnownHostsFile", func(t *testing.T) {
			khPath := filepath.Join(tempDir, "insecure_kh")
			if err := os.WriteFile(khPath, []byte("# empty known_hosts\n"), 0666); err != nil {
				t.Fatalf("failed to write insecure known_hosts: %v", err)
			}
			if err := os.Chmod(khPath, 0666); err != nil {
				t.Fatalf("failed to chmod insecure known_hosts: %v", err)
			}

			cfg := ClientConfig{
				KnownHostsFile: khPath,
			}
			_, err := buildHostKeyCallback(cfg)
			if err == nil {
				t.Fatal("expected error for insecure known_hosts file permissions, got nil")
			}
			if !strings.Contains(err.Error(), "insecure permissions") {
				t.Fatalf("expected 'insecure permissions' in error, got: %v", err)
			}
		})
	}

	t.Run("MissingHostKeyConfig", func(t *testing.T) {
		// Set HOME to an empty temporary dir to ensure no ~/.ssh/known_hosts exists
		t.Setenv("HOME", tempDir)

		cfg := ClientConfig{
			InsecureHostKey: false,
			KnownHostsFile:  "",
		}
		_, err := buildHostKeyCallback(cfg)
		if err == nil {
			t.Fatal("expected error when no host key verification is configured, got nil")
		}
		if !strings.Contains(err.Error(), "SSH host key verification not configured") {
			t.Fatalf("expected 'SSH host key verification not configured' in error, got: %v", err)
		}
	})
}

func TestBuildAuthMethod(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("PasswordAuth", func(t *testing.T) {
		cfg := ClientConfig{
			Password: "secret-password",
		}
		auth, err := buildAuthMethod(cfg)
		if err != nil {
			t.Fatalf("expected no error for password auth, got: %v", err)
		}
		if auth == nil {
			t.Fatal("expected non-nil auth method")
		}
	})

	t.Run("ValidKeyFileAuth", func(t *testing.T) {
		keyPEM := generateTestPrivateKeyPEM(t)
		keyPath := filepath.Join(tempDir, "test_id_ed25519")
		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			t.Fatalf("failed to write key file: %v", err)
		}

		cfg := ClientConfig{
			KeyFile: keyPath,
		}
		auth, err := buildAuthMethod(cfg)
		if err != nil {
			t.Fatalf("expected no error for valid key file, got: %v", err)
		}
		if auth == nil {
			t.Fatal("expected non-nil auth method")
		}
	})

	t.Run("NonExistentKeyFile", func(t *testing.T) {
		cfg := ClientConfig{
			KeyFile: filepath.Join(tempDir, "non_existent_key"),
		}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error for non-existent key file, got nil")
		}
	})

	t.Run("InvalidKeyFileContent", func(t *testing.T) {
		keyPath := filepath.Join(tempDir, "corrupt_key")
		if err := os.WriteFile(keyPath, []byte("NOT_A_PRIVATE_KEY"), 0600); err != nil {
			t.Fatalf("failed to write corrupt key: %v", err)
		}

		cfg := ClientConfig{
			KeyFile: keyPath,
		}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error for corrupt key file, got nil")
		}
	})

	t.Run("NoAuthMethod", func(t *testing.T) {
		cfg := ClientConfig{}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error when no auth method is configured, got nil")
		}
	})
}

func TestClientCloseNil(t *testing.T) {
	c := &Client{commandTimeout: 5 * time.Second}
	if err := c.Close(); err != nil {
		t.Fatalf("expected nil error on Close() when conn is nil, got: %v", err)
	}
}
