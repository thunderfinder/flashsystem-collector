package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/config"
	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
	"github.com/flashsystem-collector/internal/zabbix"
	"golang.org/x/crypto/ssh"
)

func TestEscapeJSONString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{`quote "test"`, `quote \"test\"`},
		{"line\nbreak", `line\nbreak`},
		{"tab\tseparated", `tab\tseparated`},
		{`backslash \ path`, `backslash \\ path`},
		{"", ""},
	}

	for _, tt := range tests {
		got := escapeJSONString(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONString(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestRecordsToLLDItems(t *testing.T) {
	fieldMap := map[string]string{
		"id":     "{#ID}",
		"name":   "{#NAME}",
		"status": "{#STATUS}",
	}

	t.Run("PopulatedRecords", func(t *testing.T) {
		records := []parser.Record{
			{"id": "0", "name": "vol0", "status": "online"},
			{"id": "1", "name": "vol1", "status": "offline"},
		}

		items := recordsToLLDItems(records, fieldMap)
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if items[0]["{#ID}"] != "0" || items[0]["{#NAME}"] != "vol0" || items[0]["{#STATUS}"] != "online" {
			t.Errorf("unexpected items[0]: %+v", items[0])
		}
		if items[1]["{#ID}"] != "1" || items[1]["{#NAME}"] != "vol1" || items[1]["{#STATUS}"] != "offline" {
			t.Errorf("unexpected items[1]: %+v", items[1])
		}
	})

	t.Run("MissingFieldsDefaultToEmptyString", func(t *testing.T) {
		records := []parser.Record{
			{"id": "2"}, // missing "name" and "status"
		}

		items := recordsToLLDItems(records, fieldMap)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0]["{#ID}"] != "2" {
			t.Errorf("expected {#ID}='2', got %q", items[0]["{#ID}"])
		}
		if items[0]["{#NAME}"] != "" {
			t.Errorf("expected empty string for missing {#NAME}, got %q", items[0]["{#NAME}"])
		}
		if items[0]["{#STATUS}"] != "" {
			t.Errorf("expected empty string for missing {#STATUS}, got %q", items[0]["{#STATUS}"])
		}
	})

	t.Run("EmptyRecords", func(t *testing.T) {
		items := recordsToLLDItems([]parser.Record{}, fieldMap)
		if len(items) != 0 {
			t.Errorf("expected 0 items for empty records, got %d", len(items))
		}
	})
}

func TestBuildFromStaleCache(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "stale_test.cache.json")
	c := cache.New(cachePath)

	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: "10.0.0.1",
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	startTime := time.Now().Add(-200 * time.Millisecond)
	sshErr := errors.New("dial tcp 10.0.0.1:22: connect: connection refused")

	t.Run("NoCacheEntriesAvailable", func(t *testing.T) {
		out := buildFromStaleCache(c, cfg, startTime, sshErr)
		if out == nil {
			t.Fatal("expected non-nil output")
		}
		if out.Host != "10.0.0.1" {
			t.Errorf("expected host 10.0.0.1, got %s", out.Host)
		}
		if out.Version != version {
			t.Errorf("expected version %s, got %s", version, out.Version)
		}
		// Since all collectors failed and no cache was found, success must be false
		if out.Status.Success {
			t.Errorf("expected status.success false when no cache available and SSH fails")
		}
		if len(out.Status.Errors) == 0 {
			t.Fatalf("expected status.errors to be populated")
		}
		// Verify all 12 collectors are registered in CollectorRuns
		if len(out.Status.CollectorRuns) != 12 {
			t.Errorf("expected 12 CollectorRuns, got %d", len(out.Status.CollectorRuns))
		}
		for _, run := range out.Status.CollectorRuns {
			if !strings.Contains(run.Error, "SSH failed and no cache available") {
				t.Errorf("collector %s: expected 'no cache available' error, got %q", run.Name, run.Error)
			}
			if run.CacheHit {
				t.Errorf("collector %s: expected CacheHit false", run.Name)
			}
		}
	})

	t.Run("WithStaleCacheEntries", func(t *testing.T) {
		// Populate expired entries for system and pools
		sysKey := cache.MakeKey("10.0.0.1", "system")
		poolsKey := cache.MakeKey("10.0.0.1", "pools")

		sysRecs := []parser.Record{{"id": "0", "name": "FS_Cluster", "status": "online"}}
		poolsRecs := []parser.Record{{"id": "0", "name": "Pool0", "status": "online"}}

		_ = c.Set(sysKey, sysRecs, -5*time.Minute)
		_ = c.Set(poolsKey, poolsRecs, -5*time.Minute)

		out := buildFromStaleCache(c, cfg, startTime, sshErr)
		if out == nil {
			t.Fatal("expected non-nil output")
		}
		// Because SSH dial failed for the collection cycle, every collector result contains
		// an error (either "SSH failed (stale cache ...)" or "SSH failed and no cache available"),
		// so Status.Success is correctly false, but the stale records are preserved in payload.
		if out.Status.Success {
			t.Errorf("expected status.success false when SSH fails, got true")
		}

		runsByName := make(map[string]zabbix.CollectorRun)
		for _, r := range out.Status.CollectorRuns {
			runsByName[r.Name] = r
		}

		sysRun, ok := runsByName["system"]
		if !ok {
			t.Fatal("missing 'system' collector run")
		}
		if !sysRun.CacheHit {
			t.Errorf("expected system CacheHit true")
		}
		if sysRun.RecordCount != 1 {
			t.Errorf("expected system RecordCount 1, got %d", sysRun.RecordCount)
		}
		if !strings.Contains(sysRun.Error, "stale cache from") {
			t.Errorf("expected stale cache note in error, got %q", sysRun.Error)
		}
		if sysRun.CachedAt == nil {
			t.Errorf("expected CachedAt to be non-nil for system")
		}
	})

	t.Run("WithCorruptedCacheEntry", func(t *testing.T) {
		corruptKey := cache.MakeKey("10.0.0.1", "nodes")
		// Insert corrupted shape (not []parser.Record)
		_ = c.Set(corruptKey, 12345, -5*time.Minute)

		out := buildFromStaleCache(c, cfg, startTime, sshErr)
		runsByName := make(map[string]zabbix.CollectorRun)
		for _, r := range out.Status.CollectorRuns {
			runsByName[r.Name] = r
		}

		nodeRun, ok := runsByName["nodes"]
		if !ok {
			t.Fatal("missing 'nodes' collector run")
		}
		if !strings.Contains(nodeRun.Error, "SSH failed and cache corrupted") {
			t.Errorf("expected 'cache corrupted' in error, got %q", nodeRun.Error)
		}
	})
}

// setupTestSSHServer sets up an in-memory loopback SSH server for discovery testing.
func setupTestSSHServer(t *testing.T, handler func(cmd string) (stdout string, stderr string, exitCode uint32)) (*internalssh.Client, func()) {
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
			return nil, nil
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
							if req.Type == "exec" {
								if len(req.Payload) < 4 {
									_ = req.Reply(false, nil)
									return
								}
								cmdLen := binary.BigEndian.Uint32(req.Payload[:4])
								cmd := string(req.Payload[4 : 4+cmdLen])
								_ = req.Reply(true, nil)

								stdout, stderr, exitCode := handler(cmd)
								if stdout != "" {
									_, _ = io.WriteString(channel, stdout)
								}
								if stderr != "" {
									_, _ = io.WriteString(channel.Stderr(), stderr)
								}

								status := struct{ Status uint32 }{exitCode}
								_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(&status))
								return
							}
							_ = req.Reply(false, nil)
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
	port, _ := strconv.Atoi(portStr)

	client, err := internalssh.New(internalssh.ClientConfig{
		Host:            host,
		Port:            port,
		User:            "testuser",
		Password:        "testpass",
		DialTimeout:     2 * time.Second,
		CommandTimeout:  2 * time.Second,
		InsecureHostKey: true,
	})
	if err != nil {
		_ = listener.Close()
		close(stopped)
		t.Fatalf("failed to create client: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		close(stopped)
		_ = listener.Close()
		wg.Wait()
	}

	return client, cleanup
}

func TestRunDiscoveryCollector(t *testing.T) {
	handler := func(cmd string) (string, string, uint32) {
		switch cmd {
		case "svcinfo lsdrive -delim :":
			return "id:status:enclosure_id:slot_id:tech_type:capacity\n0:online:1:1:flash:894.3GB\n", "", 0
		case "svcinfo lsmdiskgrp -bytes -delim :":
			return "id:name:status\n0:Pool0:online\n", "", 0
		case "svcinfo lsvdisk -bytes -delim :":
			return "id:name:status:mdisk_grp_name\n0:vol0:online:Pool0\n", "", 0
		case "svcinfo lsenclosure -delim :":
			return "id:status:product_MTM\n1:online:2076-24C\n", "", 0
		case "svcinfo lsnode -delim :":
			return "id:name:status:IO_group_name\n1:node1:online:io_grp0\n", "", 0
		case "svcinfo lsportfc -delim :":
			return "id:WWPN:status:node_name:port_speed\n1:5005076801001234:active:node1:16Gb\n", "", 0
		case "svcinfo lsenclosurebattery -delim :":
			return "enclosure_id:battery_id:status:percent_charged:end_of_life_warning:recondition_needed\n1:1:online:100:no:no\n", "", 0
		case "svcinfo lsenclosurepsu -delim :":
			return "enclosure_id:psu_id:status:input_failed:output_failed:fan_failed\n1:1:online:no:no:no\n", "", 0
		default:
			return "", fmt.Sprintf("unhandled command: %s", cmd), 1
		}
	}

	client, cleanup := setupTestSSHServer(t, handler)
	defer cleanup()

	cfg := &config.Config{
		Cache: config.CacheConfig{
			DiscoveryTTL: 4 * time.Hour,
			MetricsTTL:   5 * time.Minute,
		},
	}

	validCategories := []struct {
		category string
		expField string
	}{
		{"drives", "{#DRIVEID}"},
		{"pools", "{#POOLNAME}"},
		{"volumes", "{#VOLUMENAME}"},
		{"enclosures", "{#ENCLOSUREMODEL}"},
		{"nodes", "{#NODENAME}"},
		{"ports", "{#PORTID}"},
		{"batteries", "{#BATTERYCHARGE}"},
		{"psus", "{#PSUSTATUS}"},
	}

	for _, tc := range validCategories {
		t.Run("Category_"+tc.category, func(t *testing.T) {
			recs, fieldMap, err := runDiscoveryCollector(client, tc.category, cfg)
			if err != nil {
				t.Fatalf("unexpected error for category %q: %v", tc.category, err)
			}
			if len(recs) != 1 {
				t.Fatalf("expected 1 record for category %q, got %d", tc.category, len(recs))
			}
			foundMacro := false
			for _, macro := range fieldMap {
				if macro == tc.expField {
					foundMacro = true
					break
				}
			}
			if !foundMacro {
				t.Errorf("category %q fieldMap missing expected macro %q", tc.category, tc.expField)
			}

			// Validate conversion to LLD items works cleanly
			lldItems := recordsToLLDItems(recs, fieldMap)
			if len(lldItems) != 1 {
				t.Fatalf("expected 1 LLD item, got %d", len(lldItems))
			}
			if _, ok := lldItems[0][tc.expField]; !ok {
				t.Errorf("expected macro %s in generated LLD item", tc.expField)
			}
		})
	}

	t.Run("UnknownCategoryFails", func(t *testing.T) {
		_, _, err := runDiscoveryCollector(client, "invalid_category", cfg)
		if err == nil {
			t.Fatal("expected error for unknown category, got nil")
		}
		if !strings.Contains(err.Error(), "unknown discovery category") {
			t.Errorf("expected 'unknown discovery category' in error message, got: %v", err)
		}
	})

	t.Run("CollectorFailureReturnsError", func(t *testing.T) {
		// Close client connection to simulate network drop
		_ = client.Close()
		_, _, err := runDiscoveryCollector(client, "drives", cfg)
		if err == nil {
			t.Fatal("expected error on closed client, got nil")
		}
		if !strings.Contains(err.Error(), `collector "drives" failed`) {
			t.Errorf("expected collector failure wrapper, got: %v", err)
		}
	})
}
