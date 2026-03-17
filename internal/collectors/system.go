package collectors

import (
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// SystemCollector recolecta información general del cluster FlashSystem.
type SystemCollector struct {
	ttl time.Duration
}

// NewSystemCollector crea un SystemCollector con el TTL especificado.
func NewSystemCollector(ttl time.Duration) *SystemCollector {
	return &SystemCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *SystemCollector) Name() string {
	return "system"
}

// TTL implementa Collector.
func (c *SystemCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta "svcinfo lssystem -delim :" y parsea el resultado.
//
// Formato real de salida (vertical):
//
//	id:0
//	name:FlashSystem_5045
//	location:local
//	partnership:
//	total_mdisk_capacity:44.0TB
//	space_in_mdisk_grps:44.0TB
//	space_allocated_to_vdisks:12.3TB
//	total_free_space:31.7TB
//	total_vdiskcopy_capacity:12.3TB
//	total_used_capacity:10.1TB
//	total_overallocation:28
//	total_vdisk_capacity:12.3TB
//	total_allocated_extent_capacity:12.3TB
//	statistics_status:on
//	statistics_frequency:5
//	cluster_locale:en_US
//	time_zone:230 America/New_York
//	code_level:8.7.0.0 (build 162.24.2310201505)
//	console_IP:
//	id_alias:0000000000000000
//	gm_inter_cluster_delay_simulation:0
//	gm_intra_cluster_delay_simulation:0
//	gm_max_host_delay:5
//	email_state:stopped
//	inventory_mail_interval:0
//	cluster_ntp_IP_address:
//	cluster_isns_IP_address:
//	iscsi_auth_method:none
//	iscsi_chap_secret:
//	auth_service_configured:no
//	auth_service_enabled:no
//	auth_service_url:
//	auth_service_user_name:
//	auth_service_pwd_set:no
//	auth_service_cert_set:no
//	auth_service_type:tip
//	relationship_bandwidth_limit:25
//	tiers:tier0_flash:tier1_flash:tier_enterprise:tier_nearline
//	tiers_label:Generic Flash:Flash:Enterprise:Nearline
//	has_nas_key:no
//	layer:storage
//	rc_buffer_size:256
//	compression_active:no
//	compression_virtual_capacity:0.00MB
//	compression_compressed_capacity:0.00MB
//	compression_uncompressed_capacity:0.00MB
//	cache_prefetch:on
//	email_company_name:
//	email_machine_address:
//	email_reply_address:
//	email_contact:
//	email_contact_primary:
//	email_contact_alternate:
//	email_contact_location:
//	email_contact_phone:
//	email_contact_phone_alternate:
//	email_contact_notes:
//	email_proxy_server:
//	email_proxy_port:8080
//	email_proxy_user_name:
//	email_proxy_pwd_set:no
//	total_drive_raw_capacity:44.0TB
//	number_of_nodes:2
//	total_allocated_extent_capacity:12.3TB
//	drive_based_encryption:none
//	status:online
//
// Implementa Collector.
func (c *SystemCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {
	output, err := client.Run("svcinfo lssystem -delim :")
	if err != nil {
		return nil, fmt.Errorf("system collector: %w", err)
	}

	result := parser.ParseVerticalForced(output)

	if result.Format == parser.FormatEmpty {
		// lssystem siempre retorna datos si el sistema está online.
		// Un output vacío indica un problema de conectividad o permisos.
		return nil, fmt.Errorf("system collector: empty response from lssystem (check SSH permissions)")
	}

	return result.Records, nil
}
