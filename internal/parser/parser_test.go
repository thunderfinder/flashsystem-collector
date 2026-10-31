package parser

import (
	"testing"
)

func TestParseVertical(t *testing.T) {
	output := `
id:0
name:FlashSystem_5045
location:local
code_level:8.7.0.0 (build 162.24.2310201505)
status:online
time_zone:230 America/New_York
`
	res := Parse(output)
	if res.Format != FormatVertical {
		t.Fatalf("expected FormatVertical, got %s", res.Format)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(res.Records))
	}

	rec := res.Records[0]
	if rec["id"] != "0" {
		t.Errorf("expected id '0', got %q", rec["id"])
	}
	if rec["name"] != "FlashSystem_5045" {
		t.Errorf("expected name 'FlashSystem_5045', got %q", rec["name"])
	}
	// Verify preservation of colons in value
	if rec["code_level"] != "8.7.0.0 (build 162.24.2310201505)" {
		t.Errorf("expected code_level with colon preserved, got %q", rec["code_level"])
	}
	if rec["time_zone"] != "230 America/New_York" {
		t.Errorf("expected time_zone, got %q", rec["time_zone"])
	}
}

func TestParseVerticalForced(t *testing.T) {
	output := "single_key:single_val\n"
	res := ParseVerticalForced(output)
	if res.Format != FormatVertical {
		t.Fatalf("expected FormatVertical, got %s", res.Format)
	}
	if len(res.Records) != 1 || res.Records[0]["single_key"] != "single_val" {
		t.Fatalf("unexpected record: %+v", res.Records)
	}

	emptyRes := ParseVerticalForced("")
	if emptyRes.Format != FormatEmpty || len(emptyRes.Records) != 0 {
		t.Fatalf("expected FormatEmpty for forced empty, got %+v", emptyRes)
	}
}

func TestParseTabular(t *testing.T) {
	output := `id:enclosure_id:slot_id:status:capacity
0:1:1:online:1.2TB
1:1:2:degraded:1.2TB
2:1:3:offline:1.2TB
`
	res := Parse(output)
	if res.Format != FormatTabular {
		t.Fatalf("expected FormatTabular, got %s", res.Format)
	}
	if len(res.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(res.Records))
	}

	r0 := res.Records[0]
	if r0["id"] != "0" || r0["status"] != "online" || r0["capacity"] != "1.2TB" {
		t.Errorf("unexpected record 0: %+v", r0)
	}

	r1 := res.Records[1]
	if r1["id"] != "1" || r1["status"] != "degraded" {
		t.Errorf("unexpected record 1: %+v", r1)
	}
}

func TestParseTabularWithColonsInLastField(t *testing.T) {
	output := `stat_name:stat_current:stat_peak:stat_peak_time
read_io:912:2634:231017155500
test_time:10:20:2026-03-17 12:00:00
`
	res := Parse(output)
	if res.Format != FormatTabular {
		t.Fatalf("expected FormatTabular, got %s", res.Format)
	}
	if len(res.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res.Records))
	}

	r1 := res.Records[1]
	// Last field should preserve extra colons
	if r1["stat_peak_time"] != "2026-03-17 12:00:00" {
		t.Errorf("expected last field with colons preserved, got %q", r1["stat_peak_time"])
	}
}

func TestParseEmptyAndNoResults(t *testing.T) {
	emptyCases := []string{
		"",
		"   \n\r\n  ",
		"No results",
		"no result",
		"no objects found",
		"CMMVC5845E The specified object does not exist",
		"CMMVC5753E The command was not recognized",
	}

	for _, c := range emptyCases {
		res := Parse(c)
		if res.Format != FormatEmpty {
			t.Errorf("expected FormatEmpty for %q, got %s", c, res.Format)
		}
		if len(res.Records) != 0 {
			t.Errorf("expected 0 records for %q, got %d", c, len(res.Records))
		}
	}
}

func TestGetField(t *testing.T) {
	rec := Record{
		"status": "online",
		"empty":  "   ",
	}

	if val := GetField(rec, "status", "unknown"); val != "online" {
		t.Errorf("expected online, got %q", val)
	}
	if val := GetField(rec, "empty", "default"); val != "default" {
		t.Errorf("expected default for whitespace value, got %q", val)
	}
	if val := GetField(rec, "non_existent", "fallback"); val != "fallback" {
		t.Errorf("expected fallback for missing key, got %q", val)
	}
	if val := GetFieldLower(rec, "status", "UNKNOWN"); val != "online" {
		t.Errorf("expected lowercase online, got %q", val)
	}
}

func TestParseLineEndings(t *testing.T) {
	// CRLF line endings from remote Windows/IBM CLI with multi-column tabular
	output := "id:name:status\r\n1:node1:online\r\n2:node2:online\r\n"
	res := Parse(output)
	if res.Format != FormatTabular {
		t.Fatalf("expected FormatTabular, got %s", res.Format)
	}
	if len(res.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res.Records))
	}
	if res.Records[0]["name"] != "node1" || res.Records[1]["name"] != "node2" {
		t.Errorf("unexpected records: %+v", res.Records)
	}
}
