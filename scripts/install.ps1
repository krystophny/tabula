[CmdletBinding()]
param(
    [string]$Version,
    [switch]$Uninstall,
    [switch]$Yes,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoOwner = if ($env:SLOPSHELL_REPO_OWNER) { $env:SLOPSHELL_REPO_OWNER } else { "sloppy-org" }
$RepoName = if ($env:SLOPSHELL_REPO_NAME) { $env:SLOPSHELL_REPO_NAME } else { "slopshell" }
$ReleaseApiBase = if ($env:SLOPSHELL_RELEASE_API_BASE) { $env:SLOPSHELL_RELEASE_API_BASE } else { "https://api.github.com/repos/$RepoOwner/$RepoName/releases" }
$SkipBrowser = $env:SLOPSHELL_INSTALL_SKIP_BROWSER -eq "1"
$AssumeYes = $Yes.IsPresent -or ($env:SLOPSHELL_ASSUME_YES -eq "1")

$InstallRoot = if ($env:SLOPSHELL_INSTALL_ROOT) { $env:SLOPSHELL_INSTALL_ROOT } else { Join-Path $env:LOCALAPPDATA "slopshell" }
$BinaryPath = Join-Path $InstallRoot "slopshell.exe"
$DataRoot = Join-Path $InstallRoot "data"
$ProjectDir = Join-Path $DataRoot "project"
$WebDataDir = Join-Path $DataRoot "web-data"
$PiperRoot = Join-Path $DataRoot "piper-tts"
$PiperVenv = Join-Path $PiperRoot "venv"
$ModelDir = Join-Path $PiperRoot "models"
$ScriptDir = Join-Path $DataRoot "scripts"
$PiperScriptPath = Join-Path $ScriptDir "piper_tts_server.py"
$LlmDir = Join-Path $DataRoot "llm"
$LlmModelDir = Join-Path $LlmDir "models"
$LlmSetupScript = Join-Path $ScriptDir "setup-local-llm.sh"
$SkipLlm = $env:SLOPSHELL_INSTALL_SKIP_LLM -eq "1"
$WebHost = if ($env:SLOPSHELL_WEB_HOST) { $env:SLOPSHELL_WEB_HOST } else { "127.0.0.1" }

function Write-Log {
    param([string]$Message)
    Write-Host "[slopshell-install] $Message"
}

function Throw-InstallError {
    param([string]$Message)
    throw "[slopshell-install] ERROR: $Message"
}

function Invoke-Step {
    param([ScriptBlock]$Action, [string]$Display)
    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] $Display"
        return
    }
    & $Action
}

function Confirm-DefaultYes {
    param([string]$Prompt)
    if ($AssumeYes) {
        Write-Log "SLOPSHELL_ASSUME_YES accepted: $Prompt"
        return $true
    }
    if (-not [Environment]::UserInteractive) {
        Write-Log "non-interactive session defaults to yes: $Prompt"
        return $true
    }
    $answer = Read-Host "$Prompt [Y/n]"
    return [string]::IsNullOrWhiteSpace($answer) -or ($answer -match '^(?i)y(es)?$')
}

function Normalize-Version {
    param([string]$Raw)
    if (-not $Raw) { return "" }
    $clean = $Raw.TrimStart('v', 'V')
    return "v$clean"
}

function Resolve-Arch {
    if ($env:PROCESSOR_ARCHITECTURE -match 'ARM64') { return "arm64" }
    return "amd64"
}

function Require-Codex {
    $cmd = Get-Command codex -ErrorAction SilentlyContinue
    if (-not $cmd) {
        Throw-InstallError "codex app-server is required but codex is not in PATH"
    }
    return $cmd.Source
}

function Require-Python {
    $python = Get-Command python -ErrorAction SilentlyContinue
    if (-not $python) {
        $python = Get-Command py -ErrorAction SilentlyContinue
    }
    if (-not $python) {
        Throw-InstallError "Python 3.10+ is required"
    }

    $versionOutput = & $python.Source -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')"
    if (-not $versionOutput) {
        Throw-InstallError "unable to detect Python version"
    }
    $parts = $versionOutput.Trim().Split('.')
    $major = [int]$parts[0]
    $minor = [int]$parts[1]
    if ($major -lt 3 -or ($major -eq 3 -and $minor -lt 10)) {
        Throw-InstallError "Python 3.10+ is required"
    }
    return $python.Source
}

