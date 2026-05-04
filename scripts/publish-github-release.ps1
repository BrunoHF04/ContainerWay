# Publica ou atualiza uma release no GitHub com artefatos Windows + Linux.
#
# Pré-requisitos:
# - Token com escopo repo (classic) ou permissão de Contents: write (fine-grained).
# - Arquivos existentes nos caminhos informados (rode .\build.ps1 -Version 1.1.0 se precisar gerar .deb/.flatpak).
#
# Uso (PowerShell):
#   $env:GITHUB_TOKEN = "ghp_..."   # ou fine-grained PAT
#   .\scripts\publish-github-release.ps1
#
# Parâmetros comuns:
#   .\scripts\publish-github-release.ps1 -Tag v1.1.0 -Prerelease:$false
#   .\scripts\publish-github-release.ps1 -ExePath .\ContainerWay.exe -DebPath .\dist\deb\containerway_1.1.0_amd64.deb -FlatpakPath .\dist\flatpak\containerway-1.1.0.flatpak

param(
    [string]$Owner = "",
    [string]$Repo = "",
    [string]$Tag = "v1.1.0",
    [string]$ReleaseName = "",
    [string]$ReleaseBody = "",
    [bool]$Prerelease = $true,
    [string]$ExePath = "",
    [string]$DebPath = "",
    [string]$FlatpakPath = ""
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$token = [string]$env:GITHUB_TOKEN
if ([string]::IsNullOrWhiteSpace($token)) {
    throw "Defina a variavel de ambiente GITHUB_TOKEN com um Personal Access Token (escopo repo ou Contents: write)."
}

Set-Location $PSScriptRoot\..

if ([string]::IsNullOrWhiteSpace($Owner) -or [string]::IsNullOrWhiteSpace($Repo)) {
    $remote = (git remote get-url origin).Trim()
    if ($remote -match "github\.com[:/]([^/]+)/([^/.]+)") {
        $Owner = $Matches[1]
        $Repo = $Matches[2]
    } else {
        throw "Não foi possível extrair Owner/Repo do remote: $remote"
    }
}

if ([string]::IsNullOrWhiteSpace($ExePath)) {
    $ExePath = Join-Path (Get-Location) "ContainerWay.exe"
}
if ([string]::IsNullOrWhiteSpace($DebPath)) {
    $DebPath = Join-Path (Get-Location) "dist\deb\containerway_1.1.0_amd64.deb"
}
if ([string]::IsNullOrWhiteSpace($FlatpakPath)) {
    $FlatpakPath = Join-Path (Get-Location) "dist\flatpak\containerway-1.1.0.flatpak"
}

foreach ($p in @($ExePath, $DebPath, $FlatpakPath)) {
    if (-not (Test-Path -LiteralPath $p)) {
        throw "Arquivo não encontrado: $p`nGere os artefatos com: .\build.ps1 -Version 1.1.0"
    }
}

if ([string]::IsNullOrWhiteSpace($ReleaseName)) {
    $ver = $Tag.TrimStart("v")
    $ReleaseName = "ContainerWay v$ver"
}

if ([string]::IsNullOrWhiteSpace($ReleaseBody)) {
    $ReleaseBody = @"
## Downloads

- **Windows:** \`ContainerWay.exe\`
- **Linux (Debian/Ubuntu):** \`containerway_*_amd64.deb\` — instalar com \`sudo dpkg -i ...\` e corrigir dependências com \`sudo apt-get install -f\` se necessário.
- **Linux (Flatpak):** \`containerway-*.flatpak\` — instalar com \`flatpak install --bundle ...\`

## Requisitos

- **Windows:** Windows 10/11 ou Windows Server; acesso SSH ao host Linux remoto.
- **Linux desktop:** dependências gráficas (X11/Wayland) conforme o pacote; Flatpak requer runtime instalado na máquina de destino.

## Observações

- Em ambientes Windows sem OpenGL nativo, o app tenta aplicar fallback automaticamente na inicialização.
"@
}

$apiHeaders = @{
    "User-Agent"    = "ContainerWay-publish-github-release.ps1"
    "Authorization" = "Bearer $token"
    "Accept"        = "application/vnd.github+json"
    "X-GitHub-Api-Version" = "2022-11-28"
}

function Invoke-GitHubApi {
    param(
        [string]$Method,
        [string]$Uri,
        [object]$Body = $null
    )
    $params = @{
        Method      = $Method
        Uri         = $Uri
        Headers     = $apiHeaders
        ContentType = "application/json"
    }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 12)
    }
    return Invoke-RestMethod @params
}

$releasesUri = "https://api.github.com/repos/$Owner/$Repo/releases"
$existing = $null
try {
    $existing = Invoke-GitHubApi -Method GET -Uri "$releasesUri/tags/$Tag"
} catch {
    $existing = $null
}

if ($null -eq $existing) {
    $createBody = @{
        tag_name         = $Tag
        name             = $ReleaseName
        body             = $ReleaseBody
        draft            = $false
        prerelease       = $Prerelease
        generate_release_notes = $false
    }
    $existing = Invoke-GitHubApi -Method POST -Uri $releasesUri -Body $createBody
    Write-Host "Release criada: $($existing.html_url)"
} else {
    $patchBody = @{
        name       = $ReleaseName
        body       = $ReleaseBody
        prerelease = $Prerelease
    }
    $existing = Invoke-GitHubApi -Method PATCH -Uri "$releasesUri/$($existing.id)" -Body $patchBody
    Write-Host "Release atualizada: $($existing.html_url)"
}

$releaseId = [int]$existing.id
$uploadBase = "https://uploads.github.com/repos/$Owner/$Repo/releases/$releaseId/assets"

$assetNames = @(
    (Split-Path -Leaf $ExePath),
    (Split-Path -Leaf $DebPath),
    (Split-Path -Leaf $FlatpakPath)
)

if ($existing.assets) {
    foreach ($asset in $existing.assets) {
        if ($assetNames -contains $asset.name) {
            Write-Host "Removendo asset existente: $($asset.name)"
            Invoke-GitHubApi -Method DELETE -Uri "https://api.github.com/repos/$Owner/$Repo/releases/assets/$($asset.id)"
        }
    }
}

function Upload-ReleaseAsset {
    param(
        [string]$FilePath
    )
    $name = [Uri]::EscapeDataString((Split-Path -Leaf $FilePath))
    $uploadUri = "$uploadBase?name=$name"
    $bytes = [System.IO.File]::ReadAllBytes($FilePath)
    $uploadHeaders = @{
        "User-Agent"    = "ContainerWay-publish-github-release.ps1"
        "Authorization" = "Bearer $token"
        "Accept"        = "application/vnd.github+json"
        "X-GitHub-Api-Version" = "2022-11-28"
        "Content-Type"  = "application/octet-stream"
    }
    Invoke-RestMethod -Method POST -Uri $uploadUri -Headers $uploadHeaders -Body $bytes | Out-Null
    Write-Host "Enviado: $name"
}

Upload-ReleaseAsset -FilePath $ExePath
Upload-ReleaseAsset -FilePath $DebPath
Upload-ReleaseAsset -FilePath $FlatpakPath

Write-Host "Concluído. Verifique a release no GitHub."
