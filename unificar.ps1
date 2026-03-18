#SCRIPT DE POWERSHELL PARA UNIFICAR ARCHIVOS EN UN SOLO TXT
# Nombre del archivo de salida
$archivoSalida = "resultado_unificado$date.txt"

# Ruta base
$basePath = Get-Location

# Crear/limpiar archivo de salida
New-Item -ItemType File -Path $archivoSalida -Force | Out-Null

# Obtener todas las carpetas (incluyendo la raíz)
$carpetas = Get-ChildItem -Recurse -Directory
$carpetas = ,$basePath + $carpetas  # incluir carpeta raíz

foreach ($carpeta in $carpetas) {

    # Ruta relativa
    $rutaRelativa = Resolve-Path -Path $carpeta.FullName -Relative

    # Encabezado de carpeta
    $headerCarpeta = "`n" + ("#" * 60) + "`n"
    $headerCarpeta += " CARPETA: $rutaRelativa `n"
    $headerCarpeta += ("#" * 60) + "`n"

    Add-Content -Path $archivoSalida -Value $headerCarpeta

    # Obtener archivos de la carpeta actual
    $archivos = Get-ChildItem -Path $carpeta.FullName -File |
        Where-Object { $_.Name -ne $archivoSalida }

    foreach ($archivo in $archivos) {

        $rutaArchivoRelativa = Resolve-Path -Path $archivo.FullName -Relative

        # Encabezado de archivo
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