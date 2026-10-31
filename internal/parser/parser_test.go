package parser

import (
	"reflect"
	"testing"
)

func TestFormatString(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatVertical, "vertical"},
		{FormatTabular, "tabular"},
		{FormatEmpty, "empty"},
		{FormatUnknown, "unknown"},
		{Format(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.format.String(); got != tt.want {
			t.Errorf("Format(%d).String() = %q; want %q", tt.format, got, tt.want)
		}
	}
}

func TestParseEmptyAndErrorOutputs(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFormat Format
	}{
		{"empty string", "", FormatEmpty},
		{"only whitespace", "   \t\n   ", FormatEmpty},
		{"carriage returns only", "\r\r\n", FormatEmpty},
		{"no results standard", "no results", FormatEmpty},
		{"no result singular", "no result", FormatEmpty},
		{"no objects found", "no objects found", FormatEmpty},
		{"ibm error code CMMVC5845E", "CMMVC5845E", FormatEmpty},
		{"ibm error code prefix lowercase", "cmmvc5753e The command failed", FormatEmpty},
		{"ibm error code prefix uppercase with space", "CMMVC6017E Command cannot be initiated", FormatEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Parse(tt.input)
			if res.Format != tt.wantFormat {
				t.Errorf("Parse(%q).Format = %v; want %v", tt.input, res.Format, tt.wantFormat)
			}
			if len(res.Records) != 0 {
				t.Errorf("Parse(%q).Records len = %d; want 0", tt.input, len(res.Records))
			}
			if res.Records == nil {
				t.Errorf("Parse(%q).Records is nil; want empty slice", tt.input)
			}
		})
	}
}

func TestParseVertical(t *testing.T) {
	input := `id:0
name:FlashSystem_5045
location:local
code_level:8.7.0.0 (build 123.456)
console_IP:192.168.1.10:8443
empty_val:
:missing_key
`

	res := Parse(input)

	if res.Format != FormatVertical {
		t.Fatalf("Parse().Format = %v; want %v", res.Format, FormatVertical)
	}
	if len(res.Records) != 1 {
		t.Fatalf("Parse().Records len = %d; want 1", len(res.Records))
	}

	rec := res.Records[0]
	expected := map[string]string{
		"id":         "0",
		"name":       "FlashSystem_5045",
		"location":   "local",
		"code_level": "8.7.0.0 (build 123.456)",
		"console_IP": "192.168.1.10:8443",
		"empty_val":  "",
	}

	for k, wantVal := range expected {
		if gotVal := rec[k]; gotVal != wantVal {
			t.Errorf("Record[%q] = %q; want %q", k, gotVal, wantVal)
		}
	}

	if _, exists := rec[""]; exists {
		t.Errorf("Record contains empty key from line with missing key")
	}
}