function Get-Release {
    param([string]$Requested)

    if ($env:SLOPSHELL_RELEASE_JSON) {
        return ($env:SLOPSHELL_RELEASE_JSON | ConvertFrom-Json)
    }
    if ($DryRun.IsPresent) {
        $tag = if ($Requested) { Normalize-Version $Requested } else { "v0.0.0-test" }
        $plain = $tag.TrimStart('v')
        $arch = Resolve-Arch
        $json = @"
{
  "tag_name": "$tag",
  "assets": [
    {"name":"slopshell_${plain}_windows_${arch}.zip","browser_download_url":"https://example.invalid/slopshell.zip"},
    {"name":"checksums.txt","browser_download_url":"https://example.invalid/checksums.txt"}
  ]
}
"@
        return ($json | ConvertFrom-Json)
    }

    $url = if ($Requested) {
        "$ReleaseApiBase/tags/$(Normalize-Version $Requested)"
    } else {
        "$ReleaseApiBase/latest"
    }
    return Invoke-RestMethod -Uri $url -Headers @{"Accept"="application/vnd.github+json"}
}

function Get-Asset {
    param($Release, [string]$Name)
    $asset = $Release.assets | Where-Object { $_.name -eq $Name } | Select-Object -First 1
    if (-not $asset) {
        Throw-InstallError "release missing asset $Name"
    }
    return $asset
}

function Ensure-InstallDirectories {
    Invoke-Step -Display "Create install directories" -Action {
        New-Item -ItemType Directory -Force -Path $InstallRoot, $DataRoot, $ProjectDir, $WebDataDir, $PiperRoot, $ModelDir, $ScriptDir, $LlmDir, $LlmModelDir | Out-Null
    }
}

function Install-Binary {
    param($Release)

    $tag = $Release.tag_name
    if (-not $tag) { Throw-InstallError "release did not provide tag_name" }
    $plainVersion = $tag.TrimStart('v')
    $arch = Resolve-Arch
    $assetName = "slopshell_${plainVersion}_windows_${arch}.zip"
    $asset = Get-Asset -Release $Release -Name $assetName

    if ($DryRun.IsPresent) {
        Invoke-Step -Display "Install slopshell.exe to $BinaryPath" -Action {}
        return $tag
    }

    $checksumAsset = Get-Asset -Release $Release -Name "checksums.txt"
    $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("slopshell-install-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmpDir | Out-Null
    try {
        $zipPath = Join-Path $tmpDir $assetName
        $checksumPath = Join-Path $tmpDir "checksums.txt"
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zipPath
        Invoke-WebRequest -Uri $checksumAsset.browser_download_url -OutFile $checksumPath

        $expected = Select-String -Path $checksumPath -Pattern ("\s$([regex]::Escape($assetName))$") | ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1
        if (-not $expected) {
            Throw-InstallError "checksum entry missing for $assetName"
        }
        $actual = (Get-FileHash -Algorithm SHA256 -Path $zipPath).Hash.ToLowerInvariant()
        if ($actual -ne $expected.ToLowerInvariant()) {
            Throw-InstallError "checksum mismatch for $assetName"
        }

        $extractDir = Join-Path $tmpDir "extract"
        Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force
        $exeSource = Get-ChildItem -Path $extractDir -Filter "slopshell.exe" -Recurse | Select-Object -First 1
        if (-not $exeSource) {
            Throw-InstallError "slopshell.exe missing in release archive"
        }
        Copy-Item -Path $exeSource.FullName -Destination $BinaryPath -Force

        $piperSource = Get-ChildItem -Path $extractDir -Filter "piper_tts_server.py" -Recurse | Select-Object -First 1
        if (-not $piperSource) {
            Throw-InstallError "scripts/piper_tts_server.py missing in release archive"
        }
        Copy-Item -Path $piperSource.FullName -Destination $PiperScriptPath -Force
    }
    finally {
        Remove-Item -Recurse -Force -Path $tmpDir -ErrorAction SilentlyContinue
    }

    return $tag
}

function Ensure-UserPath {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @()
    if ($userPath) {
        $parts = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }
    if ($parts -contains $InstallRoot) {
        return
    }
    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] Add $InstallRoot to user PATH"
        return
    }
    $newPath = (($parts + $InstallRoot) -join ';')
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Log "added $InstallRoot to user PATH"
}

