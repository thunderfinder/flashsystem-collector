package parser

import (
	"strings"
)

// Record representa una fila de datos parseada de svcinfo.
// Las claves son los nombres de campo tal como los retorna la CLI.
// Todos los valores son strings — la conversión a tipos numéricos
// es responsabilidad de cada collector.
type Record map[string]string

// Result contiene los registros parseados y metadata del parse.
type Result struct {
	Records  []Record
	Format   Format
	Command  string
	RawLines int // Cantidad de líneas en el output original
}

// Format indica el formato detectado en el output.
type Format int

const (
	FormatUnknown  Format = iota
	FormatVertical        // key:value por línea (ej: lssystem)
	FormatTabular         // primera línea headers, resto valores (ej: lsdrive)
	FormatEmpty           // output vacío o sin resultados
)

// String retorna la representación textual del formato.
func (f Format) String() string {
	switch f {
	case FormatVertical:
		return "vertical"
	case FormatTabular:
		return "tabular"
	case FormatEmpty:
		return "empty"
	default:
		return "unknown"
	}
}

// delim es el delimitador estándar usado con -delim : en svcinfo.
const delim = ":"

// Parse parsea el output de un comando svcinfo y retorna un Result.
// Detecta automáticamente el formato (vertical o tabular).
// Nunca retorna nil en Records — retorna slice vacío si no hay datos.
// No retorna error — los problemas de parse se reflejan en Records vacío.
func Parse(output string) Result {
	result := Result{
		Records: []Record{},
	}

	// Normalizar line endings (FlashSystem puede retornar \r\n).
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.TrimSpace(output)

	if output == "" {
		result.Format = FormatEmpty
		return result
	}

	lines := splitLines(output)
	result.RawLines = len(lines)

	// Filtrar líneas vacías.
	lines = filterEmpty(lines)

	if len(lines) == 0 {
		result.Format = FormatEmpty
		return result
	}

	// Detectar "No results" — respuesta estándar de svcinfo cuando no hay datos.
	if len(lines) == 1 && isNoResults(lines[0]) {
		result.Format = FormatEmpty
		return result
	}

	// Detectar formato: vertical vs tabular.
	format := detectFormat(lines)
	result.Format = format

	switch format {
	case FormatVertical:
		result.Records = parseVertical(lines)
	case FormatTabular:
		result.Records = parseTabular(lines)
	default:
		// Formato desconocido: intentar tabular como fallback.
		result.Format = FormatTabular
		result.Records = parseTabular(lines)
	}

	return result
}

// ParseVerticalForced parsea el output forzando formato vertical.
// Usar cuando se sabe con certeza que el comando retorna formato vertical
// (ej: lssystem, lssystemstats cuando retorna un solo bloque).
func ParseVerticalForced(output string) Result {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.TrimSpace(output)

	result := Result{
		Records: []Record{},
		Format:  FormatVertical,
	}

	if output == "" {
		result.Format = FormatEmpty
		return result
	}

	lines := filterEmpty(splitLines(output))
	result.RawLines = len(lines)

	if len(lines) == 0 || (len(lines) == 1 && isNoResults(lines[0])) {
		result.Format = FormatEmpty
		return result
	}

	result.Records = parseVertical(lines)
	return result
}

// detectFormat determina si el output es vertical o tabular.
//
// Lógica de detección:
//   - Formato VERTICAL: cada línea tiene exactamente UN separador ":"
//     y la primera línea NO parece ser una fila de headers múltiples.
//   - Formato TABULAR: la primera línea tiene MÚLTIPLES separadores ":"
//     (son los headers de columna).
//
// Casos especiales:
//   - lssystemstats retorna tabular con muchas columnas.
//   - lssystem retorna vertical con un campo por línea.
func detectFormat(lines []string) Format {
	if len(lines) == 0 {
		return FormatEmpty
	}

	firstLine := lines[0]
	colonCount := strings.Count(firstLine, delim)

	if colonCount == 0 {
		// Sin delimitadores — formato desconocido, tratar como tabular.
		return FormatTabular
	}

	if colonCount == 1 {
		// Una sola columna en la primera línea.
		// Si hay más líneas y todas tienen exactamente 1 ":", es vertical.
		// Si la segunda línea también tiene 1 ":", probablemente vertical.
		if len(lines) > 1 {
			secondColons := strings.Count(lines[1], delim)
			if secondColons == 1 {
				return FormatVertical
			}
		}
		// Una sola línea con un ":" — asumir vertical (key:value).
		return FormatVertical
	}

	// Múltiples ":" en la primera línea — es la fila de headers tabular.
	return FormatTabular
}

