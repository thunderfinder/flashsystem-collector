package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
		if !strings.Contains(err.Error(), "invalid SSH key file") {
			t.Fatalf("expected 'invalid SSH key file' in error, got: %v", err)
		}
	})

	t.Run("DirectoryKeyFile", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "key_dir")
		if err := os.Mkdir(dirPath, 0700); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		cfg := ClientConfig{
			KeyFile: dirPath,
		}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error for directory key file, got nil")
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("expected 'not a regular file' in error, got: %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("InsecurePermissionsKeyFile", func(t *testing.T) {
			keyPEM := generateTestPrivateKeyPEM(t)

			// 1. Group/world-readable key (0644)
			insecureKey644 := filepath.Join(tempDir, "insecure_key_644")
			if err := os.WriteFile(insecureKey644, keyPEM, 0644); err != nil {
				t.Fatalf("failed to write key file: %v", err)
			}
			if err := os.Chmod(insecureKey644, 0644); err != nil {
				t.Fatalf("failed to chmod key file: %v", err)
			}

			cfg := ClientConfig{
				KeyFile: insecureKey644,
			}
			_, err := buildAuthMethod(cfg)
			if err == nil {
				t.Fatal("expected error for insecure key file permissions (0644), got nil")
			}
			if !strings.Contains(err.Error(), "insecure permissions") {
				t.Fatalf("expected 'insecure permissions' in error, got: %v", err)
			}

			// 2. Group/world-writable key (0666)
			insecureKey666 := filepath.Join(tempDir, "insecure_key_666")
			if err := os.WriteFile(insecureKey666, keyPEM, 0666); err != nil {
				t.Fatalf("failed to write key file: %v", err)
			}
			if err := os.Chmod(insecureKey666, 0666); err != nil {
				t.Fatalf("failed to chmod key file: %v", err)
			}

			cfg = ClientConfig{
				KeyFile: insecureKey666,
			}
			_, err = buildAuthMethod(cfg)
			if err == nil {
				t.Fatal("expected error for insecure key file permissions (0666), got nil")
			}
			if !strings.Contains(err.Error(), "insecure permissions") {
				t.Fatalf("expected 'insecure permissions' in error, got: %v", err)
			}
		})
	}

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

// setupLoopbackSSH spins up an in-memory loopback SSH server for testing Client New, Run, Ping, and timeouts.
func setupLoopbackSSH(t *testing.T, handler func(cmd string) (stdout string, stderr string, exitCode uint32, delay time.Duration)) (host string, port int, cleanup func()) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == "gooduser" && string(password) == "goodpass" {
				return nil, nil
			}
			return nil, errors.New("auth failed")
		},
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}

	var wg sync.WaitGroup
	stopped := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			tcpConn, err := listener.Accept()
			if err != nil {
				select {
				case <-stopped:
					return
				default:
					return
				}
			}

			go func(c net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(c, serverConfig)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					ch, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}

					go func(channel ssh.Channel, reqs <-chan *ssh.Request) {
						defer channel.Close()
						for req := range reqs {
							switch req.Type {
							case "exec":
								if len(req.Payload) < 4 {
									_ = req.Reply(false, nil)
									return
								}
								cmdLen := binary.BigEndian.Uint32(req.Payload[:4])
								cmd := string(req.Payload[4 : 4+cmdLen])
								_ = req.Reply(true, nil)

								stdout, stderr, exitCode, delay := handler(cmd)
								if delay > 0 {
									time.Sleep(delay)
								}
								if stdout != "" {
									_, _ = io.WriteString(channel, stdout)
								}
								if stderr != "" {
									_, _ = io.WriteString(channel.Stderr(), stderr)
								}

								status := struct{ Status uint32 }{exitCode}
								_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(&status))
								return
							case "signal":
								// Acknowledge remote signals such as SIGTERM
								_ = req.Reply(true, nil)
							default:
								_ = req.Reply(false, nil)
							}
						}
					}(ch, requests)
				}
				_ = sshConn.Close()
			}(tcpConn)
		}
	}()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	portInt, _ := strconv.Atoi(portStr)

	cleanup = func() {
		close(stopped)
		_ = listener.Close()
		wg.Wait()
	}

	return host, portInt, cleanup
}

