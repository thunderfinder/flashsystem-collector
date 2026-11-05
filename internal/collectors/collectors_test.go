package collectors

import (
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
	"golang.org/x/crypto/ssh"
)

func TestAllCollectorsRegistration(t *testing.T) {
	metricsTTL := 5 * time.Minute
	discoveryTTL := 4 * time.Hour
	perfTTL := 60 * time.Second

	all := AllCollectors(metricsTTL, discoveryTTL, perfTTL)
	if len(all) != 12 {
		t.Fatalf("expected 12 registered collectors, got %d", len(all))
	}

	names := make(map[string]time.Duration)
	for _, c := range all {
		names[c.Name()] = c.TTL()
	}

	expected := map[string]time.Duration{
		"system":      metricsTTL,
		"nodes":       metricsTTL,
		"enclosures":  discoveryTTL,
		"drives":      discoveryTTL,
		"pools":       metricsTTL,
		"volumes":     metricsTTL,
		"ports":       discoveryTTL,
		"flashcopy":   metricsTTL,
		"replication": metricsTTL,
		"performance": perfTTL,
		"batteries":   discoveryTTL,
		"psus":        discoveryTTL,
	}

	for expName, expTTL := range expected {
		ttl, ok := names[expName]
		if !ok {
			t.Errorf("collector %q was not registered in AllCollectors", expName)
		} else if ttl != expTTL {
			t.Errorf("collector %q expected TTL %v, got %v", expName, expTTL, ttl)
		}
	}
}

type mockCollector struct {
	name     string
	ttl      time.Duration
	collect  func(client *internalssh.Client) ([]parser.Record, error)
	runCount int32
}

func (m *mockCollector) Name() string       { return m.name }
func (m *mockCollector) TTL() time.Duration { return m.ttl }
func (m *mockCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	atomic.AddInt32(&m.runCount, 1)
	if m.collect != nil {
		return m.collect(client)
	}
	return []parser.Record{{"id": "0", "status": "online"}}, nil
}