function Setup-Piper {
    Write-Host "=== Piper TTS (GPL, runs as HTTP sidecar) ==="
    Write-Host "Piper TTS will be installed as a local HTTP service."
    Write-Host "License: GPL (isolated via HTTP boundary, does not affect Slopshell MIT license)"
    Write-Host "Voice models: en_GB-alan-medium (MIT-compatible)"

    if (-not (Confirm-DefaultYes "Install Piper TTS?")) {
        Write-Log "skipping Piper TTS setup"
        return
    }

    if ($DryRun.IsPresent) {
        Invoke-Step -Display "Create Piper venv and install piper-tts fastapi uvicorn" -Action {}
        Invoke-Step -Display "Download Piper voice model en_GB-alan-medium" -Action {}
        return
    }

    $python = Require-Python
    if (-not (Test-Path $PiperVenv)) {
        & $python -m venv $PiperVenv
    }
    $venvPython = Join-Path $PiperVenv "Scripts\python.exe"
    & $venvPython -m pip install --upgrade pip
    & $venvPython -m pip install piper-tts fastapi uvicorn

    $onnx = Join-Path $ModelDir "en_GB-alan-medium.onnx"
    $json = Join-Path $ModelDir "en_GB-alan-medium.onnx.json"
    if (-not (Test-Path $onnx)) {
        Write-Log "model card: https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/alan/medium/MODEL_CARD"
        Invoke-WebRequest -Uri "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/alan/medium/en_GB-alan-medium.onnx" -OutFile $onnx
        Invoke-WebRequest -Uri "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/alan/medium/en_GB-alan-medium.onnx.json" -OutFile $json
    }
}

function Setup-LocalLlm {
    if ($SkipLlm) {
        Write-Log "skipping local LLM due to SLOPSHELL_INSTALL_SKIP_LLM=1"
        return
    }
    Write-Host "=== Local LLMs (llama.cpp, optional) ==="
    Write-Host "A fast Qwen3.5 9B coordinator runs on port 8081 for Slopshell routing and replies."
    Write-Host "A Codex-focused gpt-oss-120b runtime runs on port 8080 for local Codex agent profiles."
    Write-Host "Requires llama.cpp (llama-server binary)."

    if (-not (Confirm-DefaultYes "Install local LLM service?")) {
        Write-Log "skipping local LLM setup"
        return
    }

    $llamaCmd = Get-Command llama-server -ErrorAction SilentlyContinue
    if (-not $llamaCmd) {
        $wingetCmd = Get-Command winget -ErrorAction SilentlyContinue
        if ($wingetCmd -and (Confirm-DefaultYes "Install llama.cpp via winget?")) {
            if ($DryRun.IsPresent) {
                Invoke-Step -Display "winget install ggml-org.llama-cpp" -Action {}
            } else {
                winget install --id ggml-org.llama-cpp --accept-source-agreements --accept-package-agreements
            }
        } else {
            Write-Log "llama-server not found; install llama.cpp and ensure llama-server is on PATH"
        }
    }

    $modelFile = "Qwen3.5-9B-Q4_K_M.gguf"
    $modelPath = Join-Path $LlmModelDir $modelFile
    if (Test-Path $modelPath) {
        Write-Log "LLM model already present: $modelFile"
    } elseif (Confirm-DefaultYes "Download Qwen3.5 9B GGUF model (~5.3 GB)?") {
        if ($DryRun.IsPresent) {
            Invoke-Step -Display "Download $modelFile" -Action {}
        } else {
            $modelUrl = "https://huggingface.co/lmstudio-community/Qwen3.5-9B-GGUF/resolve/main/Qwen3.5-9B-Q4_K_M.gguf?download=true"
            Invoke-WebRequest -Uri $modelUrl -OutFile $modelPath
        }
    }
}

