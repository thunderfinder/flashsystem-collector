# instructions.md
# flashsystem-collector — Documentación Técnica Completa

---

## 1. Descripción del proyecto

`flashsystem-collector` es un binario Go que recolecta métricas operativas de sistemas **IBM FlashSystem / Spectrum Virtualize** mediante conexión SSH y comandos `svcinfo` de la CLI nativa del storage. El binario serializa los resultados en **JSON estructurado** emitido por stdout, diseñado para ser consumido por **Zabbix** como External Script.

El sistema opera en dos modos excluyentes:

- **`collect`**: ejecuta todos los collectors (system, nodes, enclosures, drives, pools, volumes, ports, flashcopy, replication, performance, batteries, psus) en paralelo y emite un JSON único con todos los datos. Este JSON está diseñado para usarse como **Master Item** en Zabbix, del cual se derivan Dependent Items.
- **`discover`**: ejecuta un único collector para una categoría específica y emite JSON en formato **LLD (Low Level Discovery)** de Zabbix con macros `{#MACRO}`.

El sistema implementa un **cache persistente en JSON** en disco para evitar conexiones SSH innecesarias. Si SSH falla, intenta servir datos del cache aunque estén expirados (stale cache) antes de retornar error.

---

## 2. Arquitectura detectada

### Componentes encontrados

```
flashsystem-collector/
├── go.mod                              # Módulo: github.com/flashsystem-collector
├── main.go                             # Punto de entrada, routing de subcomandos
└── internal/
    ├── config/
    │   └── config.go                   # Carga y validación de configuración
    ├── ssh/
    │   └── client.go                   # Cliente SSH con timeout por comando
    ├── cache/
    │   └── cache.go                    # Cache JSON persistente thread-safe
    ├── parser/
    │   └── parser.go                   # Parser dual vertical/tabular de svcinfo
    ├── collectors/
    │   ├── collector.go                # Interfaz Collector, Runner, AllCollectors()
    │   ├── system.go                   # lssystem (formato vertical)
    │   ├── nodes.go                    # lsnode
    │   ├── enclosures.go               # lsenclosure
    │   ├── drives.go                   # lsdrive
    │   ├── pools.go                    # lsmdiskgrp
    │   ├── volumes.go                  # lsvdisk
    │   ├── ports.go                    # lsportfc
    │   ├── flashcopy.go                # lsfcmap
    │   ├── replication.go              # lsreplicationrelationship / lsrcrelationship
    │   └── performance.go              # lssystemstats
    └── zabbix/
        └── output.go                   # Construcción y serialización del JSON final
```

### Organización del código

| Paquete | Responsabilidad |
|---------|----------------|
| `main` | Routing de subcomandos, orquestación, fallback stale cache |
| `config` | Flags CLI + env vars, validación, ruta del archivo de cache |
| `ssh` | Conexión SSH, ejecución de comandos con timeout por contexto |
| `cache` | Lectura/escritura JSON en disco, TTL por entrada, escritura atómica |
| `parser` | Detección de formato y parseo de output de svcinfo |
| `collectors` | Un archivo por recurso IBM, interfaz común, runner paralelo |
| `zabbix` | Construcción del JSON de salida, reducción por tamaño, LLD |

### Flujo de ejecución — modo `collect`

```
main()
  └─ os.Args[1] == "collect"
       └─ runCollect()
            ├─ config.Load()              — flags + env vars + validación
            ├─ cache.New(cacheFile)       — carga cache desde disco
            ├─ cache.Purge()              — elimina entradas expiradas
            ├─ internalssh.New()          — dial TCP + handshake SSH
            │    └─ si falla → buildFromStaleCache() → emitJSON() → return
            ├─ collectors.AllCollectors() — instancia los 10 collectors con sus TTLs
            ├─ collectors.NewRunner()     — configura semáforo MaxConcurrent=3
            ├─ runner.RunAll()            — ejecuta collectors en paralelo
            │    └─ por cada collector:
            │         ├─ cache.Get(key)   — si hit → retorna records cacheados
            │         └─ si miss:
            │              ├─ adquiere slot semáforo
            │              ├─ col.Collect(client) → client.Run(cmd)
            │              ├─ libera slot semáforo
            │              └─ cache.Set(key, records, ttl)
            ├─ zabbix.Build()             — construye Output, aplica límites
            └─ emitJSON()                 — stdout
```

