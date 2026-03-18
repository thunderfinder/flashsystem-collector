#SCRIPT DE POWERSHELL PARA UNIFICAR ARCHIVOS EN UN SOLO TXT
# Definir el nombre del archivo de salida
$archivoSalida = "resultado_unificado.txt"

# Obtener todos los archivos de la carpeta actual y sus subcarpetas
# Excluimos el propio archivo de salida para evitar un bucle infinito
$archivos = Get-ChildItem -Recurse -File | Where-Object { $_.Name -ne $archivoSalida }

# Limpiar o crear el archivo de salida
New-Item -ItemType File -Path $archivoSalida -Force | Out-Null

foreach ($archivo in $archivos) {
    # Crear el encabezado con el nombre del archivo
    $encabezado = "`n" + ("=" * 50) + "`n"
    $encabezado += " ARCHIVO: $($archivo.FullName) `n"
    $encabezado += ("=" * 50) + "`n"

    # Escribir el encabezado en el archivo final
    Add-Content -Path $archivoSalida -Value $encabezado

    # Intentar leer y añadir el contenido del archivo
    try {
        $contenido = Get-Content -Path $archivo.FullName -Raw -ErrorAction Stop
        Add-Content -Path $archivoSalida -Value $contenido
    }
    catch {
        Add-Content -Path $archivoSalida -Value "[ERROR: No se pudo leer este archivo (puede ser un binario o estar bloqueado)]"
    }
}

Write-Host "Proceso completado. Todos los archivos se han unido en: $archivoSalida" -ForegroundColor Green