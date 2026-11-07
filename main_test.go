package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
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
		{`hello world`, `hello world`},
		{`hello "world"`, `hello \"world\"`},
		{`line1` + "\n" + `line2`, `line1\nline2`},
		{``, ``},
	}

	for _, tt := range tests {
		got := escapeJSONString(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONString(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestEscapeJSONStringEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`tab` + "\t" + `and\backslash`, `tab\tand\\backslash`},
		{`carriage` + "\r" + `return`, `carriage\rreturn`},
		{`"quoted" string`, `\"quoted\" string`},
		{`unicode: ñ, 🚀, 中文`, `unicode: ñ, 🚀, 中文`},
	}

	for _, tt := range tests {
		got := escapeJSONString(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONString(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestPrintUsage(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printUsage()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	usageText := buf.String()

	expectedPhrases := []string{
		"flashsystem-collector",
		"Usage:",
		"collect",
		"discover",
		"-category",
		"-host",
		"-user",
		"-keyfile",
		"drives|pools|volumes|enclosures|nodes|ports|batteries|psus",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(usageText, phrase) {
			t.Errorf("expected usage text to contain %q", phrase)
		}
	}
}

func TestRecordsToLLDItems(t *testing.T) {
	records := []parser.Record{
		{
			"id":     "0",
			"name":   "Pool0",
			"status": "online",
		},
		{
			"id":   "1",
			"name": "Pool1",
		},
	}

	fieldMap := map[string]string{
		"id":     "{#POOLID}",
		"name":   "{#POOLNAME}",
		"status": "{#POOLSTATUS}",
	}

	items := recordsToLLDItems(records, fieldMap)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0]["{#POOLID}"] != "0" || items[0]["{#POOLNAME}"] != "Pool0" || items[0]["{#POOLSTATUS}"] != "online" {
		t.Errorf("unexpected item 0: %+v", items[0])
	}

	if items[1]["{#POOLID}"] != "1" || items[1]["{#POOLNAME}"] != "Pool1" || items[1]["{#POOLSTATUS}"] != "" {
		t.Errorf("unexpected item 1: %+v", items[1])
	}
}

func TestEmitEmptyLLD(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	emitEmptyLLD()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if output != `{"data":[]}` {
		t.Errorf("expected {\"data\":[]}, got %q", output)
	}
}

func TestEmitLLDFromCache(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	items := []map[string]string{
		{"{#DRIVEID}": "0", "{#DRIVESTATUS}": "online"},
	}
	emitLLDFromCache(items)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"{#DRIVEID}": "0"`) || !strings.Contains(output, `"data"`) {
		t.Errorf("expected JSON containing {#DRIVEID}, got %q", output)
	}
}

func TestBuildFromStaleCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "test.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.100"
	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: host,
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	// Populate some stale entries
	key := cache.MakeKey(host, "pools")
	records := []parser.Record{
		{"id": "0", "name": "Pool0", "status": "online"},
	}
	// Expire immediately
	if err := c.Set(key, records, -1*time.Second); err != nil {
		t.Fatalf("failed setting cache: %v", err)
	}

	startTime := time.Now()
	sshErr := errors.New("connection timeout")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	if out.Host != host {
		t.Errorf("expected host %q, got %q", host, out.Host)
	}

	if string(out.Pools) == "[]" {
		t.Errorf("expected pools to be populated from stale cache, got %s", string(out.Pools))
	}

}

func TestBuildFromStaleCacheRejectsTooOld(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "old.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.100"
	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: host,
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	key := cache.MakeKey(host, "pools")
	// Save entry that was cached 25 hours ago (beyond maxStaleAge = 24h)
	oldTime := time.Now().Add(-25 * time.Hour)
	entry := cache.Entry{
		Key:       key,
		Value:     []byte(`[{"id":"0","name":"Pool0","status":"online"}]`),
		CachedAt:  oldTime,
		ExpiresAt: oldTime.Add(5 * time.Minute),
	}

	// Manually save this entry via internal save file test
	cacheData := struct {
		Version   string                 `json:"version"`
		UpdatedAt time.Time              `json:"updated_at"`
		Entries   map[string]cache.Entry `json:"entries"`
	}{
		Version:   "1.0",
		UpdatedAt: oldTime,
		Entries:   map[string]cache.Entry{key: entry},
	}
	data, _ := json.Marshal(cacheData)
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		t.Fatalf("failed to write old cache file: %v", err)
	}

	// Re-load cache from disk
	c = cache.New(cachePath)
	startTime := time.Now()
	sshErr := errors.New("storage unreachable")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	// Pools should NOT be populated because the entry was cached > 24 hours ago
	if string(out.Pools) != "[]" {
		t.Errorf("expected pools to be empty [] due to stale expiration, got: %s", string(out.Pools))
	}
}

func TestRunDiscoveryCollectorInvalidCategory(t *testing.T) {
	cfg := &config.Config{}
	_, _, err := runDiscoveryCollector(nil, "nonexistent_cat", cfg)
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "unknown discovery category") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuildFromStaleCacheCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "corrupt.cache.json")
	c := cache.New(cachePath)

	host := "10.0.0.10"
	cfg := &config.Config{
		SSH: config.SSHConfig{Host: host},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	key := cache.MakeKey(host, "system")
	// Save entry where the Value is valid JSON (so the cache file unmarshals fine with Version: "1")
	// but cannot unmarshal into []parser.Record (e.g. integer or invalid structure for records)
	recentTime := time.Now().Add(-10 * time.Minute)
	entry := cache.Entry{
		Key:       key,
		Value:     []byte(`{"unexpected": "not_an_array_of_records"}`),
		CachedAt:  recentTime,
		ExpiresAt: recentTime.Add(5 * time.Minute),
	}
	cacheData := struct {
		Version   string                 `json:"version"`
		UpdatedAt time.Time              `json:"updated_at"`
		Entries   map[string]cache.Entry `json:"entries"`
	}{
		Version:   "1",
		UpdatedAt: recentTime,
		Entries:   map[string]cache.Entry{key: entry},
	}
	data, _ := json.Marshal(cacheData)
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		t.Fatalf("failed to write corrupted cache file: %v", err)
	}

	c = cache.New(cachePath)
	startTime := time.Now()
	sshErr := errors.New("ssh dial timeout")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if string(out.System) != "[]" {
		t.Errorf("expected system records to be empty [] on corruption, got: %s", string(out.System))
	}
	var foundError bool
	for _, run := range out.Status.CollectorRuns {
		if run.Name == "system" && strings.Contains(run.Error, "cache corrupted") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("expected collector run for system to report cache corrupted, got: %+v", out.Status.CollectorRuns)
	}
}

func TestEmitJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	out := &zabbix.Output{
		Host:      "fs.example.com",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Status: zabbix.CollectionStatus{
			Success: true,
		},
	}

	emitJSON(out)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"host": "fs.example.com"`) {
		t.Errorf("expected host in JSON output, got: %s", output)
	}
}

func setupLoopbackSSHDiscoveryServer(t *testing.T, handler func(cmd string) (stdout string, stderr string, exitCode uint32)) (*internalssh.Client, func()) {
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
		User:            "monitor",
		Password:        "secret",
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

func TestRunDiscoveryCollectorValidCategories(t *testing.T) {
	cmdResponses := map[string]string{
		"svcinfo lsdrive -delim :":            "id:status:capacity:enclosure_id:slot_id:tech_type\n0:online:960247313203:1:1:flash\n",
		"svcinfo lsmdiskgrp -bytes -delim :":  "id:name:status:capacity\n10:pool_tier0:online:1099511627776\n",
		"svcinfo lsvdisk -bytes -delim :":     "id:name:status:mdisk_grp_name\n100:vol_db_01:online:pool_tier0\n",
		"svcinfo lsenclosure -delim :":        "id:status:product_MTM\n1:online:2076-24F\n",
		"svcinfo lsnode -delim :":             "id:name:status:IO_group_name\n1:node_canister_1:online:io_grp0\n",
		"svcinfo lsportfc -delim :":           "id:WWPN:status:node_name:port_speed\n1:500507680100ABCD:active:node_canister_1:16Gb\n",
		"svcinfo lsenclosurebattery -delim :": "enclosure_id:battery_id:status:percent_charged:end_of_life_warning:recondition_needed\n1:1:online:98:no:no\n",
		"svcinfo lsenclosurepsu -delim :":     "enclosure_id:psu_id:status:input_failed:output_failed:fan_failed\n1:1:online:no:no:no\n",
	}

	client, cleanup := setupLoopbackSSHDiscoveryServer(t, func(cmd string) (string, string, uint32) {
		if resp, ok := cmdResponses[cmd]; ok {
			return resp, "", 0
		}
		return "", fmt.Sprintf("unrecognized discovery command %s", cmd), 1
	})
	defer cleanup()

	cfg := &config.Config{
		Cache: config.CacheConfig{
			DiscoveryTTL: 4 * time.Hour,
			MetricsTTL:   5 * time.Minute,
		},
	}

	testCases := []struct {
		category       string
		expectedMacro  string
		expectedVal    string
		expectedRecord int
	}{
		{
			category:       "drives",
			expectedMacro:  "{#DRIVEID}",
			expectedVal:    "0",
			expectedRecord: 1,
		},
		{
			category:       "pools",
			expectedMacro:  "{#POOLNAME}",
			expectedVal:    "pool_tier0",
			expectedRecord: 1,
		},
		{
			category:       "volumes",
			expectedMacro:  "{#VOLUMENAME}",
			expectedVal:    "vol_db_01",
			expectedRecord: 1,
		},
		{
			category:       "enclosures",
			expectedMacro:  "{#ENCLOSUREMODEL}",
			expectedVal:    "2076-24F",
			expectedRecord: 1,
		},
		{
			category:       "nodes",
			expectedMacro:  "{#NODENAME}",
			expectedVal:    "node_canister_1",
			expectedRecord: 1,
		},
		{
			category:       "ports",
			expectedMacro:  "{#PORTSTATUS}",
			expectedVal:    "active",
			expectedRecord: 1,
		},
		{
			category:       "batteries",
			expectedMacro:  "{#BATTERYCHARGE}",
			expectedVal:    "98",
			expectedRecord: 1,
		},
		{
			category:       "psus",
			expectedMacro:  "{#PSUID}",
			expectedVal:    "1",
			expectedRecord: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.category, func(t *testing.T) {
			records, fieldMap, err := runDiscoveryCollector(client, tc.category, cfg)
			if err != nil {
				t.Fatalf("runDiscoveryCollector failed for %s: %v", tc.category, err)
			}
			if len(records) != tc.expectedRecord {
				t.Fatalf("expected %d records for %s, got %d", tc.expectedRecord, tc.category, len(records))
			}
			if len(fieldMap) == 0 {
				t.Fatalf("expected non-empty fieldMap for %s", tc.category)
			}

			// Validate conversion to Zabbix LLD items
			lldItems := recordsToLLDItems(records, fieldMap)
			if len(lldItems) != tc.expectedRecord {
				t.Fatalf("expected %d LLD items for %s, got %d", tc.expectedRecord, tc.category, len(lldItems))
			}

			item := lldItems[0]
			val, ok := item[tc.expectedMacro]
			if !ok {
				t.Fatalf("macro %s missing from LLD item: %+v", tc.expectedMacro, item)
			}
			if val != tc.expectedVal {
				t.Errorf("macro %s = %q, expected %q", tc.expectedMacro, val, tc.expectedVal)
			}
		})
	}
}

func TestRunDiscoveryCollectorExecutionFailure(t *testing.T) {
	client, cleanup := setupLoopbackSSHDiscoveryServer(t, func(cmd string) (string, string, uint32) {
		return "", "CMMVC5753E connection lost or command error", 1
	})
	defer cleanup()

	cfg := &config.Config{
		Cache: config.CacheConfig{
			DiscoveryTTL: 4 * time.Hour,
			MetricsTTL:   5 * time.Minute,
		},
	}

	_, _, err := runDiscoveryCollector(client, "drives", cfg)
	if err == nil {
		t.Fatal("expected error on command failure, got nil")
	}
	if !strings.Contains(err.Error(), `collector "drives" failed`) {
		t.Errorf("unexpected error format: %v", err)
	}
}

func TestDialSSH(t *testing.T) {
	// Setup a dedicated server for dialSSH to test config mapping
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
	defer listener.Close()

	go func() {
		for {
			tcpConn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, chans, reqs, err := ssh.NewServerConn(c, serverConfig)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.UnknownChannelType, "rejected")
				}
			}(tcpConn)
		}
	}()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	t.Run("SuccessfulDial", func(t *testing.T) {
		cfg := &config.Config{
			SSH: config.SSHConfig{
				Host:            host,
				Port:            port,
				User:            "gooduser",
				Password:        "goodpass",
				DialTimeout:     2 * time.Second,
				CommandTimeout:  2 * time.Second,
				InsecureHostKey: true,
			},
		}

		c, err := dialSSH(cfg)
		if err != nil {
			t.Fatalf("expected dialSSH to succeed, got: %v", err)
		}
		defer c.Close()
	})

	t.Run("FailedDialAuth", func(t *testing.T) {
		cfg := &config.Config{
			SSH: config.SSHConfig{
				Host:            host,
				Port:            port,
				User:            "gooduser",
				Password:        "wrongpass",
				DialTimeout:     2 * time.Second,
				CommandTimeout:  2 * time.Second,
				InsecureHostKey: true,
			},
		}

		_, err := dialSSH(cfg)
		if err == nil {
			t.Fatal("expected dialSSH to fail with wrong password, got nil")
		}
	})

	t.Run("FailedDialConnectionRefused", func(t *testing.T) {
		// Pick an unused local port
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		_, pStr, _ := net.SplitHostPort(l.Addr().String())
		unusedPort, _ := strconv.Atoi(pStr)
		_ = l.Close()

		cfg := &config.Config{
			SSH: config.SSHConfig{
				Host:            "127.0.0.1",
				Port:            unusedPort,
				User:            "gooduser",
				Password:        "goodpass",
				DialTimeout:     200 * time.Millisecond,
				CommandTimeout:  200 * time.Millisecond,
				InsecureHostKey: true,
			},
		}

		_, err = dialSSH(cfg)
		if err == nil {
			t.Fatal("expected dialSSH to fail on closed port, got nil")
		}
	})
}

