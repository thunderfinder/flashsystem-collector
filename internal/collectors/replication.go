package collectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/flashsystem-collector/internal/parser"
	internalssh "github.com/flashsystem-collector/internal/ssh"
)

// ReplicationCollector recolecta información de relaciones de replicación remota.
type ReplicationCollector struct {
	ttl time.Duration
}

// NewReplicationCollector crea un ReplicationCollector con el TTL especificado.
func NewReplicationCollector(ttl time.Duration) *ReplicationCollector {
	return &ReplicationCollector{ttl: ttl}
}

// Name implementa Collector.
func (c *ReplicationCollector) Name() string {
	return "replication"
}

// TTL implementa Collector.
func (c *ReplicationCollector) TTL() time.Duration {
	return c.ttl
}

// Collect ejecuta comandos de replicación con fallback entre versiones.
//
// Spectrum Virtualize v8.7+ usa lsreplicationrelationship.
// Versiones anteriores usan lsrcrelationship.
// Ambos retornan formato tabular.
//
// Retorna Records vacío sin error si no hay replicación configurada.
//
// Implementa Collector.
func (c *ReplicationCollector) Collect(client *internalssh.Client) ([]parser.Record, error) {

	// Intentar comando moderno primero (v8.7+).
	output, err := client.Run("svcinfo lsreplicationrelationship -delim :")
	if err == nil {
		result := parser.Parse(output)
		return result.Records, nil
	}

	// Si el error es "comando no encontrado" o similar, intentar legacy.
	// Distinguir entre error de comando no existente vs error de red/timeout.
	if isCommandNotFound(err) {
		output, err = client.Run("svcinfo lsrcrelationship -delim :")
		if err != nil {
			// Si el legacy también falla con "no encontrado", no hay replicación.
			if isCommandNotFound(err) {
				return []parser.Record{}, nil
			}
			return nil, fmt.Errorf("replication collector (legacy): %w", err)
		}
		result := parser.Parse(output)
		return result.Records, nil
	}

	return nil, fmt.Errorf("replication collector: %w", err)
}

// isCommandNotFound detecta si el error de SSH es por comando no reconocido.
// Spectrum Virtualize retorna mensajes específicos para comandos inválidos.
func isCommandNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cmmvc5753e") || // IBM: command not valid
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "command not recognized")
}
