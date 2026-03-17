#!/usr/bin/env bash
# =============================================================================
# flashsystem_collector.sh
# Wrapper para Zabbix External Scripts
#
# Uso desde Zabbix item key:
#   collect:  flashsystem_collector.sh[{HOST.IP},collect]
#   discover: flashsystem_collector.sh[{HOST.IP},discover,drives]
#
# Variables de entorno requeridas (configurar en zabbix_server.conf o
# en el archivo de entorno /etc/zabbix/flashsystem/env):
#   FS_USER      - Usuario SSH del FlashSystem
#   FS_KEYFILE   - Ruta a la clave privada SSH
#   FS_INSECURE  - "true" para deshabilitar host key check (redes aisladas)
#
# =============================================================================
set -euo pipefail

# -----------------------------------------------------------------------------
# CONFIGURACIÓN
# -----------------------------------------------------------------------------
readonly COLLECTOR_BIN="/usr/local/bin/flashsystem-collector"
readonly ENV_FILE="/etc/zabbix/flashsystem/env"
readonly LOG_FILE="/var/log/zabbix/flashsystem_collector.log"
readonly LOG_MAX_BYTES=10485760  # 10MB - rotar si supera este tamaño
readonly LOCK_DIR="/var/lock/flashsystem"
readonly LOCK_TIMEOUT=55         # segundos - menor que intervalo Zabbix (60s)

# -----------------------------------------------------------------------------
# FUNCIONES DE UTILIDAD
# -----------------------------------------------------------------------------

log() {
    local level="$1"
    shift
    local msg="$*"
    local ts
    ts=$(date '+%Y-%m-%d %H:%M:%S')
    echo "${ts} [${level}] ${msg}" >> "${LOG_FILE}" 2>/dev/null || true
}

log_info()  { log "INFO " "$@"; }
log_warn()  { log "WARN " "$@"; }
log_error() { log "ERROR" "$@"; }

# Rotar log si supera el tamaño máximo.
rotate_log_if_needed() {
    if [[ -f "${LOG_FILE}" ]]; then
        local size
        size=$(stat -c%s "${LOG_FILE}" 2>/dev/null || echo 0)
        if [[ "${size}" -gt "${LOG_MAX_BYTES}" ]]; then
            mv "${LOG_FILE}" "${LOG_FILE}.1" 2>/dev/null || true
            touch "${LOG_FILE}" 2>/dev/null || true
        fi
    fi
}

# Emitir JSON de error estructurado a stdout.
# Zabbix siempre recibe JSON válido, nunca output vacío.
emit_error_json() {
    local msg="$1"
    local ts
    ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    printf '{"timestamp":"%s","version":"wrapper-1.0","status":{"success":false,"errors":["%s"]}}\n' \
        "${ts}" "${msg//\"/\\\"}"
}

# Emitir LLD vacío para discover en caso de error.
emit_empty_lld() {
    printf '{"data":[]}\n'
}

# Verificar que el binario del collector existe y es ejecutable.
check_binary() {
    if [[ ! -f "${COLLECTOR_BIN}" ]]; then
        log_error "Binary not found: ${COLLECTOR_BIN}"
        return 1
    fi
    if [[ ! -x "${COLLECTOR_BIN}" ]]; then
        log_error "Binary not executable: ${COLLECTOR_BIN}"
        return 1
    fi
    return 0
}

# Cargar variables de entorno desde archivo si existe.
load_env_file() {
    if [[ -f "${ENV_FILE}" ]]; then
        # Leer solo líneas con formato KEY=VALUE, ignorar comentarios y vacías.
        # Usar set -a para exportar automáticamente las variables cargadas.
        set -a
        # shellcheck source=/dev/null
        source "${ENV_FILE}"
        set +a
        log_info "Loaded env from ${ENV_FILE}"
    fi
}

# Verificar variables de entorno obligatorias.
check_env() {
    local host="$1"
    local missing=()

    if [[ -z "${host}" ]]; then
        missing+=("HOST (argument 1)")
    fi
    if [[ -z "${FS_USER:-}" ]]; then
        missing+=("FS_USER")
    fi
    # Requiere exactamente uno: FS_KEYFILE o FS_PASSWORD.
    if [[ -z "${FS_KEYFILE:-}" && -z "${FS_PASSWORD:-}" ]]; then
        missing+=("FS_KEYFILE or FS_PASSWORD")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required config: ${missing[*]}"
        return 1
    fi

    # Verificar que el keyfile existe si fue configurado.
    if [[ -n "${FS_KEYFILE:-}" && ! -f "${FS_KEYFILE}" ]]; then
        log_error "SSH key file not found: ${FS_KEYFILE}"
        return 1
    fi

    return 0
}

# Construir argumentos de autenticación SSH para el collector.
build_auth_args() {
    local auth_args=()

    if [[ -n "${FS_KEYFILE:-}" ]]; then
        auth_args+=("-keyfile" "${FS_KEYFILE}")
    elif [[ -n "${FS_PASSWORD:-}" ]]; then
        # Password: menos seguro pero soportado para redes aisladas.
        auth_args+=("-password" "${FS_PASSWORD}")
    fi

    if [[ "${FS_INSECURE:-false}" == "true" || "${FS_INSECURE:-false}" == "1" ]]; then
        auth_args+=("-insecure")
    elif [[ -n "${FS_KNOWN_HOSTS:-}" ]]; then
        auth_args+=("-known-hosts" "${FS_KNOWN_HOSTS}")
    fi

    if [[ -n "${FS_CACHE_DIR:-}" ]]; then
        auth_args+=("-cache-dir" "${FS_CACHE_DIR}")
    fi

    if [[ -n "${FS_METRICS_TTL:-}" ]]; then
        auth_args+=("-metrics-ttl" "${FS_METRICS_TTL}")
    fi

    if [[ -n "${FS_DISCOVERY_TTL:-}" ]]; then
        auth_args+=("-discovery-ttl" "${FS_DISCOVERY_TTL}")
    fi

    if [[ "${FS_VERBOSE:-false}" == "true" ]]; then
        auth_args+=("-verbose")
    fi

    printf '%s\n' "${auth_args[@]}"
}

