package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// testSSHServer represents a lightweight in-memory SSH server for unit and integration testing.
type testSSHServer struct {
	listener net.Listener
	host     string
	port     int
	hostKey  ssh.Signer
	stopChan chan struct{}
	wg       sync.WaitGroup
}

type serverOptions struct {
	authUsers       map[string]string            // username -> password
	authKeys        map[string]map[string]struct{} // username -> set of authorized public key strings
	commandHandlers map[string]func(cmd string, ch ssh.Channel)
	defaultHandler  func(cmd string, ch ssh.Channel)
}

func startTestSSHServer(t *testing.T, opts serverOptions) *testSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer from host key: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if opts.authUsers != nil {
				if expectedPass, ok := opts.authUsers[conn.User()]; ok && expectedPass == string(pass) {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("authentication failed")
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if opts.authKeys != nil {
				if authorizedKeys, ok := opts.authKeys[conn.User()]; ok {
					keyStr := string(ssh.MarshalAuthorizedKey(key))
					if _, exists := authorizedKeys[keyStr]; exists {
						return nil, nil
					}
				}
			}
			return nil, fmt.Errorf("public key authentication failed")
		},
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}

	tcpAddr := listener.Addr().(*net.TCPAddr)
	srv := &testSSHServer{
		listener: listener,
		host:     tcpAddr.IP.String(),
		port:     tcpAddr.Port,
		hostKey:  signer,
		stopChan: make(chan struct{}),
	}

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-srv.stopChan:
					return
				default:
					return
				}
			}

			srv.wg.Add(1)
			go func(c net.Conn) {
				defer srv.wg.Done()
				defer c.Close()

				sConn, chans, reqs, err := ssh.NewServerConn(c, serverConfig)
				if err != nil {
					return
				}
				defer sConn.Close()

				go ssh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						return
					}

					go func(ch ssh.Channel, reqList <-chan *ssh.Request) {
						defer ch.Close()
						for req := range reqList {
							switch req.Type {
							case "exec":
								cmdLen := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
								cmd := string(req.Payload[4 : 4+cmdLen])
								if req.WantReply {
									req.Reply(true, nil)
								}

								if handler, ok := opts.commandHandlers[cmd]; ok {
									handler(cmd, ch)
								} else if opts.defaultHandler != nil {
									opts.defaultHandler(cmd, ch)
								} else {
									ch.Write([]byte("mock response for: " + cmd + "\n"))
									ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								}
								return
							case "signal":
								if req.WantReply {
									req.Reply(true, nil)
								}
							default:
								if req.WantReply {
									req.Reply(false, nil)
								}
							}
						}
					}(channel, requests)
				}
			}(conn)
		}
	}()

	return srv
}

func (s *testSSHServer) Close() {
	close(s.stopChan)
	_ = s.listener.Close()
	s.wg.Wait()
}

// generateTestKeyFile creates a temporary ED25519 private key in OpenSSH PEM format and returns its path and public key.
func generateTestKeyFile(t *testing.T, dir, filename string) (string, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	privBytes, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	pemBlock := pem.EncodeToMemory(privBytes)
	keyPath := filepath.Join(dir, filename)
	if err := os.WriteFile(keyPath, pemBlock, 0600); err != nil {
		t.Fatalf("failed to write private key file: %v", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to create ssh public key: %v", err)
	}

	return keyPath, sshPub
}

func TestBuildAuthMethod(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("MissingCredentials", func(t *testing.T) {
		cfg := ClientConfig{}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error when no credentials provided, got nil")
		}
		if !strings.Contains(err.Error(), "no authentication method configured") {
			t.Errorf("expected 'no authentication method configured' error, got %v", err)
		}
	})

	t.Run("PasswordAuth", func(t *testing.T) {
		cfg := ClientConfig{Password: "secretpass"}
		auth, err := buildAuthMethod(cfg)
		if err != nil {
			t.Fatalf("expected password auth to succeed, got %v", err)
		}
		if auth == nil {
			t.Fatal("expected non-nil auth method")
		}
	})

	t.Run("ValidKeyFileAuth", func(t *testing.T) {
		keyPath, _ := generateTestKeyFile(t, tempDir, "valid_key")
		cfg := ClientConfig{KeyFile: keyPath}
		auth, err := buildAuthMethod(cfg)
		if err != nil {
			t.Fatalf("expected keyfile auth to succeed, got %v", err)
		}
		if auth == nil {
			t.Fatal("expected non-nil auth method")
		}
	})

	t.Run("NonExistentKeyFile", func(t *testing.T) {
		cfg := ClientConfig{KeyFile: filepath.Join(tempDir, "missing_key")}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error for non-existent keyfile, got nil")
		}
		if !strings.Contains(err.Error(), "cannot read SSH key file") {
			t.Errorf("expected 'cannot read SSH key file' error, got %v", err)
		}
	})

	t.Run("InvalidKeyContent", func(t *testing.T) {
		badKeyPath := filepath.Join(tempDir, "bad_key")
		if err := os.WriteFile(badKeyPath, []byte("invalid-key-data"), 0600); err != nil {
			t.Fatalf("failed to write bad key file: %v", err)
		}
		cfg := ClientConfig{KeyFile: badKeyPath}
		_, err := buildAuthMethod(cfg)
		if err == nil {
			t.Fatal("expected error for invalid key content, got nil")
		}
		if !strings.Contains(err.Error(), "cannot parse SSH private key") {
			t.Errorf("expected 'cannot parse SSH private key' error, got %v", err)
		}
	})
}