func TestRunner(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("DefaultMaxConcurrent", func(t *testing.T) {
		c := cache.New(filepath.Join(tempDir, "cache_default.json"))
		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 0}, c, nil)
		if runner.cfg.MaxConcurrent != 3 {
			t.Errorf("expected default MaxConcurrent 3, got %d", runner.cfg.MaxConcurrent)
		}
	})

	t.Run("CacheMissSuccess", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_miss.json")
		c := cache.New(cacheFile)

		col := &mockCollector{
			name: "nodes",
			ttl:  10 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return []parser.Record{{"id": "1", "name": "node1"}}, nil
			},
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 2}, c, []Collector{col})
		results := runner.RunAll(nil)

		res, ok := results["nodes"]
		if !ok {
			t.Fatal("expected 'nodes' in results")
		}
		if res.CacheHit {
			t.Error("expected CacheHit to be false on initial run")
		}
		if res.Error != "" {
			t.Errorf("expected no error, got: %s", res.Error)
		}
		if len(res.Records) != 1 || res.Records[0]["name"] != "node1" {
			t.Errorf("unexpected records: %+v", res.Records)
		}

		// Verify it was stored in cache
		key := cache.MakeKey("10.0.0.1", "nodes")
		entry, ok := c.Get(key)
		if !ok {
			t.Fatal("expected entry to be cached in cache")
		}
		var cachedRecs []parser.Record
		if err := entry.Unmarshal(&cachedRecs); err != nil {
			t.Fatalf("failed to unmarshal cached records: %v", err)
		}
		if len(cachedRecs) != 1 || cachedRecs[0]["name"] != "node1" {
			t.Errorf("cached records mismatch: %+v", cachedRecs)
		}
	})

	t.Run("CacheHitBypassesSSH", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_hit.json")
		c := cache.New(cacheFile)

		col := &mockCollector{
			name: "pools",
			ttl:  10 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return []parser.Record{{"id": "0", "name": "pool0"}}, nil
			},
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 2}, c, []Collector{col})

		// First run: miss
		res1 := runner.RunAll(nil)
		if res1["pools"].CacheHit {
			t.Error("first run should be cache miss")
		}
		if atomic.LoadInt32(&col.runCount) != 1 {
			t.Fatalf("expected runCount 1, got %d", col.runCount)
		}

		// Second run: hit
		res2 := runner.RunAll(nil)
		poolRes := res2["pools"]
		if !poolRes.CacheHit {
			t.Error("second run should be cache hit")
		}
		if poolRes.CachedAt == nil {
			t.Error("expected CachedAt to be populated on cache hit")
		}
		if poolRes.Error != "" {
			t.Errorf("expected no error on cache hit, got %s", poolRes.Error)
		}
		if len(poolRes.Records) != 1 || poolRes.Records[0]["name"] != "pool0" {
			t.Errorf("unexpected cached records: %+v", poolRes.Records)
		}
		// Collector's Collect method should not have been called a second time
		if atomic.LoadInt32(&col.runCount) != 1 {
			t.Errorf("expected runCount to remain 1 on cache hit, got %d", col.runCount)
		}
	})

	t.Run("StaleCacheFallbackOnError", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_stale.json")
		c := cache.New(cacheFile)

		key := cache.MakeKey("10.0.0.1", "volumes")
		// Manually populate cache with an already expired entry
		initialRecs := []parser.Record{{"id": "0", "name": "vol0"}}
		_ = c.Set(key, initialRecs, -1*time.Minute)

		// Verify Get returns false (expired) while GetStale returns true
		if _, ok := c.Get(key); ok {
			t.Fatal("expected entry to be expired in normal Get")
		}
		if _, ok := c.GetStale(key); !ok {
			t.Fatal("expected entry to be accessible via GetStale")
		}

		col := &mockCollector{
			name: "volumes",
			ttl:  5 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return nil, errors.New("connection refused")
			},
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 2}, c, []Collector{col})
		results := runner.RunAll(nil)

		res := results["volumes"]
		if !res.CacheHit {
			t.Error("expected CacheHit true when fallback to stale cache")
		}
		if res.CachedAt == nil {
			t.Error("expected CachedAt not nil when fallback to stale cache")
		}
		if len(res.Records) != 1 || res.Records[0]["name"] != "vol0" {
			t.Errorf("expected stale records returned, got: %+v", res.Records)
		}
		if res.Error == "" || !strings.Contains(res.Error, "SSH failed (using stale cache)") {
			t.Errorf("expected stale cache note in error, got: %s", res.Error)
		}
	})

	t.Run("StaleCacheExpiredBeyondMaxStaleAge", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_stale_too_old.json")
		c := cache.New(cacheFile)

		key := cache.MakeKey("10.0.0.1", "volumes")
		oldTime := time.Now().Add(-25 * time.Hour)
		entry := cache.Entry{
			Key:       key,
			Value:     []byte(`[{"id":"0","name":"vol0"}]`),
			CachedAt:  oldTime,
			ExpiresAt: oldTime.Add(5 * time.Minute),
		}

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
		if err := os.WriteFile(cacheFile, data, 0600); err != nil {
			t.Fatalf("failed to write old cache file: %v", err)
		}

		c = cache.New(cacheFile)

		col := &mockCollector{
			name: "volumes",
			ttl:  5 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return nil, errors.New("storage offline")
			},
		}

		runner := NewRunner(RunnerConfig{
			Host:          "10.0.0.1",
			MaxConcurrent: 2,
			MaxStaleAge:   24 * time.Hour,
		}, c, []Collector{col})

		results := runner.RunAll(nil)
		res := results["volumes"]

		// Because entry is 25 hours old, it should not be accepted as stale
		if res.CacheHit {
			t.Error("expected CacheHit false because stale entry exceeds MaxStaleAge")
		}
		if len(res.Records) != 0 {
			t.Errorf("expected 0 records for expired stale entry, got %d", len(res.Records))
		}
		if res.Error != "storage offline" {
			t.Errorf("expected direct error 'storage offline', got %q", res.Error)
		}
	})

	t.Run("SSHFailureWithoutCache", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_empty_err.json")
		c := cache.New(cacheFile)

		col := &mockCollector{
			name: "ports",
			ttl:  5 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return nil, errors.New("timeout reached")
			},
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 2}, c, []Collector{col})
		results := runner.RunAll(nil)

		res := results["ports"]
		if res.CacheHit {
			t.Error("expected CacheHit false")
		}
		if res.CachedAt != nil {
			t.Error("expected CachedAt nil")
		}
		if len(res.Records) != 0 {
			t.Errorf("expected empty records slice, got %d records", len(res.Records))
		}
		if res.Error != "timeout reached" {
			t.Errorf("expected 'timeout reached' error, got %q", res.Error)
		}
	})

	t.Run("CorruptCacheFallsBackToSSH", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_corrupt.json")
		c := cache.New(cacheFile)

		key := cache.MakeKey("10.0.0.1", "system")
		// Put an invalid type (string instead of []parser.Record) in cache entry
		_ = c.Set(key, "invalid_json_shape_not_records", 5*time.Minute)

		col := &mockCollector{
			name: "system",
			ttl:  5 * time.Minute,
			collect: func(client *internalssh.Client) ([]parser.Record, error) {
				return []parser.Record{{"id": "0", "status": "online"}}, nil
			},
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: 2}, c, []Collector{col})
		results := runner.RunAll(nil)

		res := results["system"]
		if res.CacheHit {
			t.Error("expected CacheHit false when cache entry unmarshal fails")
		}
		if len(res.Records) != 1 || res.Records[0]["status"] != "online" {
			t.Errorf("expected fresh collection records, got %+v", res.Records)
		}
	})

	t.Run("ConcurrencyAndSemaphoreLimit", func(t *testing.T) {
		cacheFile := filepath.Join(tempDir, "cache_concurrency.json")
		c := cache.New(cacheFile)

		const maxConcurrent = 2
		const numCollectors = 6

		var activeCount int32
		var maxObserved int32

		cols := make([]Collector, numCollectors)
		for i := 0; i < numCollectors; i++ {
			idx := i
			cols[i] = &mockCollector{
				name: filepath.Join("col", string(rune('a'+idx))),
				ttl:  1 * time.Minute,
				collect: func(client *internalssh.Client) ([]parser.Record, error) {
					cur := atomic.AddInt32(&activeCount, 1)
					for {
						oldMax := atomic.LoadInt32(&maxObserved)
						if cur <= oldMax || atomic.CompareAndSwapInt32(&maxObserved, oldMax, cur) {
							break
						}
					}
					time.Sleep(20 * time.Millisecond)
					atomic.AddInt32(&activeCount, -1)
					return []parser.Record{{"index": string(rune('0' + idx))}}, nil
				},
			}
		}

		runner := NewRunner(RunnerConfig{Host: "10.0.0.1", MaxConcurrent: maxConcurrent}, c, cols)
		results := runner.RunAll(nil)

		if len(results) != numCollectors {
			t.Fatalf("expected %d results, got %d", numCollectors, len(results))
		}
		if maxObserved > maxConcurrent {
			t.Errorf("observed active concurrency %d exceeded MaxConcurrent %d", maxObserved, maxConcurrent)
		}
	})
}