// parseVertical parsea formato vertical donde cada línea es "key:value".
// Retorna un único Record con todos los campos.
//
// Ejemplo de input (lssystem -delim :):
//
//	id:0
//	name:FlashSystem_5045
//	location:local
//	...
//
// Manejo especial: si el valor contiene ":", lo preserva completo.
// Ej: "code_level:8.7.0.0 (build 123.456)" → value = "8.7.0.0 (build 123.456)"
func parseVertical(lines []string) []Record {
	record := Record{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Separar en key y value usando solo el PRIMER ":" como separador.
		// Esto preserva valores que contienen ":" (ej: timestamps, versiones).
		idx := strings.Index(line, delim)
		if idx < 0 {
			// Línea sin delimitador — ignorar.
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if key == "" {
			// Clave vacía — ignorar.
			continue
		}

		record[key] = value
	}

	if len(record) == 0 {
		return []Record{}
	}

	return []Record{record}
}

// parseTabular parsea formato tabular donde la primera línea son headers
// y las siguientes son filas de valores, separados por ":".
//
// Ejemplo de input (lsdrive -delim :):
//
//	id:enclosure_id:slot_id:status:capacity
//	0:1:1:online:1.2TB
//	1:1:2:online:1.2TB
//
// Manejo de columnas desiguales:
//   - Si una fila tiene menos valores que headers: campos faltantes = "".
//   - Si una fila tiene más valores que headers: valores extra se concatenan
//     al último campo (preserva valores con ":" en el último campo).
func parseTabular(lines []string) []Record {
	if len(lines) < 2 {
		// Sin datos (solo header o nada).
		return []Record{}
	}

	// Parsear headers desde la primera línea.
	headers := splitTabularLine(lines[0])
	if len(headers) == 0 {
		return []Record{}
	}

	records := make([]Record, 0, len(lines)-1)

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isNoResults(line) {
			continue
		}

		record := buildRecord(headers, line)
		if len(record) > 0 {
			records = append(records, record)
		}
	}

	return records
}

// buildRecord construye un Record a partir de los headers y una línea de valores.
// Maneja el caso donde el último campo puede contener el delimitador.
func buildRecord(headers []string, line string) Record {
	record := Record{}

	if len(headers) == 0 {
		return record
	}

	// Para todos los campos excepto el último, usamos Split con límite.
	// Para el último campo, tomamos el resto de la línea para preservar
	// valores que contengan ":".
	//
	// Estrategia: split con límite = len(headers).
	// strings.SplitN(s, sep, n) retorna máximo n substrings,
	// con el último conteniendo el resto no spliteado.
	parts := strings.SplitN(line, delim, len(headers))

	for i, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}

		if i < len(parts) {
			record[header] = strings.TrimSpace(parts[i])
		} else {
			// Columna ausente en esta fila.
			record[header] = ""
		}
	}

	return record
}

// splitTabularLine divide una línea de headers/valores por ":".
// Trimea espacios de cada elemento.
func splitTabularLine(line string) []string {
	parts := strings.Split(line, delim)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitLines divide el output en líneas individuales.
func splitLines(output string) []string {
	return strings.Split(output, "\n")
}

// filterEmpty retorna solo las líneas no vacías.
func filterEmpty(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

// isNoResults detecta las variantes de "sin resultados" que retorna svcinfo.
// Spectrum Virtualize retorna distintas variantes según la versión.
func isNoResults(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return lower == "no results" ||
		lower == "no result" ||
		lower == "no objects found" ||
		lower == "cmmvc5845e" || // Código de error IBM: objeto no encontrado
		strings.HasPrefix(lower, "cmmvc") // Cualquier error IBM empieza con CMMVC
}

// GetField extrae un campo de un Record con valor por defecto si no existe.
// Trimea espacios del valor retornado.
func GetField(r Record, field, defaultVal string) string {
	if v, ok := r[field]; ok {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			return trimmed
		}
	}
	return defaultVal
}

// GetFieldLower extrae un campo y lo convierte a minúsculas.
// Útil para comparaciones de estado (online/offline/degraded).
func GetFieldLower(r Record, field, defaultVal string) string {
	return strings.ToLower(GetField(r, field, defaultVal))
}
