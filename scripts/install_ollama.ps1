# install_ollama.ps1 — Install Ollama on Windows and pull the models Career Agent Core needs.
#
# Usage (PowerShell):
#   .\scripts\install_ollama.ps1              # install + pull models
#   .\scripts\install_ollama.ps1 -NoModels    # install only
#
# Models pulled: read from .env in the repo root when present (OLLAMA_MODEL,
# OLLAMA_VISION_MODEL, OLLAMA_EMBED_MODEL, OLLAMA_HOST), else these installer
# defaults. A real environment variable of the same name always wins over .env:
#   OLLAMA_MODEL (llama3.1), OLLAMA_VISION_MODEL (llava), OLLAMA_EMBED_MODEL (nomic-embed-text)
#
# After pulling (or with -NoModels), the script confirms every configured model
# is actually present via GET /api/tags. Normally that is fatal on failure;
# with -NoModels it is a warning, since skipping downloads was the point.

param(
    [switch]$NoModels
)

$ErrorActionPreference = "Stop"

function Log($msg)  { Write-Host "[install] $msg" -ForegroundColor Cyan }
function Warn($msg) { Write-Host "[install] $msg" -ForegroundColor Yellow }

# ------------------------------------------------------------------ .env

# Repo root from this script's own location, not the caller's working
# directory, so the script behaves the same however it is invoked.
$RepoRoot = Split-Path -Parent $PSScriptRoot
$EnvFile  = Join-Path $RepoRoot ".env"

# Read one key out of .env without executing the file. .env is user-authored
# and holds secrets (ANTHROPIC_API_KEY, IMAP_APP_PASSWORD), so nothing here
# dot-sources it and nothing but the four model keys is ever read or logged.
# Quote and trailing-comment handling matches godotenv's, which is what
# cmd/agent loads .env with -- installer and agent must resolve the same file
# to the same model names, or bugs.md #441 simply moves rather than closes.
# '#'-commented lines never match the anchored pattern, which is what keeps
# .env.example's commented-out recommendation from being read as config.
function Get-EnvFileValue($key) {
    if (-not (Test-Path -LiteralPath $EnvFile)) { return $null }
    $pattern = '^\s*(?:export\s+)?' + [regex]::Escape($key) + '\s*=(.*)$'
    $value = $null
    foreach ($line in Get-Content -LiteralPath $EnvFile) {
        $m = [regex]::Match($line, $pattern)
        if ($m.Success) { $value = $m.Groups[1].Value }   # last match wins
    }
    if ($null -eq $value) { return $null }
    $value = $value.Trim()
    if ($value.StartsWith('"')) {
        $value = $value.Substring(1)
        $end = $value.IndexOf('"')
        if ($end -ge 0) { $value = $value.Substring(0, $end) }
    } elseif ($value.StartsWith("'")) {
        $value = $value.Substring(1)
        $end = $value.IndexOf("'")
        if ($end -ge 0) { $value = $value.Substring(0, $end) }
    } else {
        # Unquoted: '#' begins a comment. No Ollama model name contains one.
        $hash = $value.IndexOf('#')
        if ($hash -ge 0) { $value = $value.Substring(0, $hash) }
    }
    $value = $value.Trim()
    if ($value -eq "") { return $null }
    return $value
}

function Resolve-Setting($envValue, $key, $fallback) {
    if ($envValue) { return $envValue }
    $fromFile = Get-EnvFileValue $key
    if ($fromFile) { return $fromFile }
    return $fallback
}

$TextModel   = Resolve-Setting $env:OLLAMA_MODEL        "OLLAMA_MODEL"        "llama3.1"
$VisionModel = Resolve-Setting $env:OLLAMA_VISION_MODEL "OLLAMA_VISION_MODEL" "llava"
$EmbedModel  = Resolve-Setting $env:OLLAMA_EMBED_MODEL  "OLLAMA_EMBED_MODEL"  "nomic-embed-text"
$OllamaUrl   = Resolve-Setting $env:OLLAMA_HOST         "OLLAMA_HOST"         "http://localhost:11434"