function Write-TaskFiles {
    param([string]$CodexPath)

    $webCmd = 'set "SLOPSHELL_INTENT_LLM_URL=http://127.0.0.1:8081" && set "SLOPSHELL_INTENT_LLM_MODEL=local" && set "SLOPSHELL_INTENT_LLM_PROFILE=qwen3.5-9b" && set "SLOPSHELL_INTENT_LLM_PROFILE_OPTIONS=qwen3.5-9b,qwen3.5-4b" && "' + $BinaryPath + '" server --workspace-dir "' + $ProjectDir + '" --data-dir "' + $WebDataDir + '" --web-host ' + $WebHost + ' --web-port 8420 --app-server-url ws://127.0.0.1:8787 --tts-url http://127.0.0.1:8424'
    $piperCmd = 'set "PIPER_MODEL_DIR=' + $ModelDir + '" && "' + (Join-Path $PiperVenv 'Scripts\python.exe') + '" -m uvicorn piper_tts_server:app --app-dir "' + $ScriptDir + '" --host 127.0.0.1 --port 8424'
    $codexCmd = '"' + $CodexPath + '" app-server --listen ws://127.0.0.1:8787'

    $llamaPath = (Get-Command llama-server -ErrorAction SilentlyContinue)
    $llmCmd = ""
    $codexLlmCmd = ""
    if ($llamaPath) {
        $llmCmd = '"' + $llamaPath.Source + '" -m "' + (Join-Path $LlmModelDir "Qwen3.5-9B-Q4_K_M.gguf") + '" --host 127.0.0.1 --port 8081 -c 131072 --threads 4 -ngl 99 --parallel 1 --alias qwen3.5-9b --reasoning-budget -1 --jinja'
        $codexLlmCmd = '"' + $llamaPath.Source + '" --gpt-oss-120b-default --host 127.0.0.1 --port 8080 -c 32768 --threads 8 -ngl auto --parallel 1 --alias gpt-oss-120b --reasoning-budget -1 --jinja'
    }

    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] Register scheduled tasks slopshell-web, slopshell-piper-tts, slopshell-codex-app-server, slopshell-llm, slopshell-codex-llm"
        return
    }

    schtasks /Create /SC ONLOGON /TN "slopshell-codex-app-server" /TR $codexCmd /F | Out-Null
    schtasks /Create /SC ONLOGON /TN "slopshell-piper-tts" /TR ("cmd /c " + $piperCmd) /F | Out-Null
    schtasks /Run /TN "slopshell-codex-app-server" | Out-Null
    schtasks /Run /TN "slopshell-piper-tts" | Out-Null

    if ($llmCmd) {
        schtasks /Create /SC ONLOGON /TN "slopshell-llm" /TR $llmCmd /F | Out-Null
        schtasks /Run /TN "slopshell-llm" | Out-Null
    }
    if ($codexLlmCmd) {
        schtasks /Create /SC ONLOGON /TN "slopshell-codex-llm" /TR $codexLlmCmd /F | Out-Null
        schtasks /Run /TN "slopshell-codex-llm" | Out-Null
    }

    schtasks /Create /SC ONLOGON /TN "slopshell-web" /TR $webCmd /F | Out-Null
    schtasks /Run /TN "slopshell-web" | Out-Null
}

function Print-WindowsSTTNotice {
    Write-Log "Speech-to-text requires voxtype (Linux/macOS only)"
}

function Open-Browser {
    if ($SkipBrowser) {
        Write-Log "skipping browser open due to SLOPSHELL_INSTALL_SKIP_BROWSER=1"
        return
    }
    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] Start browser at http://127.0.0.1:8420"
        return
    }
    Start-Process "http://127.0.0.1:8420" | Out-Null
}

function Print-Summary {
    param([string]$Tag)
    Write-Host ""
    Write-Host "Install complete"
    Write-Host "  Version:      $Tag"
    Write-Host "  Binary:       $BinaryPath"
    Write-Host "  Data root:    $DataRoot"
    Write-Host "  Project dir:  $ProjectDir"
    Write-Host "  Piper models: $ModelDir"
    Write-Host "  Web URL:      http://127.0.0.1:8420"
    Write-Host "  Intent LLM:   http://127.0.0.1:8081"
    Write-Host "  Codex LLM:    http://127.0.0.1:8080"
}

function Remove-Task {
    param([string]$TaskName)
    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] Delete scheduled task $TaskName"
        return
    }
    schtasks /Delete /TN $TaskName /F | Out-Null 2>$null
}

function Uninstall-Slopshell {
    Remove-Task "slopshell-web"
    Remove-Task "slopshell-llm"
    Remove-Task "slopshell-codex-llm"
    Remove-Task "slopshell-piper-tts"
    Remove-Task "slopshell-codex-app-server"

    if ($DryRun.IsPresent) {
        Write-Log "[dry-run] Remove $BinaryPath"
    } else {
        Remove-Item -Force -ErrorAction SilentlyContinue -Path $BinaryPath
    }

    if (Confirm-DefaultYes "Remove $DataRoot data directory?") {
        if ($DryRun.IsPresent) {
            Write-Log "[dry-run] Remove $DataRoot"
        } else {
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue -Path $DataRoot
        }
    }

    Write-Log "uninstall complete"
}

function Install-Slopshell {
    $codexPath = Require-Codex
    Require-Python | Out-Null
    Ensure-InstallDirectories
    $release = Get-Release -Requested $Version
    $tag = Install-Binary -Release $release
    Ensure-UserPath
    Setup-Piper
    Setup-LocalLlm
    Print-WindowsSTTNotice
    Write-TaskFiles -CodexPath $codexPath
    Open-Browser
    Print-Summary -Tag $tag
}

if ($Uninstall.IsPresent) {
    Uninstall-Slopshell
} else {
    Install-Slopshell
}
