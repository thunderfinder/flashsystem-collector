package parser

import (
	"strings"
	"testing"
)

const sampleLssystem = `id:0
name:FlashSystem_5045
location:local
partnership:
total_mdisk_capacity:44.0TB
space_in_mdisk_grps:44.0TB
space_allocated_to_vdisks:12.3TB
total_free_space:31.7TB
total_vdiskcopy_capacity:12.3TB
total_used_capacity:10.1TB
total_overallocation:28
total_vdisk_capacity:12.3TB
total_allocated_extent_capacity:12.3TB
statistics_status:on
statistics_frequency:5
cluster_locale:en_US
time_zone:230 America/New_York
code_level:8.7.0.0 (build 162.24.2310201505)
console_IP:
id_alias:0000000000000000
gm_inter_cluster_delay_simulation:0
gm_intra_cluster_delay_simulation:0
gm_max_host_delay:5
email_state:stopped
inventory_mail_interval:0
cluster_ntp_IP_address:
cluster_isns_IP_address:
iscsi_auth_method:none
iscsi_chap_secret:
auth_service_configured:no
auth_service_enabled:no
auth_service_url:
auth_service_user_name:
auth_service_pwd_set:no
auth_service_cert_set:no
auth_service_type:tip
relationship_bandwidth_limit:25
tiers:tier0_flash:tier1_flash:tier_enterprise:tier_nearline
tiers_label:Generic Flash:Flash:Enterprise:Nearline
has_nas_key:no
layer:storage
rc_buffer_size:256
compression_active:no
compression_virtual_capacity:0.00MB
compression_compressed_capacity:0.00MB
compression_uncompressed_capacity:0.00MB
cache_prefetch:on
email_company_name:
email_machine_address:
email_reply_address:
email_contact:
email_contact_primary:
email_contact_alternate:
email_contact_location:
email_contact_phone:
email_contact_phone_alternate:
email_contact_notes:
email_proxy_server:
email_proxy_port:8080
email_proxy_user_name:
email_proxy_pwd_set:no
total_drive_raw_capacity:44.0TB
number_of_nodes:2
drive_based_encryption:none
status:online
`

const sampleLsdrive = `id:status:error_sequence_number:use:tech_type:capacity:enclosure_id:slot_id:auto_manage:drive_class_id
0:online:0:member:flash:894.3GB:1:1:inactive:0
1:online:0:member:flash:894.3GB:1:2:inactive:0
2:online:0:member:flash:894.3GB:1:3:inactive:0
3:online:0:member:flash:894.3GB:1:4:inactive:0
4:online:0:member:flash:894.3GB:1:5:inactive:0
5:online:0:member:flash:894.3GB:1:6:inactive:0
6:online:0:member:flash:894.3GB:1:7:inactive:0
7:online:0:member:flash:894.3GB:1:8:inactive:0
8:online:0:member:flash:894.3GB:1:9:inactive:0
9:online:0:member:flash:894.3GB:1:10:inactive:0
10:online:0:member:flash:894.3GB:1:11:inactive:0
11:online:0:member:flash:894.3GB:1:12:inactive:0
12:online:0:member:flash:894.3GB:1:13:inactive:0
13:online:0:member:flash:894.3GB:1:14:inactive:0
14:online:0:member:flash:894.3GB:1:15:inactive:0
15:online:0:member:flash:894.3GB:1:16:inactive:0
16:online:0:member:flash:894.3GB:1:17:inactive:0
17:online:0:member:flash:894.3GB:1:18:inactive:0
18:online:0:member:flash:894.3GB:1:19:inactive:0
19:online:0:member:flash:894.3GB:1:20:inactive:0
20:online:0:member:flash:894.3GB:1:21:inactive:0
21:online:0:member:flash:894.3GB:1:22:inactive:0
22:online:0:member:flash:894.3GB:1:23:inactive:0
23:online:0:member:flash:894.3GB:1:24:inactive:0
`

const sampleLssystemstats = `stat_name:stat_current:stat_peak:stat_peak_time
compression_cpu_pc:0:0:231017191500
cpu_pc:3:8:231017161500
fc_mb:42:156:231017155500
fc_io:1823:4821:231017155500
sas_mb:0:0:231017191500
sas_io:0:0:231017191500
iscsi_mb:0:0:231017191500
iscsi_io:0:0:231017191500
read_mb:21:98:231017155500
write_mb:21:58:231017155500
read_io:912:2634:231017155500
write_io:911:2187:231017155500
read_ms:0:2:231017155500
write_ms:0:1:231017155500
drive_r_mb:21:98:231017155500
drive_w_mb:21:58:231017155500
drive_r_io:912:2634:231017155500
drive_w_io:911:2187:231017155500
drive_ms:0:2:231017155500
vdisk_mb:42:156:231017155500
vdisk_io:1823:4821:231017155500
vdisk_ms:0:1:231017155500
mdisk_mb:42:156:231017155500
mdisk_io:1823:4821:231017155500
mdisk_ms:0:2:231017155500
`

func generateTabularData(rows int) string {
	var sb strings.Builder
	sb.WriteString("id:name:IO_group_id:IO_group_name:status:mdisk_grp_id:mdisk_grp_name:capacity:type:formatted:mdisk_id:mdisk_name:FC_id:FC_name:RC_id:RC_name:vdisk_UID:fc_map_count:copy_count:fast_write_state:se_copy_count:RC_change:compressed_copy_count\n")
	for i := 0; i < rows; i++ {
		sb.WriteString("0:vol_prod_001:0:io_grp0:online:0:Pool0:100.00GB:striped:yes:::none::none::60050768018105AA0000000000000001:0:1:empty:0:no:0\n")
	}
	return sb.String()
}