// setupLoopbackSSHServer spins up an in-memory loopback SSH server responding to commands.
func setupLoopbackSSHServer(t *testing.T, handler func(cmd string) (stdout string, stderr string, exitCode uint32)) (*internalssh.Client, func()) {
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

func TestCollectorsWithSSH(t *testing.T) {
	t.Run("StandardCollectorsSuccess", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			switch cmd {
			case "svcinfo lssystem -delim :":
				return "id:0\nname:FS5045\ncode_level:8.7.0.0 (build 123.456)\nstatus:online\n", "", 0
			case "svcinfo lsnode -delim :":
				return "id:name:status\n1:node1:online\n2:node2:online\n", "", 0
			case "svcinfo lsenclosure -delim :":
				return "id:status:type\n1:online:control\n", "", 0
			case "svcinfo lsdrive -delim :":
				return "id:status:capacity\n0:online:960247313203\n", "", 0
			case "svcinfo lsmdiskgrp -bytes -delim :":
				return "id:name:status:capacity\n0:Pool0:online:48378378240\n", "", 0
			case "svcinfo lsvdisk -bytes -delim :":
				return "id:name:status:capacity\n0:vol0:online:10737418240\n", "", 0
			case "svcinfo lsportfc -delim :":
				return "id:node_id:status:speed\n1:1:active:16Gb\n", "", 0
			case "svcinfo lsfcmap -delim :":
				return "id:name:source_vdisk_name:target_vdisk_name:status\n0:map0:vol0:vol0_copy:copied\n", "", 0
			case "svcinfo lssystemstats -delim :":
				return "stat_name:stat_current:stat_peak\ncpu_pc:15:35\nfc_io:2500:5000\n", "", 0
			case "svcinfo lsenclosurebattery -delim :":
				return "enclosure_id:battery_id:status:percent_charged\n1:1:online:100\n", "", 0
			case "svcinfo lsenclosurepsu -delim :":
				return "enclosure_id:psu_id:status\n1:1:online\n", "", 0
			default:
				return "", fmt.Sprintf("unrecognized command: %s", cmd), 1
			}
		}

		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		// Test System
		sysCol := NewSystemCollector(5 * time.Minute)
		sysRecs, err := sysCol.Collect(client)
		if err != nil {
			t.Fatalf("system collector failed: %v", err)
		}
		if len(sysRecs) != 1 || sysRecs[0]["name"] != "FS5045" || sysRecs[0]["code_level"] != "8.7.0.0 (build 123.456)" {
			t.Errorf("system records mismatch: %+v", sysRecs)
		}

		// Test Nodes
		nodeCol := NewNodesCollector(5 * time.Minute)
		nodeRecs, err := nodeCol.Collect(client)
		if err != nil {
			t.Fatalf("nodes collector failed: %v", err)
		}
		if len(nodeRecs) != 2 || nodeRecs[0]["name"] != "node1" {
			t.Errorf("nodes records mismatch: %+v", nodeRecs)
		}

		// Test Enclosures
		encCol := NewEnclosuresCollector(4 * time.Hour)
		encRecs, err := encCol.Collect(client)
		if err != nil {
			t.Fatalf("enclosures collector failed: %v", err)
		}
		if len(encRecs) != 1 || encRecs[0]["id"] != "1" {
			t.Errorf("enclosures records mismatch: %+v", encRecs)
		}

		// Test Drives
		drvCol := NewDrivesCollector(4 * time.Hour)
		drvRecs, err := drvCol.Collect(client)
		if err != nil {
			t.Fatalf("drives collector failed: %v", err)
		}
		if len(drvRecs) != 1 || drvRecs[0]["capacity"] != "894.3GB" {
			t.Errorf("drives records mismatch: %+v", drvRecs)
		}

		// Test Pools
		poolCol := NewPoolsCollector(5 * time.Minute)
		poolRecs, err := poolCol.Collect(client)
		if err != nil {
			t.Fatalf("pools collector failed: %v", err)
		}
		if len(poolRecs) != 1 || poolRecs[0]["name"] != "Pool0" {
			t.Errorf("pools records mismatch: %+v", poolRecs)
		}

		// Test Volumes
		volCol := NewVolumesCollector(5 * time.Minute)
		volRecs, err := volCol.Collect(client)
		if err != nil {
			t.Fatalf("volumes collector failed: %v", err)
		}
		if len(volRecs) != 1 || volRecs[0]["name"] != "vol0" {
			t.Errorf("volumes records mismatch: %+v", volRecs)
		}

		// Test Ports
		portCol := NewPortsCollector(4 * time.Hour)
		portRecs, err := portCol.Collect(client)
		if err != nil {
			t.Fatalf("ports collector failed: %v", err)
		}
		if len(portRecs) != 1 || portRecs[0]["speed"] != "16Gb" {
			t.Errorf("ports records mismatch: %+v", portRecs)
		}

		// Test FlashCopy
		fcCol := NewFlashCopyCollector(5 * time.Minute)
		fcRecs, err := fcCol.Collect(client)
		if err != nil {
			t.Fatalf("flashcopy collector failed: %v", err)
		}
		if len(fcRecs) != 1 || fcRecs[0]["name"] != "map0" {
			t.Errorf("flashcopy records mismatch: %+v", fcRecs)
		}

		// Test Performance
		perfCol := NewPerformanceCollector(60 * time.Second)
		perfRecs, err := perfCol.Collect(client)
		if err != nil {
			t.Fatalf("performance collector failed: %v", err)
		}
		if len(perfRecs) != 2 || perfRecs[0]["stat_name"] != "cpu_pc" {
			t.Errorf("performance records mismatch: %+v", perfRecs)
		}

		// Test Batteries
		batCol := NewBatteriesCollector(4 * time.Hour)
		batRecs, err := batCol.Collect(client)
		if err != nil {
			t.Fatalf("batteries collector failed: %v", err)
		}
		if len(batRecs) != 1 || batRecs[0]["percent_charged"] != "100" {
			t.Errorf("batteries records mismatch: %+v", batRecs)
		}

		// Test PSUs
		psuCol := NewPSUsCollector(4 * time.Hour)
		psuRecs, err := psuCol.Collect(client)
		if err != nil {
			t.Fatalf("psus collector failed: %v", err)
		}
		if len(psuRecs) != 1 || psuRecs[0]["psu_id"] != "1" {
			t.Errorf("psus records mismatch: %+v", psuRecs)
		}
	})

	t.Run("SystemCollectorEmptyResponse", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			return "", "", 0
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		sysCol := NewSystemCollector(5 * time.Minute)
		_, err := sysCol.Collect(client)
		if err == nil {
			t.Fatal("expected error for empty system response, got nil")
		}
		if !strings.Contains(err.Error(), "empty response from lssystem") {
			t.Errorf("expected 'empty response from lssystem' in error, got: %v", err)
		}
	})

	t.Run("PerformanceCollectorEmptyResponse", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			return "", "", 0
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		perfCol := NewPerformanceCollector(60 * time.Second)
		_, err := perfCol.Collect(client)
		if err == nil {
			t.Fatal("expected error for empty performance response, got nil")
		}
		if !strings.Contains(err.Error(), "empty response from lssystemstats") {
			t.Errorf("expected 'empty response from lssystemstats' in error, got: %v", err)
		}
	})

	t.Run("ReplicationCollectorModernSuccess", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			if cmd == "svcinfo lsreplicationrelationship -delim :" {
				return "id:name:state\n0:rel0:consistent_synchronized\n", "", 0
			}
			return "", "not found", 1
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		repCol := NewReplicationCollector(5 * time.Minute)
		recs, err := repCol.Collect(client)
		if err != nil {
			t.Fatalf("replication collector failed: %v", err)
		}
		if len(recs) != 1 || recs[0]["name"] != "rel0" {
			t.Errorf("replication records mismatch: %+v", recs)
		}
	})

	t.Run("ReplicationCollectorFallbackToLegacy", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			if cmd == "svcinfo lsreplicationrelationship -delim :" {
				// IBM Spectrum Virtualize error when command is unrecognized
				return "", "CMMVC5753E The specified command is not valid", 1
			}
			if cmd == "svcinfo lsrcrelationship -delim :" {
				return "id:name:state\n10:legacy_rel:consistent_copying\n", "", 0
			}
			return "", "unexpected command", 1
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		repCol := NewReplicationCollector(5 * time.Minute)
		recs, err := repCol.Collect(client)
		if err != nil {
			t.Fatalf("replication fallback to legacy failed: %v", err)
		}
		if len(recs) != 1 || recs[0]["name"] != "legacy_rel" {
			t.Errorf("replication legacy records mismatch: %+v", recs)
		}
	})

	t.Run("ReplicationCollectorBothNotFoundYieldsEmpty", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			return "", "CMMVC5753E command not recognized", 1
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		repCol := NewReplicationCollector(5 * time.Minute)
		recs, err := repCol.Collect(client)
		if err != nil {
			t.Fatalf("expected empty records without error, got err: %v", err)
		}
		if len(recs) != 0 {
			t.Errorf("expected 0 records, got %d", len(recs))
		}
	})

	t.Run("ReplicationCollectorNonCommandErrorFails", func(t *testing.T) {
		handler := func(cmd string) (string, string, uint32) {
			return "", "fatal IO error occurred", 1
		}
		client, cleanup := setupLoopbackSSHServer(t, handler)
		defer cleanup()

		repCol := NewReplicationCollector(5 * time.Minute)
		_, err := repCol.Collect(client)
		if err == nil {
			t.Fatal("expected error on non-command-not-found error, got nil")
		}
	})
}

