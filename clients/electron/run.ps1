# run.ps1 — Lanza la app de escritorio Passtore apuntando al agente compilado.
#
# Uso (PowerShell, desde clients/electron):
#   .\run.ps1
#
# Requiere:
#   - Haber compilado los binarios:  go build -o bin\passtore-agent.exe .\cmd\passtore-agent  (desde la raíz)
#   - Tener un vault creado:          .\bin\passtore.exe init
#   - Haber instalado dependencias:   npm install   (dentro de clients/electron)

$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$repo = Resolve-Path (Join-Path $here '..\..')

$agent = Join-Path $repo 'bin\passtore-agent.exe'
if (-not (Test-Path $agent)) {
  Write-Error "No encuentro $agent. Compílalo:  go build -o bin\passtore-agent.exe .\cmd\passtore-agent"
}
$env:PASSTORE_AGENT_BIN = $agent

$electron = Join-Path $here 'node_modules\electron\dist\electron.exe'
if (-not (Test-Path $electron)) {
  Write-Error "Electron no está instalado. Ejecuta 'npm install' en clients/electron."
}

Write-Host "Lanzando Passtore (agente: $agent)…"
& $electron $here