func TestBuildHostKeyCallback(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("InsecureHostKey", func(t *testing.T) {
		cfg := ClientConfig{InsecureHostKey: true}
		cb, err := buildHostKeyCallback(cfg)
		if err != nil {
			t.Fatalf("expected insecure host key callback, got %v", err)
		}
		if cb == nil {
			t.Fatal("expected non-nil callback")
		}
	})

	t.Run("ValidKnownHostsFile", func(t *testing.T) {
		khPath := filepath.Join(tempDir, "known_hosts_valid")
		if err := os.WriteFile(khPath, []byte("# empty known_hosts\n"), 0600); err != nil {
			t.Fatalf("failed to create known_hosts: %v", err)
		}
		cfg := ClientConfig{KnownHostsFile: khPath}
		cb, err := buildHostKeyCallback(cfg)
		if err != nil {
			t.Fatalf("expected valid known_hosts callback, got %v", err)
		}
		if cb == nil {
			t.Fatal("expected non-nil callback")
		}
	})

	t.Run("MissingKnownHostsFile", func(t *testing.T) {
		cfg := ClientConfig{KnownHostsFile: filepath.Join(tempDir, "non_existent_kh")}
		_, err := buildHostKeyCallback(cfg)
		if err == nil {
			t.Fatal("expected error for non-existent known_hosts file, got nil")
		}
		if !strings.Contains(err.Error(), "cannot load known_hosts") {
			t.Errorf("expected 'cannot load known_hosts' error, got %v", err)
		}
	})

	t.Run("UnconfiguredVerificationError", func(t *testing.T) {
		// Mock home directory without ~/.ssh/known_hosts
		t.Setenv("HOME", filepath.Join(tempDir, "fakehome"))
		cfg := ClientConfig{
			InsecureHostKey: false,
			KnownHostsFile:  "",
		}
		_, err := buildHostKeyCallback(cfg)
		if err == nil {
			t.Fatal("expected error when host key verification is not configured, got nil")
		}
		if !strings.Contains(err.Error(), "SSH host key verification not configured") {
			t.Errorf("expected unconfigured error message, got %v", err)
		}
	})
}

func TestSSHClientConnectionAndAuthentication(t *testing.T) {
	tempDir := t.TempDir()
	keyPath, pubKey := generateTestKeyFile(t, tempDir, "id_ed25519")
	authorizedPubString := string(ssh.MarshalAuthorizedKey(pubKey))

	srv := startTestSSHServer(t, serverOptions{
		authUsers: map[string]string{
			"monitor": "validPassword123",
		},
		authKeys: map[string]map[string]struct{}{
			"monitor": {
				authorizedPubString: {},
			},
		},
	})
	defer srv.Close()

	t.Run("PasswordAuthSuccess", func(t *testing.T) {
		client, err := New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			Password:        "validPassword123",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err != nil {
			t.Fatalf("expected successful connection, got error: %v", err)
		}
		defer client.Close()

		if client.conn == nil {
			t.Fatal("expected client.conn to be non-nil")
		}
	})

	t.Run("PasswordAuthFailure", func(t *testing.T) {
		_, err := New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			Password:        "wrongPassword",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err == nil {
			t.Fatal("expected connection failure on wrong password, got nil")
		}
		if !strings.Contains(err.Error(), "ssh handshake") {
			t.Errorf("expected ssh handshake failure, got: %v", err)
		}
	})

	t.Run("KeyFileAuthSuccess", func(t *testing.T) {
		client, err := New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			KeyFile:         keyPath,
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err != nil {
			t.Fatalf("expected successful key auth, got error: %v", err)
		}
		defer client.Close()
	})

	t.Run("KeyFileAuthFailure", func(t *testing.T) {
		otherKeyPath, _ := generateTestKeyFile(t, tempDir, "other_ed25519")
		_, err := New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			KeyFile:         otherKeyPath,
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: true,
		})
		if err == nil {
			t.Fatal("expected connection failure on unauthorized key, got nil")
		}
		if !strings.Contains(err.Error(), "ssh handshake") {
			t.Errorf("expected ssh handshake failure, got: %v", err)
		}
	})

	t.Run("DialUnreachableAddress", func(t *testing.T) {
		_, err := New(ClientConfig{
			Host:            "127.0.0.1",
			Port:            1, // Unlikely to be listening
			User:            "monitor",
			Password:        "validPassword123",
			DialTimeout:     100 * time.Millisecond,
			CommandTimeout:  100 * time.Millisecond,
			InsecureHostKey: true,
		})
		if err == nil {
			t.Fatal("expected dial error to closed port, got nil")
		}
		if !strings.Contains(err.Error(), "tcp dial") {
			t.Errorf("expected tcp dial error, got: %v", err)
		}
	})
}

