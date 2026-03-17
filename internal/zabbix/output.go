package zabbix

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/flashsystem-collector/internal/cache"
	"github.com/flashsystem-collector/internal/collectors"
	"github.com/flashsystem-collector/internal/parser"
)

// Output es la estructura raíz del JSON que Zabbix consume.
// Diseñada para usarse como Master Item con Dependent Items y LLD rules.
type Output struct {
	Timestamp   string           `json:"timestamp"`
	Host        string           `json:"host"`
	Version     string           `json:"version"`
	System      json.RawMessage  `json:"system"`
	Nodes       json.RawMessage  `json:"nodes"`
	Enclosures  json.RawMessage  `json:"enclosures"`
	Drives      json.RawMessage  `json:"drives"`
	Pools       json.RawMessage  `json:"pools"`
	Volumes     json.RawMessage  `json:"volumes"`
	Ports       json.RawMessage  `json:"ports"`
	FlashCopy   json.RawMessage  `json:"flashcopy"`
	Replication json.RawMessage  `json:"replication"`
	Performance json.RawMessage  `json:"performance"`
	Status      CollectionStatus `json:"status"`
	CacheStats  cache.Stats      `json:"cache_stats"`
}

// CollectionStatus contiene el resumen de la ejecución del collector.
type CollectionStatus struct {
	Success       bool           `json:"success"`
	DurationMs    int64          `json:"duration_ms"`
	CollectorRuns []CollectorRun `json:"collector_runs"`
	Errors        []string       `json:"errors,omitempty"`
	TruncatedAt   map[string]int `json:"truncated_at,omitempty"`
}

// CollectorRun contiene el estado de ejecución de un collector individual.
type CollectorRun struct {
	Name        string     `json:"name"`
	RecordCount int        `json:"record_count"`
	CacheHit    bool       `json:"cache_hit"`
	DurationMs  int64      `json:"duration_ms"`
	Error       string     `json:"error,omitempty"`
	CachedAt    *time.Time `json:"cached_at,omitempty"`
}

// BuilderConfig contiene los parámetros para construir el Output.
type BuilderConfig struct {
	Host         string
	Version      string
	MaxJSONBytes int
	MaxVolumes   int
	MaxDrives    int
}

// Build construye el Output final a partir de los resultados de los collectors.
// Aplica límites de tamaño. Retorna el Output y un error solo si la
// serialización JSON falla completamente (situación extremadamente rara).
func Build(
	cfg BuilderConfig,
	results map[string]collectors.Result,
	cacheStats cache.Stats,
	startTime time.Time,
) (*Output, error) {

	out := &Output{
		Timestamp:  startTime.UTC().Format(time.RFC3339),
		Host:       cfg.Host,
		Version:    cfg.Version,
		CacheStats: cacheStats,
		Status: CollectionStatus{
			Success:       true,
			CollectorRuns: make([]CollectorRun, 0, len(results)),
			TruncatedAt:   make(map[string]int),
		},
	}

	// --- Poblar cada sección del output ---
	sectionErrors := []string{}

	out.System = marshalSection(results, "system", &sectionErrors)
	out.Nodes = marshalSection(results, "nodes", &sectionErrors)
	out.Enclosures = marshalSection(results, "enclosures", &sectionErrors)
	out.Pools = marshalSection(results, "pools", &sectionErrors)
	out.FlashCopy = marshalSection(results, "flashcopy", &sectionErrors)
	out.Replication = marshalSection(results, "replication", &sectionErrors)
	out.Performance = marshalSection(results, "performance", &sectionErrors)
	out.Ports = marshalSection(results, "ports", &sectionErrors)

	// Volumes y Drives tienen límite de registros configurable.
	out.Volumes = marshalSectionLimited(
		results, "volumes", cfg.MaxVolumes,
		out.Status.TruncatedAt, &sectionErrors,
	)
	out.Drives = marshalSectionLimited(
		results, "drives", cfg.MaxDrives,
		out.Status.TruncatedAt, &sectionErrors,
	)

	// --- Construir CollectorRuns para diagnóstico ---
	// Orden determinístico para facilitar comparación entre ejecuciones.
	collectorOrder := []string{
		"system", "nodes", "enclosures", "drives",
		"pools", "volumes", "ports", "flashcopy",
		"replication", "performance",
	}

	for _, name := range collectorOrder {
		res, ok := results[name]
		if !ok {
			continue
		}
		run := CollectorRun{
			Name:        res.Name,
			RecordCount: len(res.Records),
			CacheHit:    res.CacheHit,
			DurationMs:  res.DurationMs,
			CachedAt:    res.CachedAt,
		}
		if res.Error != "" {
			run.Error = res.Error
			sectionErrors = append(sectionErrors, fmt.Sprintf("%s: %s", name, res.Error))
		}
		out.Status.CollectorRuns = append(out.Status.CollectorRuns, run)
	}

	if len(sectionErrors) > 0 {
		out.Status.Errors = sectionErrors
		// No marcamos Success=false por errores parciales.
		// Zabbix puede monitorear Status.Errors para alertar.
		// Solo marcamos false si TODOS los collectors fallaron.
		allFailed := true
		for _, res := range results {
			if res.Error == "" {
				allFailed = false
				break
			}
		}
		if allFailed {
			out.Status.Success = false
		}
	}

	out.Status.DurationMs = time.Since(startTime).Milliseconds()

	// Limpiar TruncatedAt si no hubo truncaciones.
	if len(out.Status.TruncatedAt) == 0 {
		out.Status.TruncatedAt = nil
	}

	// --- Validar tamaño del JSON ---
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("zabbix output: cannot serialize final JSON: %w", err)
	}

	if len(data) > cfg.MaxJSONBytes {
		// El output supera el límite. Intentar reducir truncando más agresivamente.
		out, err = reduceOutput(out, cfg, startTime)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Serialize convierte el Output a JSON indentado listo para stdout.
func Serialize(out *Output) ([]byte, error) {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("zabbix output: cannot serialize to JSON: %w", err)
	}
	return data, nil
}