if (Test-Path -LiteralPath $EnvFile) {
    Log "Read .env (keys it leaves unset fall back to installer defaults): text=$TextModel vision=$VisionModel embed=$EmbedModel"
} else {
    Log "No .env found - using installer defaults: text=$TextModel vision=$VisionModel embed=$EmbedModel. Copy .env.example to .env first to have this installer follow your configured models."
}

if (Get-Command ollama -ErrorAction SilentlyContinue) {
    Log "Ollama is already installed: $((Get-Command ollama).Source)"
} elseif (Get-Command winget -ErrorAction SilentlyContinue) {
    Log "Installing Ollama via winget..."
    winget install -e --id Ollama.Ollama --accept-source-agreements --accept-package-agreements
} else {
    Log "winget not found - downloading the official installer..."
    $setup = Join-Path $env:TEMP "OllamaSetup.exe"
    Invoke-WebRequest -Uri "https://ollama.com/download/OllamaSetup.exe" -OutFile $setup
    Log "Running installer (follow the prompts)..."
    Start-Process -FilePath $setup -Wait
}

# Refresh PATH so a just-installed ollama.exe is found in this session
$env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
            [Environment]::GetEnvironmentVariable("Path", "User")

if (-not (Get-Command ollama -ErrorAction SilentlyContinue)) {
    throw "Ollama installed but not on PATH yet - open a new terminal and re-run this script to pull models."
}

# The Windows installer starts Ollama automatically; wait for the API
Log "Waiting for the Ollama server..."
$up = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        Invoke-RestMethod -Uri "$OllamaUrl/api/version" -TimeoutSec 2 | Out-Null
        $up = $true; break
    } catch { Start-Sleep -Seconds 1 }
}
if (-not $up) {
    Log "Server not responding - starting 'ollama serve' in the background..."
    Start-Process -FilePath "ollama" -ArgumentList "serve" -WindowStyle Hidden
    Start-Sleep -Seconds 5
}

if ($NoModels) {
    Log "Skipping model downloads (-NoModels). Pull later with:"
    Log "  ollama pull $TextModel; ollama pull $VisionModel; ollama pull $EmbedModel"
} else {
    Log "Pulling models (several GB on first run)..."
    foreach ($model in @($TextModel, $VisionModel, $EmbedModel)) {
        Log "  -> ollama pull $model"
        ollama pull $model
    }
}

# ------------------------------------------------------------------ verify

# Ollama's /api/tags reports untagged pulls with an implicit ":latest" (a
# pulled "nomic-embed-text" shows up as "nomic-embed-text:latest"), so add the
# suffix to any bare name before comparing. Getting this wrong would make the
# check cry wolf, which is worse than not having it.
function Normalize-ModelName($name) {
    if ($name -like "*:*") { return $name }
    return "${name}:latest"
}

# Confirm every model this script was configured to pull is actually present.
# $fatal throws, naming what is missing and the exact pull command to run; the
# -NoModels path only warns, since the user opted out of downloading.
function Confirm-Models($fatal, $models) {
    try {
        $tags = Invoke-RestMethod -Uri "$OllamaUrl/api/tags" -TimeoutSec 10
    } catch {
        $msg = "Could not reach $OllamaUrl/api/tags to verify models. Check 'ollama serve' and re-run."
        if ($fatal) { throw $msg }
        Warn "$msg Skipping verification."
        return
    }
    # A reachable server with an empty library returns {"models":[]}: a
    # different problem from an unreachable one, with a different fix.
    $installed = @()
    if ($tags.models) { $installed = @($tags.models | ForEach-Object { Normalize-ModelName $_.name }) }
    if ($installed.Count -eq 0) { Warn "$OllamaUrl reports no installed models at all." }

    $missing = @($models | Where-Object { $installed -notcontains (Normalize-ModelName $_) })
    if ($missing.Count -eq 0) {
        Log "Verified all configured models are present: $($models -join ' ')"
        return
    }
    foreach ($m in $missing) { Warn "  missing: $m  (run: ollama pull $m)" }
    if ($fatal) {
        throw "Model verification failed after pulling - see the missing models above."
    }
    Warn "-NoModels was used, so this is not fatal."
}

Confirm-Models (-not $NoModels) @($TextModel, $VisionModel, $EmbedModel)

Log "Done. Career Agent Core will use Ollama automatically (LLM_PROVIDER defaults to 'ollama')."
