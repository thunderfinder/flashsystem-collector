package parser

import (
	"reflect"
	"testing"
)

func TestParseVertical(t *testing.T) {
	input := `id:0
name:FlashSystem_5045
location:local
code_level:8.7.0.0 (build 123.456)
`
	res := Parse(input)
	if res.Format != FormatVertical {
		t.Fatalf("expected FormatVertical, got %v", res.Format)
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
	if rec["code_level"] != "8.7.0.0 (build 123.456)" {
		t.Errorf("expected colon preserved in code_level, got %q", rec["code_level"])
	}
}

func TestParseTabular(t *testing.T) {
	input := `id:enclosure_id:slot_id:status:capacity
0:1:1:online:1.2TB
1:1:2:online:1.2TB
`
	res := Parse(input)
	if res.Format != FormatTabular {
		t.Fatalf("expected FormatTabular, got %v", res.Format)
	}
	if len(res.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res.Records))
	}
	expectedFirst := Record{
		"id":           "0",
		"enclosure_id": "1",
		"slot_id":      "1",
		"status":       "online",
		"capacity":     "1.2TB",
	}
	if !reflect.DeepEqual(res.Records[0], expectedFirst) {
		t.Errorf("expected record 0 to match %v, got %v", expectedFirst, res.Records[0])
	}
}

func TestParseEmptyAndNoResults(t *testing.T) {
	tests := []string{
		"",
		"   \n  \n",
		"CMMVC5753E The specified object does not exist",
		"No objects found",
	}

	for _, input := range tests {
		res := Parse(input)
		if len(res.Records) != 0 {
			t.Errorf("expected 0 records for input %q, got %d", input, len(res.Records))
		}
	}
}

func TestGetField(t *testing.T) {
	rec := Record{"status": " online "}
	if got := GetField(rec, "status", "unknown"); got != "online" {
		t.Errorf("expected 'online', got %q", got)
	}
	if got := GetField(rec, "missing", "default"); got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
	if got := GetFieldLower(rec, "status", "unknown"); got != "online" {
		t.Errorf("expected 'online', got %q", got)
	}
}