// marshalSection extrae los Records de un resultado y los serializa a JSON raw.
// Si el resultado no existe o tiene error, retorna "[]".
// Los errores de serialización se acumulan en errors (nunca son fatales).
func marshalSection(
	results map[string]collectors.Result,
	name string,
	errors *[]string,
) json.RawMessage {
	res, ok := results[name]
	if !ok {
		return json.RawMessage("[]")
	}

	records := res.Records
	if records == nil {
		records = []parser.Record{}
	}

	data, err := json.Marshal(records)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("marshal %s: %v", name, err))
		return json.RawMessage("[]")
	}

	return json.RawMessage(data)
}

// marshalSectionLimited serializa Records aplicando un límite máximo de entradas.
// Si se trunca, registra la cantidad original en truncatedAt.
func marshalSectionLimited(
	results map[string]collectors.Result,
	name string,
	maxRecords int,
	truncatedAt map[string]int,
	errors *[]string,
) json.RawMessage {
	res, ok := results[name]
	if !ok {
		return json.RawMessage("[]")
	}

	records := res.Records
	if records == nil {
		records = []parser.Record{}
	}

	if maxRecords > 0 && len(records) > maxRecords {
		truncatedAt[name] = len(records)
		records = records[:maxRecords]
	}

	data, err := json.Marshal(records)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("marshal %s: %v", name, err))
		return json.RawMessage("[]")
	}

	return json.RawMessage(data)
}

// reduceOutput intenta reducir el tamaño del output truncando secciones grandes.
// Estrategia progresiva: primero reduce volumes/drives a la mitad,
// luego a un cuarto, luego vacía las secciones opcionales.
func reduceOutput(
	out *Output,
	cfg BuilderConfig,
	startTime time.Time,
) (*Output, error) {

	// Estrategias de reducción en orden de agresividad.
	reductions := []struct {
		maxVolumes int
		maxDrives  int
		clearFC    bool
		clearRepl  bool
	}{
		{cfg.MaxVolumes / 2, cfg.MaxDrives / 2, false, false},
		{cfg.MaxVolumes / 4, cfg.MaxDrives / 4, false, false},
		{100, 100, true, false},
		{50, 50, true, true},
	}

	for _, r := range reductions {
		if r.clearFC {
			out.FlashCopy = json.RawMessage("[]")
		}
		if r.clearRepl {
			out.Replication = json.RawMessage("[]")
		}

		// Re-truncar volumes.
		if err := reTruncateSection(&out.Volumes, r.maxVolumes); err != nil {
			return nil, fmt.Errorf("reduce volumes: %w", err)
		}
		// Re-truncar drives.
		if err := reTruncateSection(&out.Drives, r.maxDrives); err != nil {
			return nil, fmt.Errorf("reduce drives: %w", err)
		}

		out.Status.DurationMs = time.Since(startTime).Milliseconds()

		data, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("zabbix output: re-serialize after reduction: %w", err)
		}

		if len(data) <= cfg.MaxJSONBytes {
			return out, nil
		}
	}

	// Si después de todas las reducciones sigue siendo grande,
	// vaciar volumes y drives completamente.
	out.Volumes = json.RawMessage("[]")
	out.Drives = json.RawMessage("[]")

	return out, nil
}

