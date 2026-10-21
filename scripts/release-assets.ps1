# Gera o asset de release e o SHA256SUMS a partir do binário já buildado.
#
#   powershell -File scripts/release-assets.ps1 -Exe build/bin/spira.exe `
#               -Asset spira-2.2.0-portable-windows-amd64.zip
#
# Duas regras que o updater do Wails exige (pkg/updater):
#
#   1. zip com UM ÚNICO entry de topo — maybeExtractInto rejeita archives com
#      mais de uma entrada de topo ("the helper has nothing meaningful to
#      swap into a single target path"). O nome do asset precisa conter o
#      GOOS e o GOARCH rodando (DefaultAssetMatcher casa por substring).
#   2. SHA256SUMS no formato `sha256sum`: "<hex-digest>  <nome-do-arquivo>",
#      que é o que parseChecksumLine do provider GitHub espera. Publicar o
#      binário SEM o sidecar derruba o install (Config.ChecksumAsset).
#
# Chamada pela task `wails3 task release` (o nome do asset vem de
# RELEASE_ASSET no Taskfile raiz — fonte única do nome).
param(
    [Parameter(Mandatory = $true)]
    [string]$Exe,

    [Parameter(Mandatory = $true)]
    [string]$Asset,

    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Exe)) {
    Write-Error "binário ausente: $Exe — rode 'wails3 task build' antes de publicar."
    exit 1
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$zip = Join-Path $OutDir $Asset

Compress-Archive -Force -Path $Exe -DestinationPath $zip

$hash = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLower()
# -NoNewline: uma linha só, sem o CRLF final que quebraria o parser.
Set-Content -NoNewline -Encoding ascii -Path (Join-Path $OutDir "SHA256SUMS") `
    -Value ("{0}  {1}" -f $hash, $Asset)

Write-Host "asset : $zip"
Write-Host "sha256: $hash"
Get-Item $zip | Select-Object Name, Length