### Flujo de ejecución — modo `discover`

```
main()
  └─ os.Args[1] == "discover"
       └─ runDiscover()
            ├─ config.Load()
            ├─ flag -category validado
            ├─ cache.New(cacheFile)
            ├─ cache.Get(MakeDiscoveryKey(host, category))
            │    └─ si hit → emitLLDFromCache() → return
            ├─ internalssh.New()
            │    └─ si falla → emitEmptyLLD() → return
            ├─ runDiscoveryCollector()    — ejecuta 1 collector
            ├─ cache.Set(discoveryKey, lldItems, DiscoveryTTL)
            └─ zabbix.LLDOutput()        — stdout formato {"data":[...]}
```

---

## 3. Requisitos identificados

### Sistema operativo
- Linux (el binario se compila con `GOOS=linux`)
- El path de cache default es `/var/tmp` — debe existir y ser escribible por el usuario que ejecuta el binario
- El directorio de lock `/var/lock/flashsystem` es usado por el wrapper Bash, no por el binario Go

### Go
- Versión mínima: **Go 1.21** (declarado en `go.mod`)
- Razón: uso de `os.ReadFile()` (1.16+), `any` como alias de `interface{}` (1.18+)

### Dependencias externas (go.mod)
| Módulo | Versión | Uso |
|--------|---------|-----|
| `golang.org/x/crypto` | v0.22.0 | Cliente SSH (`golang.org/x/crypto/ssh`, `knownhosts`) |
| `golang.org/x/sys` | v0.19.0 | Dependencia indirecta de crypto |

### Acceso de red
- Conectividad TCP al FlashSystem en el puerto configurado (default: **22**)
- El usuario SSH debe tener rol **Monitor** (solo lectura) en el FlashSystem

### Filesystem
- Directorio de cache debe ser escribible (default `/var/tmp`)
- Si se usa SSH key: el archivo `.pem` debe ser legible por el usuario que ejecuta el binario

---

## 4. Instalación (derivada del código)

### Compilar desde fuente

```bash
# Desde el directorio raíz del proyecto
go mod tidy                          # descarga dependencias y genera go.sum

# Binario estático para Linux x86_64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o flashsystem-collector .
```

### Instalar binario

```bash
sudo cp flashsystem-collector /usr/local/bin/
sudo chmod 755 /usr/local/bin/flashsystem-collector
sudo chown root:root /usr/local/bin/flashsystem-collector
```

### Instalar wrapper Zabbix (opcional)

```bash
sudo cp scripts/flashsystem_collector.sh \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
sudo chmod 755 /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
sudo chown zabbix:zabbix /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
```

---

## 5. Configuración detectada

### Fuentes de configuración (orden de prioridad)

El paquete `config` aplica en este orden: **flags CLI > variables de entorno > valores por defecto**.

Un flag explícito siempre sobreescribe la variable de entorno correspondiente.

### Flags CLI y variables de entorno equivalentes

| Flag CLI | Variable de entorno | Tipo | Default | Descripción |
|----------|-------------------|------|---------|-------------|
| `-host` | `FS_HOST` | string | — | IP o hostname del FlashSystem (**obligatorio**) |
| `-port` | `FS_PORT` | int | `22` | Puerto SSH |
| `-user` | `FS_USER` | string | — | Usuario SSH (**obligatorio**) |
| `-keyfile` | `FS_KEYFILE` | string | — | Ruta a clave privada SSH (**obligatorio** si no hay `-password`) |
| `-password` | `FS_PASSWORD` | string | — | Password SSH (**obligatorio** si no hay `-keyfile`) |
| `-insecure` | `FS_INSECURE` | bool | `false` | Deshabilita verificación de host key SSH |
| `-known-hosts` | `FS_KNOWN_HOSTS` | string | — | Ruta al archivo `known_hosts` |
| `-cache-dir` | `FS_CACHE_DIR` | string | `/var/tmp` | Directorio para el archivo de cache |
| `-metrics-ttl` | `FS_METRICS_TTL` | duration | `5m` | TTL de cache para métricas operativas |
| `-discovery-ttl` | `FS_DISCOVERY_TTL` | duration | `4h` | TTL de cache para datos de discovery |
| `-dial-timeout` | `FS_DIAL_TIMEOUT` | duration | `10s` | Timeout de conexión TCP+SSH |
| `-cmd-timeout` | `FS_CMD_TIMEOUT` | duration | `30s` | Timeout por comando SSH individual |
| `-max-json-bytes` | `FS_MAX_JSON_BYTES` | int | `1048576` | Tamaño máximo del JSON de salida (bytes) |
| `-max-volumes` | `FS_MAX_VOLUMES` | int | `500` | Máximo de volúmenes en el output |
| `-max-drives` | `FS_MAX_DRIVES` | int | `500` | Máximo de drives en el output |
| `-verbose` | `FS_VERBOSE` | bool | `false` | Logging de debug a stderr |
| `-category` | — | string | — | Solo para subcomando `discover`: `drives|pools|volumes|enclosures|nodes|ports|batteries|psus` |

