# install_ollama.ps1 — Install Ollama on Windows and pull the models Career Agent Core needs.
#
# Usage (PowerShell):
#   .\scripts\install_ollama.ps1              # install + pull models
#   .\scripts\install_ollama.ps1 -NoModels    # install only
#
# Models are overridable via the same env vars the agent reads:
#   OLLAMA_MODEL (llama3.1), OLLAMA_VISION_MODEL (llava), OLLAMA_EMBED_MODEL (nomic-embed-text)

param(
    [switch]$NoModels
)

$ErrorActionPreference = "Stop"

$TextModel   = if ($env:OLLAMA_MODEL)        { $env:OLLAMA_MODEL }        else { "llama3.1" }
$VisionModel = if ($env:OLLAMA_VISION_MODEL) { $env:OLLAMA_VISION_MODEL } else { "llava" }
$EmbedModel  = if ($env:OLLAMA_EMBED_MODEL)  { $env:OLLAMA_EMBED_MODEL }  else { "nomic-embed-text" }

function Log($msg) { Write-Host "[install] $msg" -ForegroundColor Cyan }

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
        Invoke-RestMethod -Uri "http://localhost:11434/api/version" -TimeoutSec 2 | Out-Null
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

Log "Done. Career Agent Core will use Ollama automatically (LLM_PROVIDER defaults to 'ollama')."