func TestClientNewAndPing(t *testing.T) {
	handler := func(cmd string) (string, string, uint32, time.Duration) {
		if strings.Contains(cmd, "svcinfo lssystem") {
			return "id 0\n", "", 0, 0
		}
		return "", "unknown command", 1, 0
	}

	host, port, cleanup := setupLoopbackSSH(t, handler)
	defer cleanup()

	t.Run("SuccessfulConnectionAndPing", func(t *testing.T) {
		client, err := New(ClientConfig{
			Host:            host,
			Port:            port,
			User:            "gooduser",
			Password:        "goodpass",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err != nil {
			t.Fatalf("expected New() to succeed, got: %v", err)
		}
		defer client.Close()

		if err := client.Ping(); err != nil {
			t.Fatalf("expected Ping() to succeed, got: %v", err)
		}
	})

	t.Run("AuthenticationFailure", func(t *testing.T) {
		_, err := New(ClientConfig{
			Host:            host,
			Port:            port,
			User:            "gooduser",
			Password:        "wrongpass",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err == nil {
			t.Fatal("expected authentication failure error, got nil")
		}
		if !strings.Contains(err.Error(), "ssh handshake") {
			t.Fatalf("expected 'ssh handshake' in error, got: %v", err)
		}
	})

	t.Run("TCPDialFailure", func(t *testing.T) {
		// Pick an unused local port
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		_, pStr, _ := net.SplitHostPort(l.Addr().String())
		unusedPort, _ := strconv.Atoi(pStr)
		_ = l.Close()

		_, err = New(ClientConfig{
			Host:            "127.0.0.1",
			Port:            unusedPort,
			User:            "gooduser",
			Password:        "goodpass",
			DialTimeout:     200 * time.Millisecond,
			CommandTimeout:  200 * time.Millisecond,
			InsecureHostKey: true,
		})
		if err == nil {
			t.Fatal("expected TCP dial failure error, got nil")
		}
		if !strings.Contains(err.Error(), "tcp dial") {
			t.Fatalf("expected 'tcp dial' in error, got: %v", err)
		}
	})

	t.Run("PingFailsOnCommandError", func(t *testing.T) {
		failHandler := func(cmd string) (string, string, uint32, time.Duration) {
			return "", "permission denied", 1, 0
		}
		fHost, fPort, fCleanup := setupLoopbackSSH(t, failHandler)
		defer fCleanup()

		client, err := New(ClientConfig{
			Host:            fHost,
			Port:            fPort,
			User:            "gooduser",
			Password:        "goodpass",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err != nil {
			t.Fatalf("unexpected New() error: %v", err)
		}
		defer client.Close()

		err = client.Ping()
		if err == nil {
			t.Fatal("expected Ping() to fail when command fails, got nil")
		}
		if !strings.Contains(err.Error(), "SSH ping failed") {
			t.Fatalf("expected 'SSH ping failed' in error, got: %v", err)
		}
	})
}

func TestClientRunCommand(t *testing.T) {
	handler := func(cmd string) (string, string, uint32, time.Duration) {
		switch cmd {
		case "svcinfo lssystem -delim :":
			return "id:0\nname:Cluster0\n", "", 0, 0
		case "svcinfo fails_with_stderr":
			return "", "CMMVC5753E command failed", 1, 0
		case "svcinfo fails_without_stderr":
			return "", "", 2, 0
		default:
			return "", "unknown", 127, 0
		}
	}

	host, port, cleanup := setupLoopbackSSH(t, handler)
	defer cleanup()

	client, err := New(ClientConfig{
		Host:            host,
		Port:            port,
		User:            "gooduser",
		Password:        "goodpass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  2 * time.Second,
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	t.Run("SuccessfulRun", func(t *testing.T) {
		out, err := client.Run("svcinfo lssystem -delim :")
		if err != nil {
			t.Fatalf("expected Run() to succeed, got: %v", err)
		}
		if !strings.Contains(out, "name:Cluster0") {
			t.Fatalf("unexpected command output: %q", out)
		}
	})

	t.Run("FailedRunWithStderr", func(t *testing.T) {
		_, err := client.Run("svcinfo fails_with_stderr")
		if err == nil {
			t.Fatal("expected error on failed command, got nil")
		}
		if !strings.Contains(err.Error(), "CMMVC5753E command failed") {
			t.Fatalf("expected stderr in error message, got: %v", err)
		}
	})

	t.Run("FailedRunWithoutStderr", func(t *testing.T) {
		_, err := client.Run("svcinfo fails_without_stderr")
		if err == nil {
			t.Fatal("expected error on failed command, got nil")
		}
		if !strings.Contains(err.Error(), "command \"svcinfo fails_without_stderr\" failed") {
			t.Fatalf("expected command failure message, got: %v", err)
		}
	})
}

func TestClientCommandTimeout(t *testing.T) {
	handler := func(cmd string) (string, string, uint32, time.Duration) {
		if cmd == "slow_command" {
			return "done\n", "", 0, 400 * time.Millisecond
		}
		return "fast\n", "", 0, 0
	}

	host, port, cleanup := setupLoopbackSSH(t, handler)
	defer cleanup()

	client, err := New(ClientConfig{
		Host:            host,
		Port:            port,
		User:            "gooduser",
		Password:        "goodpass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  100 * time.Millisecond,
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	_, err = client.Run("slow_command")
	if err == nil {
		t.Fatal("expected command timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected 'timed out after' in error, got: %v", err)
	}
}

func TestClientConcurrentRun(t *testing.T) {
	handler := func(cmd string) (string, string, uint32, time.Duration) {
		return fmt.Sprintf("result_of_%s\n", cmd), "", 0, 10 * time.Millisecond
	}

	host, port, cleanup := setupLoopbackSSH(t, handler)
	defer cleanup()

	client, err := New(ClientConfig{
		Host:            host,
		Port:            port,
		User:            "gooduser",
		Password:        "goodpass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  2 * time.Second,
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	const numWorkers = 10
	var wg sync.WaitGroup
	errs := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			cmd := fmt.Sprintf("cmd_%d", workerID)
			out, err := client.Run(cmd)
			if err != nil {
				errs[workerID] = err
				return
			}
			expected := fmt.Sprintf("result_of_%s\n", cmd)
			if out != expected {
				errs[workerID] = fmt.Errorf("worker %d: expected %q, got %q", workerID, expected, out)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("worker %d encountered error: %v", i, err)
		}
	}
}
