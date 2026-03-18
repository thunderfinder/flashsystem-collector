#SCRIPT DE POWERSHELL PARA UNIFICAR ARCHIVOS EN UN SOLO TXT
# Nombre del archivo de salida
$archivoSalida = "resultado_unificado.txt"

# Ruta base como string
$basePath = (Get-Location).Path

# Crear/limpiar archivo
New-Item -ItemType File -Path $archivoSalida -Force | Out-Null

# Obtener carpetas (incluyendo raíz correctamente)
$carpetas = @($basePath) + (Get-ChildItem -Recurse -Directory | Select-Object -ExpandProperty FullName)

foreach ($carpeta in $carpetas) {

    if (-not $carpeta) { continue }  # protección extra

    # Ruta relativa manual (más estable que Resolve-Path)
    $rutaRelativa = $carpeta.Replace($basePath, ".")
    if ($rutaRelativa -eq "") { $rutaRelativa = "." }

    # Header carpeta
    $headerCarpeta = "`n" + ("#" * 60) + "`n"
    $headerCarpeta += " CARPETA: $rutaRelativa `n"
    $headerCarpeta += ("#" * 60) + "`n"

    Add-Content -Path $archivoSalida -Value $headerCarpeta

    # Archivos dentro de la carpeta
    $archivos = Get-ChildItem -Path $carpeta -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -ne $archivoSalida }

    foreach ($archivo in $archivos) {

        $rutaArchivoRelativa = $archivo.FullName.Replace($basePath, ".")

        $encabezado = "`n" + ("=" * 50) + "`n"
        $encabezado += " ARCHIVO: $rutaArchivoRelativa `n"
        $encabezado += ("=" * 50) + "`n"

        Add-Content -Path $archivoSalida -Value $encabezado

        try {
            $contenido = Get-Content -Path $archivo.FullName -Raw -ErrorAction Stop
            Add-Content -Path $archivoSalida -Value $contenido
        }
        catch {
            Add-Content -Path $archivoSalida -Value "[ERROR: No se pudo leer (binario o bloqueado)]"
        }
    }
}

Write-Host "Proceso completado. Archivo generado: $archivoSalida" -ForegroundColor Green