func TestParseVertical(t *testing.T) {
	res := Parse(sampleLssystem)
	if res.Format != FormatVertical {
		t.Fatalf("expected FormatVertical, got %v", res.Format)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(res.Records))
	}
	rec := res.Records[0]
	if rec["name"] != "FlashSystem_5045" {
		t.Errorf("expected name FlashSystem_5045, got %q", rec["name"])
	}
	if rec["code_level"] != "8.7.0.0 (build 162.24.2310201505)" {
		t.Errorf("expected code_level with colon preserved, got %q", rec["code_level"])
	}
	if rec["status"] != "online" {
		t.Errorf("expected status online, got %q", rec["status"])
	}
}

func TestParseTabular(t *testing.T) {
	res := Parse(sampleLsdrive)
	if res.Format != FormatTabular {
		t.Fatalf("expected FormatTabular, got %v", res.Format)
	}
	if len(res.Records) != 24 {
		t.Fatalf("expected 24 records, got %d", len(res.Records))
	}
	first := res.Records[0]
	if first["id"] != "0" || first["status"] != "online" || first["slot_id"] != "1" {
		t.Errorf("unexpected first record: %#v", first)
	}
	last := res.Records[23]
	if last["id"] != "23" || last["slot_id"] != "24" {
		t.Errorf("unexpected last record: %#v", last)
	}
}

func TestParseEmptyAndErrors(t *testing.T) {
	testCases := []struct {
		input  string
		format Format
	}{
		{"", FormatEmpty},
		{"\r\n  \n", FormatEmpty},
		{"CMMVC5845E The specified object does not exist", FormatEmpty},
		{"No results", FormatEmpty},
		{"no objects found\n", FormatEmpty},
	}

	for _, tc := range testCases {
		res := Parse(tc.input)
		if res.Format != tc.format {
			t.Errorf("for input %q expected format %v, got %v", tc.input, tc.format, res.Format)
		}
		if len(res.Records) != 0 {
			t.Errorf("for input %q expected empty records, got %d", tc.input, len(res.Records))
		}
	}
}

func BenchmarkIsNoResults(b *testing.B) {
	line := "0:vol_prod_001:0:io_grp0:online:0:Pool0:100.00GB:striped:yes:::none::none::60050768018105AA0000000000000001:0:1:empty:0:no:0"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if isNoResults(line) {
			b.Fatal("unexpected true")
		}
	}
}

func TestParseVerticalForced(t *testing.T) {
	res := ParseVerticalForced(sampleLssystem)
	if res.Format != FormatVertical {
		t.Fatalf("expected FormatVertical, got %v", res.Format)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(res.Records))
	}
	if res.Records[0]["name"] != "FlashSystem_5045" {
		t.Errorf("expected name FlashSystem_5045, got %q", res.Records[0]["name"])
	}
}

func TestHelpers(t *testing.T) {
	r := Record{"status": "Online", "empty": "  "}
	if val := GetField(r, "status", "unknown"); val != "Online" {
		t.Errorf("expected Online, got %s", val)
	}
	if val := GetField(r, "empty", "default"); val != "default" {
		t.Errorf("expected default, got %s", val)
	}
	if val := GetField(r, "nonexistent", "default"); val != "default" {
		t.Errorf("expected default, got %s", val)
	}
	if val := GetFieldLower(r, "status", "unknown"); val != "online" {
		t.Errorf("expected online, got %s", val)
	}
}

func TestFormatCapacity(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"invalid", "invalid"},
		// Under 1 TB: display in GB
		{"894.3GB", "894.3GB"},
		{"800GB", "800GB"},
		{"500MB", "0.5GB"},
		{"1073741824", "1GB"},            // 1 GiB in bytes
		{"960247313203", "894.3GB"},       // 894.3 GiB in bytes
		{"536870912000", "500GB"},         // 500 GiB in bytes
		// 1 TB or greater: display in TB
		{"1099511627776", "1TB"},          // 1 TiB in bytes
		{"1024GB", "1TB"},
		{"1.9TB", "1.9TB"},
		{"2089072092774", "1.9TB"},        // 1.9 TiB in bytes
		{"10TB", "10TB"},
		{"1900GB", "1.9TB"},
		{"2.4TB", "2.4TB"},
	}

	for _, tt := range tests {
		actual := FormatCapacity(tt.input)
		if actual != tt.expected {
			t.Errorf("FormatCapacity(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func BenchmarkParseVertical(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := Parse(sampleLssystem)
		if len(res.Records) != 1 {
			b.Fatal("unexpected record count")
		}
	}
}

func BenchmarkParseTabularDrives(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := Parse(sampleLsdrive)
		if len(res.Records) != 24 {
			b.Fatal("unexpected record count")
		}
	}
}

func BenchmarkParseTabularStats(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := Parse(sampleLssystemstats)
		if len(res.Records) != 25 {
			b.Fatal("unexpected record count")
		}
	}
}

func BenchmarkParseTabularLargeVolumes(b *testing.B) {
	data := generateTabularData(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := Parse(data)
		if len(res.Records) != 500 {
			b.Fatal("unexpected record count")
		}
	}
}