func TestParseVerticalForced(t *testing.T) {
	t.Run("valid input", func(t *testing.T) {
		input := "key1:val1\r\nkey2:val2:with:colons\n"
		res := ParseVerticalForced(input)
		if res.Format != FormatVertical {
			t.Errorf("ParseVerticalForced().Format = %v; want %v", res.Format, FormatVertical)
		}
		if len(res.Records) != 1 {
			t.Fatalf("ParseVerticalForced().Records len = %d; want 1", len(res.Records))
		}
		if got := res.Records[0]["key2"]; got != "val2:with:colons" {
			t.Errorf("key2 = %q; want %q", got, "val2:with:colons")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		res := ParseVerticalForced("")
		if res.Format != FormatEmpty || len(res.Records) != 0 {
			t.Errorf("ParseVerticalForced(\"\") = format %v, len %d; want FormatEmpty, len 0", res.Format, len(res.Records))
		}
	})

	t.Run("no results", func(t *testing.T) {
		res := ParseVerticalForced("CMMVC5845E")
		if res.Format != FormatEmpty || len(res.Records) != 0 {
			t.Errorf("ParseVerticalForced error string = format %v, len %d; want FormatEmpty, len 0", res.Format, len(res.Records))
		}
	})
}

func TestParseTabular(t *testing.T) {
	input := `id:enclosure_id:slot_id:status:capacity:notes
0:1:1:online:1.2TB:primary:fast
1:1:2:degraded:1.2TB
`

	res := Parse(input)

	if res.Format != FormatTabular {
		t.Fatalf("Parse().Format = %v; want %v", res.Format, FormatTabular)
	}
	if len(res.Records) != 2 {
		t.Fatalf("Parse().Records len = %d; want 2", len(res.Records))
	}

	// First row has extra colon in last field, should preserve it in 'notes'
	rec0 := res.Records[0]
	if rec0["id"] != "0" || rec0["status"] != "online" || rec0["notes"] != "primary:fast" {
		t.Errorf("Record[0] mismatch: got %+v", rec0)
	}

	// Second row has fewer values than headers, missing 'notes' should be empty string
	rec1 := res.Records[1]
	if rec1["id"] != "1" || rec1["status"] != "degraded" || rec1["notes"] != "" {
		t.Errorf("Record[1] mismatch: got %+v", rec1)
	}
}

func TestParseTabularEdgeCases(t *testing.T) {
	t.Run("header only", func(t *testing.T) {
		input := "id:name:status"
		res := Parse(input)
		if len(res.Records) != 0 {
			t.Errorf("Parse(header only).Records len = %d; want 0", len(res.Records))
		}
	})

	t.Run("header with blank and no results lines", func(t *testing.T) {
		input := "id:name:status\n\nno results\n\n"
		res := Parse(input)
		if len(res.Records) != 0 {
			t.Errorf("Parse(header with no results).Records len = %d; want 0", len(res.Records))
		}
	})

	t.Run("line without delimiters fallback", func(t *testing.T) {
		input := "plain_single_line_without_colons"
		res := Parse(input)
		if res.Format != FormatTabular {
			t.Errorf("Format = %v; want %v", res.Format, FormatTabular)
		}
	})
}

func TestGetField(t *testing.T) {
	rec := Record{
		"name":  " FlashSystem-01 ",
		"empty": "   ",
	}

	tests := []struct {
		name       string
		field      string
		defaultVal string
		want       string
	}{
		{"existing field trimmed", "name", "def", "FlashSystem-01"},
		{"whitespace field uses default", "empty", "def", "def"},
		{"non-existent field uses default", "missing", "def", "def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetField(rec, tt.field, tt.defaultVal); got != tt.want {
				t.Errorf("GetField(%q, %q) = %q; want %q", tt.field, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestGetFieldLower(t *testing.T) {
	rec := Record{
		"status": "ONLINE",
	}

	if got := GetFieldLower(rec, "status", "unknown"); got != "online" {
		t.Errorf("GetFieldLower(\"status\") = %q; want %q", got, "online")
	}

	if got := GetFieldLower(rec, "missing", "DEFAULT"); got != "default" {
		t.Errorf("GetFieldLower(\"missing\") = %q; want %q", got, "default")
	}
}

func TestDetectFormatSpecial(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		wantFormat Format
	}{
		{"empty slice", []string{}, FormatEmpty},
		{"single line single colon", []string{"id:0"}, FormatVertical},
		{"multi line single colon", []string{"id:0", "name:fs"}, FormatVertical},
		{"first line single colon second line multi colon", []string{"id:0", "a:b:c"}, FormatVertical},
		{"first line multiple colons", []string{"id:name:status", "0:node1:online"}, FormatTabular},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectFormat(tt.lines); got != tt.wantFormat {
				t.Errorf("detectFormat() = %v; want %v", got, tt.wantFormat)
			}
		})
	}
}

func TestSplitTabularLine(t *testing.T) {
	line := " id : name :: status : "
	got := splitTabularLine(line)
	want := []string{"id", "name", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitTabularLine() = %v; want %v", got, want)
	}
}
