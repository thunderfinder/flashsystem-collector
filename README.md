# flashsystem-collector
Monitoreo de ibm flashsystem con zabbix go script

# flashsystem-collector — Guía de Instalación

## Requisitos

- Go 1.21+
- Linux (Zabbix Server o Proxy)
- Acceso SSH al FlashSystem con usuario de solo lectura (rol Monitor)
- Zabbix 6.0+ (compatible con 7.x)

---

## 1. Compilar el binario
```bash
# Clonar o copiar el proyecto
cd /opt/flashsystem-collector

# Descargar dependencias
go mod tidy

# Compilar binario estático para Linux x86_64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o flashsystem-collector .

# Verificar binario
file flashsystem-collector
# Debe mostrar: ELF 64-bit LSB executable, x86-64, statically linked
```

---

## 2. Instalar binario
```bash
sudo cp flashsystem-collector /usr/local/bin/
sudo chmod 755 /usr/local/bin/flashsystem-collector
sudo chown root:root /usr/local/bin/flashsystem-collector
```

---

## 3. Instalar wrapper Zabbix
```bash
sudo cp scripts/flashsystem_collector.sh \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh

sudo chmod 755 \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh

sudo chown zabbix:zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh
```

---

## 4. Configurar credenciales
```bash
# Crear directorio de configuración
sudo mkdir -p /etc/zabbix/flashsystem
sudo mkdir -p /etc/zabbix/ssh
sudo mkdir -p /var/log/zabbix
sudo mkdir -p /var/lock/flashsystem

# Opción A: SSH Key (recomendado)
sudo -u zabbix ssh-keygen -t ed25519 \
    -C "zabbix-flashsystem-monitor" \
    -f /etc/zabbix/ssh/flashsystem_key \
    -N ""

# Copiar clave pública al FlashSystem
# (ejecutar desde el servidor Zabbix)
ssh-copy-id -i /etc/zabbix/ssh/flashsystem_key.pub \
    monitor@10.10.10.50

# Crear archivo de entorno
sudo tee /etc/zabbix/flashsystem/env > /dev/null << 'EOF'
FS_USER=monitor
FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key
FS_INSECURE=true
FS_CACHE_DIR=/var/tmp
FS_METRICS_TTL=5m
FS_DISCOVERY_TTL=4h
FS_VERBOSE=false
EOF

# Asegurar permisos restrictivos en credenciales
sudo chmod 640 /etc/zabbix/flashsystem/env
sudo chown zabbix:zabbix /etc/zabbix/flashsystem/env
sudo chmod 600 /etc/zabbix/ssh/flashsystem_key
sudo chown zabbix:zabbix /etc/zabbix/ssh/flashsystem_key
sudo chown -R zabbix:zabbix /var/lock/flashsystem
```

---

## 5. Verificar instalación
```bash
# Probar conexión SSH como usuario zabbix
sudo -u zabbix ssh \
    -i /etc/zabbix/ssh/flashsystem_key \
    -o StrictHostKeyChecking=no \
    monitor@10.10.10.50 \
    "svcinfo lssystem -delim :"

# Probar collector directamente
sudo -u zabbix \
    FS_USER=monitor \
    FS_KEYFILE=/etc/zabbix/ssh/flashsystem_key \
    FS_INSECURE=true \
    /usr/local/bin/flashsystem-collector collect \
    -host 10.10.10.50 | head -c 500

# Probar wrapper (como lo llama Zabbix)
sudo -u zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh \
    10.10.10.50 collect | head -c 500

# Probar discovery
sudo -u zabbix \
    /usr/lib/zabbix/externalscripts/flashsystem_collector.sh \
    10.10.10.50 discover drives
```

---

## 6. Configurar Zabbix

### Master Item (External Check)
```
Type:           External check
Key:            flashsystem_collector.sh[{HOST.IP},collect]
Update interval: 60s
Timeout:        55s
Type of info:   Text
```

### Discovery Rules (External Check)
```
# Drives
Key: flashsystem_collector.sh[{HOST.IP},discover,drives]
Update interval: 4h

# Pools
Key: flashsystem_collector.sh[{HOST.IP},discover,pools]
Update interval: 4h

# Volumes
Key: flashsystem_collector.sh[{HOST.IP},discover,volumes]
Update interval: 4h

# Enclosures
Key: flashsystem_collector.sh[{HOST.IP},discover,enclosures]
Update interval: 4h

# Nodes
Key: flashsystem_collector.sh[{HOST.IP},discover,nodes]
Update interval: 4h

# Ports
Key: flashsystem_collector.sh[{HOST.IP},discover,ports]
Update interval: 4h
```

### Dependent Items (ejemplos)
```
# Estado del cluster
Master item:    flashsystem_collector.sh[{HOST.IP},collect]
Key:            flashsystem.system.status[{HOST.IP}]
Preprocessing:  JSONPath: $.system[0].status

# IOPS lectura
Key:            flashsystem.perf.read_io[{HOST.IP}]
Preprocessing:  JSONPath: $.performance[?(@.stat_name=='read_io')].stat_current

# Capacidad libre pool (con LLD)
Key:            flashsystem.pool.free[{HOST.IP},{#POOLNAME}]
Preprocessing:  JSONPath: $.pools[?(@.name=='{#POOLNAME}')].free_capacity

# Estado de drive (con LLD)
Key:            flashsystem.drive.status[{HOST.IP},{#DRIVEID}]
Preprocessing:  JSONPath: $.drives[?(@.id=='{#DRIVEID}')].status
```

---

## 7. Verificar logs
```bash
# Log del wrapper
tail -f /var/log/zabbix/flashsystem_collector.log

# Log del servidor Zabbix
grep -i flashsystem /var/log/zabbix/zabbix_server.log

# Verificar cache
ls -lh /var/tmp/flashsystem_*.cache.json
cat /var/tmp/flashsystem_10.10.10.50.cache.json | python3 -m json.tool | head -50
```

---

## 8. Estructura final del proyecto
```
flashsystem-collector/
├── go.mod
├── main.go
├── internal/
│   ├── config/config.go
│   ├── ssh/client.go
│   ├── cache/cache.go
│   ├── parser/parser.go
│   ├── collectors/
│   │   ├── collector.go
│   │   ├── system.go
│   │   ├── nodes.go
│   │   ├── enclosures.go
│   │   ├── drives.go
│   │   ├── pools.go
│   │   ├── volumes.go
│   │   ├── ports.go
│   │   ├── flashcopy.go
│   │   ├── replication.go
│   │   └── performance.go
│   └── zabbix/output.go
├── scripts/
│   └── flashsystem_collector.sh
└── README_INSTALACION.md
```

---

## Troubleshooting

| Síntoma | Causa probable | Solución |
|---------|---------------|----------|
| Output vacío | Timeout SSH | Aumentar `-dial-timeout` y timeout en Zabbix |
| `{"data":[]}` en discovery | Error SSH o cache vacío | Ver log, probar SSH manual |
| JSON muy grande | Muchos volúmenes | Reducir `-max-volumes` |
| Cache no se actualiza | Permisos en `/var/tmp` | `chown zabbix:zabbix /var/tmp` |
| `binary not found` | Binario no instalado | Verificar `/usr/local/bin/flashsystem-collector` |
| `missing FS_USER` | Env file no cargado | Verificar `/etc/zabbix/flashsystem/env` |