# Adquirir lock para evitar ejecuciones simultáneas del mismo host.
# Usa flock sobre un archivo de lock por host.
acquire_lock() {
    local host="$1"
    local lock_file="${LOCK_DIR}/flashsystem_${host//[^a-zA-Z0-9_-]/_}.lock"

    mkdir -p "${LOCK_DIR}" 2>/dev/null || true

    # exec sobre descriptor 200 para mantener el lock durante la ejecución.
    exec 200>"${lock_file}"

    if ! flock -w "${LOCK_TIMEOUT}" 200; then
        log_warn "Could not acquire lock for ${host} after ${LOCK_TIMEOUT}s — another instance running?"
        return 1
    fi

    log_info "Lock acquired for ${host}"
    return 0
}

# -----------------------------------------------------------------------------
# FUNCIONES PRINCIPALES
# -----------------------------------------------------------------------------

# Ejecutar collector en modo collect (Master Item).
run_collect() {
    local host="$1"
    shift
    local auth_args=("$@")

    log_info "Starting collect for host=${host}"

    local output
    local exit_code=0

    output=$(
        "${COLLECTOR_BIN}" collect \
            -host "${host}" \
            -user "${FS_USER}" \
            "${auth_args[@]}" \
            2>>"${LOG_FILE}"
    ) || exit_code=$?

    if [[ ${exit_code} -ne 0 ]]; then
        log_error "Collector exited with code ${exit_code} for host=${host}"
        # El binario Go siempre emite JSON válido incluso en error.
        # Si output está vacío (crash del proceso), emitir error JSON.
        if [[ -z "${output}" ]]; then
            emit_error_json "collector process crashed with exit code ${exit_code}"
            return
        fi
    fi

    log_info "Collect completed for host=${host} (output size: ${#output} bytes)"
    printf '%s\n' "${output}"
}

# Ejecutar collector en modo discover (LLD).
run_discover() {
    local host="$1"
    local category="$2"
    shift 2
    local auth_args=("$@")

    # Validar categoría antes de llamar al binario.
    case "${category}" in
        drives|pools|volumes|enclosures|nodes|ports)
            ;;
        *)
            log_error "Invalid discovery category: ${category}"
            emit_empty_lld
            return
            ;;
    esac

    log_info "Starting discover for host=${host} category=${category}"

    local output
    local exit_code=0

    output=$(
        "${COLLECTOR_BIN}" discover \
            -host "${host}" \
            -user "${FS_USER}" \
            -category "${category}" \
            "${auth_args[@]}" \
            2>>"${LOG_FILE}"
    ) || exit_code=$?

    if [[ ${exit_code} -ne 0 ]]; then
        log_error "Discovery exited with code ${exit_code} for host=${host} category=${category}"
        if [[ -z "${output}" ]]; then
            emit_empty_lld
            return
        fi
    fi

    log_info "Discover completed for host=${host} category=${category}"
    printf '%s\n' "${output}"
}

# -----------------------------------------------------------------------------
# MAIN
# -----------------------------------------------------------------------------

main() {
    rotate_log_if_needed

    # Argumentos posicionales desde Zabbix:
    # $1 = host (de {HOST.IP} o {HOST.DNS})
    # $2 = modo: collect | discover
    # $3 = categoría (solo para discover)
    if [[ $# -lt 2 ]]; then
        log_error "Usage: $0 <host> <collect|discover> [category]"
        emit_error_json "invalid arguments: expected host and mode"
        exit 1
    fi

    local host="$1"
    local mode="$2"
    local category="${3:-}"

    # Cargar variables de entorno desde archivo de configuración.
    load_env_file

    # Verificar binario.
    if ! check_binary; then
        case "${mode}" in
            discover) emit_empty_lld ;;
            *)        emit_error_json "collector binary not found: ${COLLECTOR_BIN}" ;;
        esac
        exit 1
    fi

    # Verificar variables de entorno obligatorias.
    if ! check_env "${host}"; then
        case "${mode}" in
            discover) emit_empty_lld ;;
            *)        emit_error_json "missing required environment variables" ;;
        esac
        exit 1
    fi

    # Adquirir lock por host para evitar ejecuciones simultáneas.
    # Si no se puede adquirir el lock, continuar de todas formas
    # (Zabbix puede tener múltiples pollers y no debe bloquearse).
    if ! acquire_lock "${host}"; then
        log_warn "Proceeding without lock for ${host}"
    fi

    # Construir argumentos de autenticación.
    mapfile -t auth_args < <(build_auth_args)

    # Ejecutar según modo.
    case "${mode}" in
        collect)
            run_collect "${host}" "${auth_args[@]}"
            ;;
        discover)
            if [[ -z "${category}" ]]; then
                log_error "discover mode requires category argument"
                emit_empty_lld
                exit 1
            fi
            run_discover "${host}" "${category}" "${auth_args[@]}"
            ;;
        *)
            log_error "Unknown mode: ${mode}. Valid: collect|discover"
            emit_error_json "unknown mode: ${mode}"
            exit 1
            ;;
    esac
}

main "$@"