func TestSSHClientKnownHostsVerification(t *testing.T) {
	tempDir := t.TempDir()

	srv := startTestSSHServer(t, serverOptions{
		authUsers: map[string]string{
			"monitor": "pass",
		},
	})
	defer srv.Close()

	serverHostPort := net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
	serverPubKey := srv.hostKey.PublicKey()

	// 1. Build a valid known_hosts entry for the server
	validKHLine := knownhosts.Line([]string{serverHostPort}, serverPubKey)
	validKHFile := filepath.Join(tempDir, "known_hosts_ok")
	if err := os.WriteFile(validKHFile, []byte(validKHLine+"\n"), 0600); err != nil {
		t.Fatalf("failed to write valid known_hosts: %v", err)
	}

	t.Run("KnownHostsMatched", func(t *testing.T) {
		client, err := New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			Password:        "pass",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: false,
			KnownHostsFile:  validKHFile,
		})
		if err != nil {
			t.Fatalf("expected known_hosts match to succeed, got: %v", err)
		}
		client.Close()
	})

	t.Run("KnownHostsMismatchedKey", func(t *testing.T) {
		// Generate an unrelated public key to simulate MITM or changed host key
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate dummy pubkey: %v", err)
		}
		dummySSHPub, err := ssh.NewPublicKey(pub)
		if err != nil {
			t.Fatalf("failed to convert dummy ssh public key: %v", err)
		}

		mismatchedKHLine := knownhosts.Line([]string{serverHostPort}, dummySSHPub)
		mismatchedKHFile := filepath.Join(tempDir, "known_hosts_mismatch")
		if err := os.WriteFile(mismatchedKHFile, []byte(mismatchedKHLine+"\n"), 0600); err != nil {
			t.Fatalf("failed to write mismatched known_hosts: %v", err)
		}

		_, err = New(ClientConfig{
			Host:            srv.host,
			Port:            srv.port,
			User:            "monitor",
			Password:        "pass",
			DialTimeout:     2 * time.Second,
			CommandTimeout:  2 * time.Second,
			InsecureHostKey: false,
			KnownHostsFile:  mismatchedKHFile,
		})
		if err == nil {
			t.Fatal("expected host key mismatch failure, got nil")
		}
		if !strings.Contains(err.Error(), "ssh handshake") {
			t.Errorf("expected ssh handshake error, got: %v", err)
		}
	})
}