func TestMarshalRecords(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		res := MarshalRecords(nil)
		if string(res) != "[]" {
			t.Errorf("expected '[]', got %q", string(res))
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		res := MarshalRecords([]parser.Record{})
		if string(res) != "[]" {
			t.Errorf("expected '[]', got %q", string(res))
		}
	})

	t.Run("populated slice", func(t *testing.T) {
		recs := []parser.Record{
			{"id": "1", "name": "vol1"},
		}
		res := MarshalRecords(recs)
		exp := `[{"id":"1","name":"vol1"}]`
		if string(res) != exp {
			t.Errorf("expected %q, got %q", exp, string(res))
		}
	})
}

func TestDrivesCapacityFormatting(t *testing.T) {
	col := NewDrivesCollector(4 * time.Hour)
	if col.Name() != "drives" {
		t.Errorf("expected name 'drives', got %s", col.Name())
	}
	if col.TTL() != 4*time.Hour {
		t.Errorf("expected TTL 4h, got %v", col.TTL())
	}

	// Verify that Drive capacity formatting correctly processes raw drive lines
	rawOutput := "id:status:error_sequence_number:use:tech_type:capacity:enclosure_id:slot_id\n" +
		"0:online:0:member:flash:960247313203:1:1\n" +
		"1:online:0:member:flash:2089072092774:1:2\n" +
		"2:online:0:member:flash:894.3GB:1:3\n" +
		"3:online:0:member:flash:1.9TB:1:4\n"

	res := parser.Parse(rawOutput)
	for _, rec := range res.Records {
		if capVal, ok := rec["capacity"]; ok && capVal != "" {
			rec["capacity"] = parser.FormatCapacity(capVal)
		}
	}

	expected := []string{"894.3GB", "1.9TB", "894.3GB", "1.9TB"}
	for i, exp := range expected {
		if res.Records[i]["capacity"] != exp {
			t.Errorf("drive record %d: expected capacity %s, got %s", i, exp, res.Records[i]["capacity"])
		}
	}
}