### Reglas de validación (aplicadas en `config.validate()`)

- `host` y `user` son obligatorios — el binario termina con error si faltan
- Exactamente **uno** de `keyfile` o `password` debe estar configurado — ambos o ninguno son error
- Si `keyfile` está configurado: el archivo debe existir y ser accesible en el momento de inicio
- Si `insecure=false` y `known-hosts` está configurado: el archivo debe existir
- Si `insecure=false` y `known-hosts` está vacío: el binario retorna error (no asume insecure automáticamente)
- `dial-timeout` debe ser >= 1s
- `cmd-timeout` debe ser >= 1s
- `metrics-ttl` debe ser >= 1s
- `discovery-ttl` debe ser >= 1m
- `max-json-bytes` debe ser >= 1024
- El directorio de cache debe existir y ser escribible (verificado creando un archivo temporal)

### Archivo de entorno del wrapper Bash

El wrapper `flashsystem_collector.sh` carga `/etc/zabbix/flashsystem/env` si existe.
Formato: pares `KEY=VALUE`, líneas con `#` son ignoradas.

```bash
# Ejemplo de /etc/zabbix/flashsystem/env
FS_USER=monitor
FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key
FS_INSECURE=true
FS_CACHE_DIR=/var/tmp
FS_METRICS_TTL=5m
FS_DISCOVERY_TTL=4h
FS_VERBOSE=false
```

### Archivo de cache

- Ruta calculada por `config.CacheFile(host)`: `<cache-dir>/flashsystem_<host_sanitizado>.cache.json`
- El host se sanitiza reemplazando `:`, `/`, `\`, ` ` por `_`
- Ejemplo: host `10.10.10.50` → `/var/tmp/flashsystem_10.10.10.50.cache.json`
- Un host diferente produce un archivo de cache diferente

---

## 6. Uso del sistema

### Subcomandos disponibles

```
flashsystem-collector <subcomando> [flags]
```

| Subcomando | Descripción |
|------------|-------------|
| `collect` | Recolecta todas las métricas, emite JSON completo |
| `discover` | Ejecuta discovery LLD para una categoría |
| `version` | Imprime la versión (`2.0.0`) y termina |

Cualquier otro valor en `os.Args[1]` produce error y termina con exit code 1.
Si no se provee ningún subcomando (`len(os.Args) < 2`), imprime usage y termina con exit code 1.

### Ejemplos de ejecución del binario

```bash
# Collect con SSH key (recomendado)
flashsystem-collector collect \
    -host 10.10.10.50 \
    -user monitor \
    -keyfile /etc/zabbix/ssh/flashsystem_key \
    -insecure

# Collect con password en red aislada
flashsystem-collector collect \
    -host 10.10.10.50 \
    -user monitor \
    -password secreto \
    -insecure

# Collect via variables de entorno
FS_HOST=10.10.10.50 FS_USER=monitor \
FS_KEYFILE=/etc/zabbix/ssh/fs_key FS_INSECURE=true \
flashsystem-collector collect

# Discovery de drives
flashsystem-collector discover \
    -host 10.10.10.50 \
    -user monitor \
    -keyfile /etc/zabbix/ssh/flashsystem_key \
    -insecure \
    -category drives