func TestSSHClientCommandExecution(t *testing.T) {
	srv := startTestSSHServer(t, serverOptions{
		authUsers: map[string]string{
			"monitor": "pass123",
		},
		commandHandlers: map[string]func(cmd string, ch ssh.Channel){
			"svcinfo lssystem -delim :": func(cmd string, ch ssh.Channel) {
				ch.Write([]byte("id:0\nname:FS5000\ncode_level:8.7.0.0\n"))
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			},
			"svcinfo lssystem -nohdr 2>/dev/null | head -1": func(cmd string, ch ssh.Channel) {
				ch.Write([]byte("0\n"))
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			},
			"failing_command": func(cmd string, ch ssh.Channel) {
				_, _ = ch.Stderr().Write([]byte("CMMVC5753E The command failed\n"))
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 1})
			},
			"hanging_command": func(cmd string, ch ssh.Channel) {
				time.Sleep(500 * time.Millisecond)
				ch.Write([]byte("delayed response\n"))
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			},
		},
	})
	defer srv.Close()

	client, err := New(ClientConfig{
		Host:            srv.host,
		Port:            srv.port,
		User:            "monitor",
		Password:        "pass123",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  100 * time.Millisecond, // short timeout for test
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to create SSH client: %v", err)
	}
	defer client.Close()

	t.Run("SuccessfulRun", func(t *testing.T) {
		out, err := client.Run("svcinfo lssystem -delim :")
		if err != nil {
			t.Fatalf("expected successful Run, got error: %v", err)
		}
		if !strings.Contains(out, "name:FS5000") {
			t.Errorf("expected output to contain 'name:FS5000', got: %q", out)
		}
	})

	t.Run("FailedRunStderrCapture", func(t *testing.T) {
		_, err := client.Run("failing_command")
		if err == nil {
			t.Fatal("expected failing command to return error, got nil")
		}
		errStr := err.Error()
		if !strings.Contains(errStr, "CMMVC5753E") {
			t.Errorf("expected error to include stderr message 'CMMVC5753E', got: %v", errStr)
		}
		if !strings.Contains(errStr, "stderr:") {
			t.Errorf("expected error to contain '(stderr: ...)', got: %v", errStr)
		}
	})

	t.Run("CommandTimeoutHandling", func(t *testing.T) {
		start := time.Now()
		_, err := client.Run("hanging_command")
		duration := time.Since(start)

		if err == nil {
			t.Fatal("expected hanging command to time out, got nil")
		}
		if !strings.Contains(err.Error(), "timed out after") {
			t.Errorf("expected 'timed out after' error, got: %v", err)
		}
		// Confirm timeout happened near 100ms, not after 500ms
		if duration > 400*time.Millisecond {
			t.Errorf("timeout took too long: %v (expected ~100ms)", duration)
		}
	})

	t.Run("PingSuccess", func(t *testing.T) {
		if err := client.Ping(); err != nil {
			t.Fatalf("expected Ping() to succeed, got: %v", err)
		}
	})
}

func TestSSHClientPingFailure(t *testing.T) {
	srv := startTestSSHServer(t, serverOptions{
		authUsers: map[string]string{
			"monitor": "pass",
		},
		commandHandlers: map[string]func(cmd string, ch ssh.Channel){
			"svcinfo lssystem -nohdr 2>/dev/null | head -1": func(cmd string, ch ssh.Channel) {
				_, _ = ch.Stderr().Write([]byte("permission denied\n"))
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 1})
			},
		},
	})
	defer srv.Close()

	client, err := New(ClientConfig{
		Host:            srv.host,
		Port:            srv.port,
		User:            "monitor",
		Password:        "pass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  1 * time.Second,
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	err = client.Ping()
	if err == nil {
		t.Fatal("expected Ping() to fail when command fails, got nil")
	}
	if !strings.Contains(err.Error(), "SSH ping failed") {
		t.Errorf("expected 'SSH ping failed' prefix, got %v", err)
	}
}

func TestSSHClientConcurrentCommands(t *testing.T) {
	srv := startTestSSHServer(t, serverOptions{
		authUsers: map[string]string{
			"monitor": "pass",
		},
		defaultHandler: func(cmd string, ch ssh.Channel) {
			parts := strings.Split(cmd, " ")
			idx := parts[len(parts)-1]
			ch.Write([]byte(fmt.Sprintf("result-%s\n", idx)))
			ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
		},
	})
	defer srv.Close()

	client, err := New(ClientConfig{
		Host:            srv.host,
		Port:            srv.port,
		User:            "monitor",
		Password:        "pass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  2 * time.Second,
		InsecureHostKey: true,
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// Execute multiple parallel commands over the same multiplexed SSH client connection
	var wg sync.WaitGroup
	numWorkers := 10
	errChan := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := fmt.Sprintf("svcinfo get %d", workerID)
			out, err := client.Run(cmd)
			if err != nil {
				errChan <- fmt.Errorf("worker %d failed: %w", workerID, err)
				return
			}
			expected := fmt.Sprintf("result-%d\n", workerID)
			if out != expected {
				errChan <- fmt.Errorf("worker %d got %q, expected %q", workerID, out, expected)
				return
			}
		}()
	}

	wg.Wait()
	close(errChan)

	var errList []string
	for err := range errChan {
		errList = append(errList, err.Error())
	}
	if len(errList) > 0 {
		t.Fatalf("encountered errors during concurrent Run():\n%s", strings.Join(errList, "\n"))
	}
}

func TestSSHClientCloseIdempotency(t *testing.T) {
	// Closing a nil or already closed client
	c := &Client{}
	if err := c.Close(); err != nil {
		t.Errorf("expected nil Close() on uninitialized client, got: %v", err)
	}
}