func TestBuildFromStaleCacheAllCollectors(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "all_collectors.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.200"
	cfg := &config.Config{
		SSH: config.SSHConfig{
			Host: host,
		},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	allCollectorNames := []string{
		"system", "nodes", "enclosures", "drives",
		"pools", "volumes", "ports", "flashcopy",
		"replication", "performance", "batteries", "psus",
	}

	for _, name := range allCollectorNames {
		key := cache.MakeKey(host, name)
		recs := []parser.Record{
			{"id": "0", "name": name + "_item", "status": "online"},
		}
		// Set expired 1 minute ago, well within 24h maxStaleAge
		if err := c.Set(key, recs, -1*time.Minute); err != nil {
			t.Fatalf("failed setting cache for %s: %v", name, err)
		}
	}

	startTime := time.Now()
	sshErr := errors.New("storage management offline")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	sections := map[string]json.RawMessage{
		"system":      out.System,
		"nodes":       out.Nodes,
		"enclosures":  out.Enclosures,
		"drives":      out.Drives,
		"pools":       out.Pools,
		"volumes":     out.Volumes,
		"ports":       out.Ports,
		"flashcopy":   out.FlashCopy,
		"replication": out.Replication,
		"performance": out.Performance,
		"batteries":   out.Batteries,
		"psus":        out.PSUs,
	}

	for name, raw := range sections {
		if string(raw) == "[]" {
			t.Errorf("section %s was expected to be populated from stale cache, got []", name)
		}
		var parsed []parser.Record
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Errorf("section %s failed to unmarshal: %v", name, err)
		} else if len(parsed) != 1 || parsed[0]["name"] != name+"_item" {
			t.Errorf("section %s record mismatch: %+v", name, parsed)
		}
	}

	if len(out.Status.CollectorRuns) != 12 {
		t.Errorf("expected 12 collector runs in status, got %d", len(out.Status.CollectorRuns))
	}
	for _, run := range out.Status.CollectorRuns {
		if !run.CacheHit {
			t.Errorf("expected CacheHit true for collector %s", run.Name)
		}
		if !strings.Contains(run.Error, "stale cache from") {
			t.Errorf("expected stale cache note in error for collector %s, got: %s", run.Name, run.Error)
		}
	}
}