// reTruncateSection deserializa una sección JSON, la trunca y la re-serializa.
func reTruncateSection(section *json.RawMessage, maxRecords int) error {
	if section == nil || string(*section) == "[]" || maxRecords <= 0 {
		return nil
	}

	var records []parser.Record
	if err := json.Unmarshal(*section, &records); err != nil {
		// Si no puede deserializar, vaciar la sección.
		*section = json.RawMessage("[]")
		return nil
	}

	if len(records) > maxRecords {
		records = records[:maxRecords]
	}

	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("cannot re-serialize section: %w", err)
	}

	*section = json.RawMessage(data)
	return nil
}

// LLDOutput genera el formato JSON para Low Level Discovery de Zabbix.
// Zabbix espera: {"data": [{"{#MACRO}": "valor"}, ...]}
// Esta función convierte Records a ese formato usando el mapeo de campos
// especificado en fieldMap.
//
// Ejemplo de uso para drives:
//
//	fieldMap := map[string]string{
//	    "id":           "{#DRIVEID}",
//	    "enclosure_id": "{#ENCLOSUREID}",
//	    "slot_id":      "{#SLOTID}",
//	    "status":       "{#DRIVESTATUS}",
//	}
func LLDOutput(records []parser.Record, fieldMap map[string]string) ([]byte, error) {
	type lldData struct {
		Data []map[string]string `json:"data"`
	}

	items := make([]map[string]string, 0, len(records))

	for _, record := range records {
		item := make(map[string]string, len(fieldMap))
		for field, macro := range fieldMap {
			value := ""
			if v, ok := record[field]; ok {
				value = v
			}
			item[macro] = value
		}
		items = append(items, item)
	}

	result := lldData{Data: items}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("lld output: cannot serialize: %w", err)
	}

	return data, nil
}

// LLD field maps para cada tipo de recurso.
// Usados por Zabbix LLD rules para crear items dinámicos.

// DriveLLDFields mapea campos de lsdrive a macros Zabbix LLD.
var DriveLLDFields = map[string]string{
	"id":           "{#DRIVEID}",
	"enclosure_id": "{#ENCLOSUREID}",
	"slot_id":      "{#SLOTID}",
	"status":       "{#DRIVESTATUS}",
	"tech_type":    "{#DRIVETYPE}",
	"capacity":     "{#DRIVECAPACITY}",
}

// PoolLLDFields mapea campos de lsmdiskgrp a macros Zabbix LLD.
var PoolLLDFields = map[string]string{
	"id":     "{#POOLID}",
	"name":   "{#POOLNAME}",
	"status": "{#POOLSTATUS}",
}

// VolumeLLDFields mapea campos de lsvdisk a macros Zabbix LLD.
var VolumeLLDFields = map[string]string{
	"id":             "{#VOLUMEID}",
	"name":           "{#VOLUMENAME}",
	"status":         "{#VOLUMESTATUS}",
	"mdisk_grp_name": "{#VOLUMEPOOL}",
}

// EnclosureLLDFields mapea campos de lsenclosure a macros Zabbix LLD.
var EnclosureLLDFields = map[string]string{
	"id":          "{#ENCLOSUREID}",
	"status":      "{#ENCLOSURESTATUS}",
	"product_MTM": "{#ENCLOSUREMODEL}",
}

// NodeLLDFields mapea campos de lsnode a macros Zabbix LLD.
var NodeLLDFields = map[string]string{
	"id":            "{#NODEID}",
	"name":          "{#NODENAME}",
	"status":        "{#NODESTATUS}",
	"IO_group_name": "{#NODEIOGROUP}",
}

// PortLLDFields mapea campos de lsportfc a macros Zabbix LLD.
var PortLLDFields = map[string]string{
	"id":         "{#PORTID}",
	"WWPN":       "{#WWPN}",
	"status":     "{#PORTSTATUS}",
	"node_name":  "{#PORTNODENAME}",
	"port_speed": "{#PORTSPEED}",
}