# Ver versión
flashsystem-collector version
```

### Ejemplos de uso del wrapper Bash

```bash
# Collect (modo Master Item Zabbix)
/usr/lib/zabbix/externalscripts/flashsystem_collector.sh 10.10.10.50 collect

# Discovery de pools
/usr/lib/zabbix/externalscripts/flashsystem_collector.sh 10.10.10.50 discover pools
```

El wrapper recibe exactamente 2 o 3 argumentos posicionales:
- `$1`: host (IP o hostname)
- `$2`: modo (`collect` o `discover`)
- `$3`: categoría (solo para `discover`)

### Formato del output — modo `collect`

JSON emitido por stdout con la siguiente estructura de primer nivel:

```json
{
  "timestamp": "2026-03-17T10:00:00Z",
  "host": "10.10.10.50",
  "version": "2.0.0",
  "system":      [ { "id": "0", "name": "FlashSystem_5045", ... } ],
  "nodes":       [ { "id": "1", "name": "node1", "status": "online", ... } ],
  "enclosures":  [ { "id": "1", "status": "online", ... } ],
  "drives":      [ { "id": "0", "status": "online", ... } ],
  "pools":       [ { "id": "0", "name": "Pool0", ... } ],
  "volumes":     [ { "id": "0", "name": "vol_prod_001", ... } ],
  "ports":       [ { "id": "1", "WWPN": "500507...", ... } ],
  "flashcopy":   [ { "id": "0", "status": "idle_or_copied", ... } ],
  "replication": [ { ... } ],
  "performance": [ { "stat_name": "read_io", "stat_current": "912", ... } ],
  "status": {
    "success": true,
    "duration_ms": 1234,
    "collector_runs": [ { "name": "system", "record_count": 1, "cache_hit": false, ... } ],
    "errors": [],
    "truncated_at": null
  },
  "cache_stats": {
    "active_entries": 10,
    "expired_entries": 0,
    "total_entries": 10,
    "file_path": "/var/tmp/flashsystem_10.10.10.50.cache.json",
    "updated_at": "2026-03-17T10:00:00Z"
  }
}
```

Todos los campos de cada Record son **strings** — los valores numéricos como capacidades o IOPS están como strings tal como los retorna la CLI de IBM.

### Formato del output — modo `discover`

```json
{
  "data": [
    { "{#DRIVEID}": "0", "{#ENCLOSUREID}": "1", "{#SLOTID}": "1", "{#DRIVESTATUS}": "online", "{#DRIVETYPE}": "flash", "{#DRIVECAPACITY}": "894.3GB" },
    { "{#DRIVEID}": "1", "{#ENCLOSUREID}": "1", "{#SLOTID}": "2", "{#DRIVESTATUS}": "online", "{#DRIVETYPE}": "flash", "{#DRIVECAPACITY}": "894.3GB" }
  ]
}
```

### Macros LLD disponibles por categoría

| Categoría | Macros generadas |
|-----------|-----------------|
| `drives` | `{#DRIVEID}`, `{#ENCLOSUREID}`, `{#SLOTID}`, `{#DRIVESTATUS}`, `{#DRIVETYPE}`, `{#DRIVECAPACITY}` |
| `pools` | `{#POOLID}`, `{#POOLNAME}`, `{#POOLSTATUS}` |
| `volumes` | `{#VOLUMEID}`, `{#VOLUMENAME}`, `{#VOLUMESTATUS}`, `{#VOLUMEPOOL}` |
| `enclosures` | `{#ENCLOSUREID}`, `{#ENCLOSURESTATUS}`, `{#ENCLOSUREMODEL}` |
| `nodes` | `{#NODEID}`, `{#NODENAME}`, `{#NODESTATUS}`, `{#NODEIOGROUP}` |
| `ports` | `{#PORTID}`, `{#WWPN}`, `{#PORTSTATUS}`, `{#PORTNODENAME}`, `{#PORTSPEED}` |

---

## 7. Integraciones detectadas

### Zabbix

El sistema está diseñado explícitamente para Zabbix mediante:

**External Scripts**: el wrapper `flashsystem_collector.sh` está diseñado para colocarse en el directorio `ExternalScripts` de Zabbix (default `/usr/lib/zabbix/externalscripts/`). Zabbix lo invoca pasando parámetros desde la key del item.

**Master Item + Dependent Items**: el modo `collect` emite un JSON único. Zabbix usa un solo item que almacena ese JSON, y los valores individuales se extraen con JSONPath en Dependent Items — esto evita múltiples conexiones SSH.

**Low Level Discovery (LLD)**: el modo `discover` emite el formato `{"data":[{"{#MACRO}":"valor"}]}` que Zabbix consume directamente en Discovery Rules para crear items y triggers dinámicos.

**Keys de Zabbix sugeridas en el código**:

```
# Master Item
flashsystem_collector.sh[{HOST.IP},collect]

# Discovery Rules
flashsystem_collector.sh[{HOST.IP},discover,drives]
flashsystem_collector.sh[{HOST.IP},discover,pools]
flashsystem_collector.sh[{HOST.IP},discover,volumes]
flashsystem_collector.sh[{HOST.IP},discover,enclosures]
flashsystem_collector.sh[{HOST.IP},discover,nodes]
flashsystem_collector.sh[{HOST.IP},discover,ports]
```

**Extracción de valores con JSONPath** (ejemplos basados en la estructura real del JSON):

```
# Estado del cluster
$.system[0].status

# IOPS de lectura actuales
$.performance[?(@.stat_name=='read_io')].stat_current

# Capacidad libre de un pool (con macro LLD {#POOLNAME})
$.pools[?(@.name=='{#POOLNAME}')].free_capacity

# Estado de un drive (con macro LLD {#DRIVEID})
$.drives[?(@.id=='{#DRIVEID}')].status

# Estado general de la colección
$.status.success

# Errores detectados
$.status.errors
```

### IBM FlashSystem / Spectrum Virtualize

Comandos SSH ejecutados por cada collector:

| Collector | Comando ejecutado |
|-----------|------------------|
| system | `svcinfo lssystem -delim :` |
| nodes | `svcinfo lsnode -delim :` |
| enclosures | `svcinfo lsenclosure -delim :` |
| drives | `svcinfo lsdrive -delim :` |
| pools | `svcinfo lsmdiskgrp -delim :` |
| volumes | `svcinfo lsvdisk -delim :` |
| ports | `svcinfo lsportfc -delim :` |
| flashcopy | `svcinfo lsfcmap -delim :` |
| replication | `svcinfo lsreplicationrelationship -delim :` (fallback: `svcinfo lsrcrelationship -delim :`) |
| performance | `svcinfo lssystemstats -delim :` |

El collector `replication` detecta automáticamente si el comando moderno falla con un error que contenga `cmmvc5753e`, `not found`, `unknown command` o `command not recognized`, y en ese caso reintenta con el comando legacy.

---

## 8. Sistema de cache

### Implementación

El cache es un archivo JSON en disco. Es **thread-safe** mediante `sync.RWMutex`. La escritura es **atómica**: los datos se escriben a un archivo temporal (`.flashsystem_cache_*.tmp`) en el mismo directorio, y luego se hace `os.Rename()` al archivo final — esto garantiza que el archivo nunca quede en estado corrupto aunque el proceso muera durante la escritura.

### Ubicación

```
<cache-dir>/flashsystem_<host_sanitizado>.cache.json
```

Default: `/var/tmp/flashsystem_10.10.10.50.cache.json`

Cada host monitoreado tiene su propio archivo de cache.

### Estructura del archivo de cache

```json
{
  "entries": {
    "10.10.10.50:pools": {
      "value": [...],
      "expires_at": "2026-03-17T10:05:00Z",
      "cached_at": "2026-03-17T10:00:00Z",
      "key": "10.10.10.50:pools"
    },
    "10.10.10.50:discovery:drives": {
      "value": [...],
      "expires_at": "2026-03-17T14:00:00Z",
      "cached_at": "2026-03-17T10:00:00Z",
      "key": "10.10.10.50:discovery:drives"
    }
  },
  "updated_at": "2026-03-17T10:00:00Z",
  "version": "1"
}
```

### Claves de cache

- Métricas: `"<host>:<collector>"` — ej: `"10.10.10.50:pools"`
- Discovery: `"<host>:discovery:<collector>"` — ej: `"10.10.10.50:discovery:drives"`

### TTLs por tipo de dato

| Tipo | TTL default | Configurable via |
|------|------------|-----------------|
| Métricas operativas (pools, volumes, nodes, system, flashcopy, replication) | 5 minutos | `-metrics-ttl` / `FS_METRICS_TTL` |
| Performance (lssystemstats) | 1 minuto | no hay flag separado — usa `PerformanceTTL` hardcoded en `AllCollectors()` pasando `cfg.Cache.PerformanceTTL` |
| Discovery (drives, enclosures, ports, pools, volumes, nodes, batteries, psus) | 4 horas | `-discovery-ttl` / `FS_DISCOVERY_TTL` |

> **Nota**: `PerformanceTTL` tiene su propio campo en `CacheConfig` (default 60s) pero no tiene flag CLI ni variable de entorno expuesta en el código analizado. Su valor es siempre el default de 60s salvo que se modifique el código fuente.

### Comportamiento ante cache corrupto

Si el archivo de cache existe pero el JSON es inválido, o si la versión del formato no es `"1"`, el cache se descarta silenciosamente y se inicializa vacío. El archivo corrompido no se elimina — será sobreescrito en la próxima escritura.

### Comportamiento de purge

Al inicio de cada ejecución `collect`, se llama `cache.Purge()` que elimina todas las entradas cuyo `expires_at` ya pasó y persiste el archivo limpio. El error de purge es ignorado (`_ = c.Purge()`).

### Stale cache (fallback por fallo SSH)

Si SSH falla durante `collect`, el sistema intenta retornar el **último valor cacheado aunque esté expirado** (`GetStale()`). El campo `error` del collector afectado incluye el mensaje `"SSH failed (stale cache from <timestamp>): <error>"`. Si tampoco hay valor stale, el campo `records` es `[]` y `error` indica `"SSH failed and no cache available"`.

---

## 9. Manejo de errores

### Salida garantizada en stdout

El sistema garantiza que **siempre hay output JSON válido en stdout**, incluso ante errores fatales. Nunca emite stdout vacío.

| Situación | Comportamiento |
|-----------|---------------|
| Error de configuración (flag faltante, archivo no accesible) | `fatalJSON()` → JSON con `status.success=false` + exit code 1 |
| SSH falla al conectar | `buildFromStaleCache()` → JSON con datos expirados o vacíos, sin exit code 1 |
| Collector individual falla (SSH timeout, comando inválido) | `result.Error` poblado, otros collectors continúan |
| JSON serialization falla | JSON de emergencia mínimo hardcoded en `emitJSON()` |
| Modo `discover`, SSH falla | `{"data":[]}` + log a stderr |
| Modo `discover`, categoría inválida | `{"data":[]}` + log a stderr |
| Proceso Go crash (solo en wrapper Bash) | wrapper detecta output vacío y emite `emit_error_json()` |

### Logs

Los logs se escriben a **stderr** (capturado por Zabbix en el log del servidor). El wrapper Bash adicionalmente escribe logs al archivo `/var/log/zabbix/flashsystem_collector.log`.

El binario Go escribe a stderr usando `logStderr()` únicamente para mensajes de error/warning. No escribe logs en condiciones normales a menos que `-verbose` esté activo.

### Exit codes del binario Go

| Exit code | Condición |
|-----------|-----------|
| `0` | Ejecución normal (incluso con errores parciales de collectors) |
| `1` | Error fatal: configuración inválida, subcomando desconocido, sin argumentos |

### `status.success` en el JSON de salida

`status.success` es `false` **solo si todos los collectors fallaron**. Si al menos uno tuvo éxito, es `true` aunque haya errores parciales. Los errores individuales se listan en `status.errors[]`.

---

## 10. Limitaciones detectadas

### Sin persistencia de conexión SSH entre procesos
Cada ejecución del binario establece una nueva conexión SSH. El cache en disco reduce la frecuencia con que se necesita SSH, pero no hay connection pooling real entre invocaciones de Zabbix.

### Un host por ejecución
El binario acepta exactamente un `-host` por ejecución. Para monitorear múltiples FlashSystems, Zabbix debe invocar el script por separado para cada host.

### `PerformanceTTL` no configurable externamente
El TTL de performance (default 60s) está en `CacheConfig` pero no está expuesto como flag CLI ni variable de entorno en el código analizado. Para cambiarlo se requiere modificar el código fuente.

### Sin paginación de volúmenes
`lsvdisk` se ejecuta sin flags de paginación — retorna todos los volúmenes en una llamada. El límite `MaxVolumes` solo trunca el output JSON, no limita la consulta SSH.

### Sin template Zabbix incluido
El código no incluye ningún archivo XML/YAML de template para Zabbix. El documento menciona la estructura de items y LLD pero el template debe crearse manualmente en Zabbix.

### Campos de Record son siempre strings
El parser retorna todos los valores como `string`. No hay conversión automática a tipos numéricos. Las expresiones JSONPath en Zabbix deben manejar que valores como `"read_io"` o `"capacity"` son strings.

### Verificación de host key requiere configuración explícita
Si `insecure=false` (default) y no se provee `-known-hosts`, el binario termina con error en lugar de usar `~/.ssh/known_hosts` automáticamente.

### El wrapper Bash requiere `flock` y `mapfile`
`flock` y `mapfile` (Bash 4+) deben estar disponibles en el sistema. Estos son estándar en RHEL/CentOS 7+ pero no están disponibles en todos los sistemas Unix.

### Rotación de log del wrapper es básica
El wrapper rota `/var/log/zabbix/flashsystem_collector.log` a `.log.1` cuando supera 10MB, pero solo mantiene 1 archivo de backup. No hay compresión ni múltiples rotaciones.

### Sin autenticación con passphrase en SSH key
`ssh.ParsePrivateKey()` se llama sin passphrase. Si la clave privada tiene passphrase, la carga falla con error.

---

## 11. Troubleshooting basado en código

### El binario termina con exit 1 y no produce output JSON

**Causa más probable**: error de configuración antes de que `fatalJSON()` pueda construir el output.

**Verificar**:
```bash
flashsystem-collector collect -host 10.10.10.50 -user monitor \
    -keyfile /ruta/key -insecure 2>&1
# Leer el mensaje de error en stderr
```

**Errores comunes de configuración**:
- `SSH host is required` → falta `-host` o `FS_HOST`
- `SSH authentication required` → falta `-keyfile` y `-password`
- `SSH key file not accessible` → el archivo de clave no existe o sin permisos de lectura
- `SSH host key verification not configured` → falta `-insecure` o `-known-hosts`
- `cache directory error` → `/var/tmp` no existe o no es escribible por el usuario

### El JSON de salida tiene `status.success: false` con todos los collectors en error

**Causa**: SSH falla al conectar y tampoco hay cache previo.

**Verificar conexión SSH manual**:
```bash
ssh -i /ruta/key -o StrictHostKeyChecking=no monitor@10.10.10.50 \
    "svcinfo lssystem -delim :"
```

**Causas posibles**:
- FlashSystem no accesible (firewall, red)
- Credenciales incorrectas
- Usuario sin permisos en el storage
- `dial-timeout` muy corto (default 10s)

### El JSON de salida incluye `"SSH failed (stale cache from ...)"`

Esto es comportamiento normal ante fallo de SSH. El sistema retornó el último valor conocido del cache. La conexión SSH falló pero hay datos históricos disponibles.

### Algunos collectors en error, otros exitosos

Normal cuando un comando específico falla en el storage:

- `performance collector: empty response from lssystemstats` → las estadísticas están deshabilitadas en el FlashSystem. Solución: `svctask startstats` en el storage.
- `replication collector` con error → no hay replicación configurada o el comando no existe en esa versión. El collector retorna `[]` si detecta que el comando no existe.
- `system collector: empty response from lssystem` → problema de permisos SSH o storage offline.

### El archivo de cache crece indefinidamente

`Purge()` se llama al inicio de cada `collect`. Si el binario no se ejecuta frecuentemente o el proceso falla antes de `Purge()`, las entradas expiradas se acumulan.

**Limpiar manualmente**:
```bash
rm /var/tmp/flashsystem_10.10.10.50.cache.json
```
El cache se reconstruye en la próxima ejecución.

### Discovery retorna `{"data":[]}`

**Causas posibles en orden de probabilidad**:
1. SSH falla — ver sección anterior
2. Categoría inválida — debe ser exactamente: `drives`, `pools`, `volumes`, `enclosures`, `nodes`, `ports`, `batteries`, `psus`
3. El storage no tiene ítems de esa categoría (ej: no hay FlashCopy configurado)
4. Cache de discovery está presente y tiene lista vacía — borrar el cache

### El wrapper Bash falla con `missing required config`

Variables `FS_USER`, `FS_KEYFILE` (o `FS_PASSWORD`) no están definidas al momento de ejecución del wrapper.

**Verificar que el archivo de entorno existe y tiene permisos correctos**:
```bash
ls -la /etc/zabbix/flashsystem/env
# Debe ser: -rw-r----- zabbix zabbix
sudo -u zabbix cat /etc/zabbix/flashsystem/env
```

### El JSON supera el límite de tamaño de Zabbix

Si `status.truncated_at` está presente en el JSON, se aplicó truncación. Si aun así el JSON es grande:

- Reducir `-max-volumes` y `-max-drives`
- En Zabbix, aumentar el parámetro `ValueCacheSize` o el límite de valor de item

---

## 12. Validación

### Verificar compilación

```bash
go build ./...
# Sin output = sin errores de compilación
```

### Verificar binario instalado

```bash
flashsystem-collector version
# Output esperado: flashsystem-collector 2.0.0
```

### Verificar conectividad SSH

```bash
sudo -u zabbix ssh \
    -i /etc/zabbix/ssh/flashsystem_key \
    -o StrictHostKeyChecking=no \
    monitor@10.10.10.50 \
    "svcinfo lssystem -delim :" | head -5
# Debe retornar líneas con formato "campo:valor"
```

### Verificar ejecución del collector

```bash
sudo -u zabbix \
    FS_USER=monitor \
    FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key \
    FS_INSECURE=true \
    /usr/local/bin/flashsystem-collector collect \
    -host 10.10.10.50 2>/dev/null | python3 -m json.tool > /dev/null
# Sin output de error = JSON válido
echo "Exit code: $?"
# Debe ser: Exit code: 0
```

### Verificar campos clave del JSON

```bash
sudo -u zabbix \
    FS_USER=monitor FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key FS_INSECURE=true \
    /usr/local/bin/flashsystem-collector collect -host 10.10.10.50 2>/dev/null \
    | python3 -c "
import json,sys
d=json.load(sys.stdin)
print('success:', d['status']['success'])
print('system name:', d['system'][0].get('name','N/A') if d['system'] else 'EMPTY')
print('nodes:', len(d['nodes']))
print('pools:', len(d['pools']))
print('drives:', len(d['drives']))
print('cache_hits:', sum(1 for r in d['status']['collector_runs'] if r['cache_hit']))
print('errors:', d['status']['errors'])
"
```

### Verificar cache

```bash
# Verificar que el archivo de cache se creó
ls -lh /var/tmp/flashsystem_*.cache.json

# Verificar estructura del cache
python3 -m json.tool /var/tmp/flashsystem_10.10.10.50.cache.json | head -30

# Verificar que segunda ejecución usa cache (cache_hit: true)
sudo -u zabbix \
    FS_USER=monitor FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key FS_INSECURE=true \
    /usr/local/bin/flashsystem-collector collect -host 10.10.10.50 2>/dev/null \
    | python3 -c "
import json,sys
d=json.load(sys.stdin)
for r in d['status']['collector_runs']:
    print(f\"{r['name']:15} cache_hit={r['cache_hit']} records={r['record_count']}\")
"
```

### Verificar discovery

```bash
sudo -u zabbix \
    FS_USER=monitor FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key FS_INSECURE=true \
    /usr/local/bin/flashsystem-collector discover \
    -host 10.10.10.50 \
    -category drives 2>/dev/null \
    | python3 -m json.tool | head -20
# Debe mostrar {"data": [{"{#DRIVEID}": "0", ...}, ...]}
```

### Verificar wrapper Zabbix

```bash
sudo -u zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh \
    10.10.10.50 collect 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print('OK:', d['status']['success'])"
# Debe mostrar: OK: True
```

---

*Documento generado por análisis estático del código fuente. Versión del proyecto: 2.0.0*