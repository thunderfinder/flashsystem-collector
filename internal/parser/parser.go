package parser

import (
	"math"
	"strconv"
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

	lines, rawLines := extractLines(output)
	result.RawLines = rawLines

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

	lines, rawLines := extractLines(output)
	result.RawLines = rawLines

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
	// Pre-allocating capacity for record map avoids hashmap growth reallocations.
	record := make(Record, len(lines))

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
	if len(headers) == 0 {
		return Record{}
	}

	// Pre-allocating map capacity matching header count avoids incremental map rehashes.
	record := make(Record, len(headers))

	// En lugar de strings.SplitN(line, delim, len(headers)) que aloca un []string
	// en el heap para cada fila, escaneamos las posiciones del delimitador in-place
	// evitando allocations intermedias por fila.
	rest := line
	lastIdx := len(headers) - 1

	for i, header := range headers {
		if header == "" {
			// Saltar extracción manteniendo sincronización del scanner
			if i < lastIdx && rest != "" {
				idx := strings.Index(rest, delim)
				if idx >= 0 {
					rest = rest[idx+len(delim):]
				} else {
					rest = ""
				}
			}
			continue
		}

		if i == lastIdx {
			// El último campo preserva cualquier delimitador restante.
			record[header] = strings.TrimSpace(rest)
			break
		}

		idx := strings.Index(rest, delim)
		if idx >= 0 {
			record[header] = strings.TrimSpace(rest[:idx])
			rest = rest[idx+len(delim):]
		} else {
			// No quedan más delimitadores en la fila
			record[header] = strings.TrimSpace(rest)
			rest = ""
		}
	}

	return record
}

// splitTabularLine divide una línea de headers/valores por ":".
// Trimea espacios de cada elemento.
// Escanea delimitadores in-place sobre la línea para evitar alocar un slice intermedio con strings.Split.
func splitTabularLine(line string) []string {
	numDelims := strings.Count(line, delim)
	result := make([]string, 0, numDelims+1)

	rest := line
	for {
		idx := strings.Index(rest, delim)
		if idx < 0 {
			trimmed := strings.TrimSpace(rest)
			if trimmed != "" {
				result = append(result, trimmed)
			}
			break
		}
		trimmed := strings.TrimSpace(rest[:idx])
		if trimmed != "" {
			result = append(result, trimmed)
		}
		rest = rest[idx+len(delim):]
	}

	return result
}

// extractLines escanea in-place las líneas de output divididas por '\n',
// filtrando líneas en blanco y pre-alocando la capacidad del slice destino
// según strings.Count(output, "\n") + 1. Retorna las líneas no vacías y
// la cantidad total de líneas (raw lines).
func extractLines(output string) ([]string, int) {
	numNewlines := strings.Count(output, "\n")
	result := make([]string, 0, numNewlines+1)
	rawCount := 0

	rest := output
	for {
		rawCount++
		idx := strings.IndexByte(rest, '\n')
		if idx < 0 {
			if strings.TrimSpace(rest) != "" {
				result = append(result, rest)
			}
			break
		}
		line := rest[:idx]
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
		rest = rest[idx+1:]
	}

	return result, rawCount
}

// isNoResults detecta las variantes de "sin resultados" que retorna svcinfo.
// Spectrum Virtualize retorna distintas variantes según la versión.
// Utiliza strings.EqualFold y verificación de prefijo para evitar alocaciones de heap
// innecesarias en cada línea procesada de tablas grandes (e.g. lsvdisk, lsdrive).
func isNoResults(line string) bool {
	line = strings.TrimSpace(line)
	return strings.EqualFold(line, "no results") ||
		strings.EqualFold(line, "no result") ||
		strings.EqualFold(line, "no objects found") ||
		(len(line) >= 5 && strings.EqualFold(line[:5], "cmmvc"))
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

// parseCapacityParts extracts the numeric string prefix and unit string suffix from val
// using zero-allocation scanning instead of regular expressions.
func parseCapacityParts(val string) (num float64, unit string, ok bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, "", false
	}

	// Must start with digit
	if val[0] < '0' || val[0] > '9' {
		return 0, "", false
	}

	i := 0
	hasDot := false
	for i < len(val) {
		c := val[i]
		if c >= '0' && c <= '9' {
			i++
		} else if c == '.' && !hasDot {
			hasDot = true
			i++
		} else {
			break
		}
	}

	numPart := val[:i]
	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, "", false
	}

	rest := strings.TrimSpace(val[i:])
	// Optional unit: verify all characters are letters if present
	for j := 0; j < len(rest); j++ {
		c := rest[j]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return 0, "", false
		}
	}

	return num, strings.ToUpper(rest), true
}

// FormatCapacity normaliza o formatea capacidades de almacenamiento según la regla:
// Para menos de un terabyte (< 1 TB) se expresa en GB, y para 1 TB o más en TB.
// Si el valor no se puede interpretar numéricamente o viene vacío, se retorna intacto.
func FormatCapacity(val string) string {
	num, unit, ok := parseCapacityParts(val)
	if !ok {
		return val
	}

	// Base-2 binary units matching IBM Storage Virtualize CLI Guide (Table 3):
	// KB: 1024, MB: 1024^2, GB: 1024^3, TB: 1024^4, PB: 1024^5
	const (
		bytesPerKB = 1024.0
		bytesPerMB = 1024.0 * bytesPerKB
		bytesPerGB = 1024.0 * bytesPerMB
		bytesPerTB = 1024.0 * bytesPerGB
		bytesPerPB = 1024.0 * bytesPerTB
	)

	var bytes float64
	switch unit {
	case "B", "":
		bytes = num
	case "KB", "KIB", "K":
		bytes = num * bytesPerKB
	case "MB", "MIB", "M":
		bytes = num * bytesPerMB
	case "GB", "GIB", "G":
		bytes = num * bytesPerGB
	case "TB", "TIB", "T":
		bytes = num * bytesPerTB
	case "PB", "PIB", "P":
		bytes = num * bytesPerPB
	default:
		// Unknown unit, return original value
		return val
	}

	// For sizes less than 1 TB: display in GB.
	// For sizes 1 TB or greater: display in TB.
	if bytes < bytesPerTB {
		gbVal := bytes / bytesPerGB
		return formatUnitNumber(gbVal) + "GB"
	}

	tbVal := bytes / bytesPerTB
	return formatUnitNumber(tbVal) + "TB"
}

func formatUnitNumber(v float64) string {
	// Round to 1 decimal place if fractional, or integer if whole
	rounded := math.Round(v*10) / 10
	if math.Abs(rounded-math.Round(rounded)) < 1e-9 {
		return strconv.FormatInt(int64(rounded), 10)
	}
	return strconv.FormatFloat(rounded, 'f', 1, 64)
}
