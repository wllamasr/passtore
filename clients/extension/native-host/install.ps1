# install.ps1 — Registra el host de native messaging de Passtore en Windows.
#
# Uso (PowerShell):
#   .\install.ps1 -ExtensionId <id-de-la-extension> -NativeExe <ruta\passtore-native.exe> [-Browser chrome|edge]
#
# El <id-de-la-extension> lo ves en chrome://extensions (o edge://extensions)
# tras cargar la extensión en "modo desarrollador" (Cargar descomprimida).

param(
  [Parameter(Mandatory = $true)][string]$ExtensionId,
  [Parameter(Mandatory = $true)][string]$NativeExe,
  [ValidateSet('chrome', 'edge')][string]$Browser = 'chrome'
)

$ErrorActionPreference = 'Stop'

$exe = (Resolve-Path $NativeExe).Path
$hostName = 'com.passtore.host'

# Carpeta estable para el manifiesto del host.
$destDir = Join-Path $env:LOCALAPPDATA 'passtore'
New-Item -ItemType Directory -Force -Path $destDir | Out-Null
$manifestPath = Join-Path $destDir "$hostName.json"

$manifest = [ordered]@{
  name            = $hostName
  description     = 'Passtore native messaging host'
  path            = $exe
  type            = 'stdio'
  allowed_origins = @("chrome-extension://$ExtensionId/")
}
$manifest | ConvertTo-Json -Depth 4 | Out-File -FilePath $manifestPath -Encoding utf8

if ($Browser -eq 'edge') {
  $regBase = 'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts'
} else {
  $regBase = 'HKCU:\Software\Google\Chrome\NativeMessagingHosts'
}
$regKey = Join-Path $regBase $hostName
New-Item -Path $regKey -Force | Out-Null
Set-ItemProperty -Path $regKey -Name '(Default)' -Value $manifestPath

Write-Host "OK. Host nativo registrado para $Browser." -ForegroundColor Green
Write-Host "  Manifiesto: $manifestPath"
Write-Host "  Ejecutable: $exe"
Write-Host "  Extension : $ExtensionId"
Write-Host "Reinicia el navegador si la extensión ya estaba abierta."