func TestBuildFromStaleCachePartialAndMissing(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "partial.cache.json")
	c := cache.New(cachePath)

	host := "192.168.1.201"
	cfg := &config.Config{
		SSH: config.SSHConfig{Host: host},
		Limits: config.LimitsConfig{
			MaxJSONBytes: 1048576,
			MaxVolumes:   500,
			MaxDrives:    500,
		},
	}

	// Only populate system and pools in cache
	_ = c.Set(cache.MakeKey(host, "system"), []parser.Record{{"id": "0", "status": "online"}}, -1*time.Minute)
	_ = c.Set(cache.MakeKey(host, "pools"), []parser.Record{{"id": "1", "name": "p1"}}, -1*time.Minute)

	startTime := time.Now()
	sshErr := errors.New("network unreachable")

	out := buildFromStaleCache(c, cfg, startTime, sshErr)
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	// System and pools should have records
	if string(out.System) == "[]" {
		t.Error("expected system to be populated from stale cache")
	}
	if string(out.Pools) == "[]" {
		t.Error("expected pools to be populated from stale cache")
	}

	// Others like volumes, drives, batteries must safely default to "[]"
	if string(out.Volumes) != "[]" {
		t.Errorf("expected empty [] for unpopulated volumes, got: %s", string(out.Volumes))
	}
	if string(out.Drives) != "[]" {
		t.Errorf("expected empty [] for unpopulated drives, got: %s", string(out.Drives))
	}
	if string(out.Batteries) != "[]" {
		t.Errorf("expected empty [] for unpopulated batteries, got: %s", string(out.Batteries))
	}

	// Verify error descriptions for unpopulated items
	var foundMissingVolumeErr bool
	for _, run := range out.Status.CollectorRuns {
		if run.Name == "volumes" && strings.Contains(run.Error, "no cache available") {
			foundMissingVolumeErr = true
			break
		}
	}
	if !foundMissingVolumeErr {
		t.Errorf("expected 'no cache available' error for volumes, got runs: %+v", out.Status.CollectorRuns)
	}
}
