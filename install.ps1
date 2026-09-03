$ErrorActionPreference = "Stop"

$Repo = "AgusRdz/local-mind"
$InstallDir = if ($env:LOCAL_MIND_INSTALL_DIR) { $env:LOCAL_MIND_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\local-mind" }

$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) { "arm64" } else { "amd64" }
$Binary = "local-mind-windows-$Arch.exe"

# --- resolve version ---
if (-not $env:LOCAL_MIND_VERSION) {
    $Release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $env:LOCAL_MIND_VERSION = $Release.tag_name
}
if (-not $env:LOCAL_MIND_VERSION) { Write-Error "failed to determine latest version"; exit 1 }

$Version = $env:LOCAL_MIND_VERSION
$Url = "https://github.com/$Repo/releases/download/$Version/$Binary"
$ChecksumsUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"
$SigUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt.sig"

Write-Host "installing local-mind $Version (windows/$Arch)..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$Destination = Join-Path $InstallDir "local-mind.exe"
$Tmp = "$Destination.tmp"
Invoke-WebRequest -Uri $Url -OutFile $Tmp

# --- verify SHA256 ---
try {
    $Checksums = Invoke-RestMethod $ChecksumsUrl
} catch {
    Write-Error "failed to download checksums.txt: $_"; Remove-Item -Force $Tmp -ErrorAction SilentlyContinue; exit 1
}
$Line = $Checksums -split "`n" | Where-Object { $_ -match "\s$([regex]::Escape($Binary))$" } | Select-Object -First 1
if (-not $Line) { Write-Error "checksum not found for $Binary"; Remove-Item -Force $Tmp -ErrorAction SilentlyContinue; exit 1 }
$Expected = ($Line -split '\s+')[0].Trim().ToLower()
$Actual = (Get-FileHash -Algorithm SHA256 $Tmp).Hash.ToLower()
if ($Actual -ne $Expected) { Write-Error "checksum mismatch: expected $Expected, got $Actual"; Remove-Item -Force $Tmp -ErrorAction SilentlyContinue; exit 1 }

# --- optional Ed25519 signature verification ---
$PubKey = Join-Path $PSScriptRoot "public_key.pem"
if ((Test-Path $PubKey) -and (Get-Command openssl -ErrorAction SilentlyContinue)) {
    try {
        $Sig = (Invoke-RestMethod $SigUrl).Trim()
        $SumsFile = "$Tmp.sums"; $SigFile = "$Tmp.sig"
        [System.IO.File]::WriteAllText($SumsFile, $Checksums)
        [System.IO.File]::WriteAllBytes($SigFile, ($Sig -split '(..)' | Where-Object { $_ } | ForEach-Object { [Convert]::ToByte($_, 16) }))
        & openssl pkeyutl -verify -pubin -inkey $PubKey -rawin -in $SumsFile -sigfile $SigFile *> $null
        if ($LASTEXITCODE -eq 0) { Write-Host "signature verified" } else { Write-Warning "signature verification failed" }
        Remove-Item -Force $SumsFile, $SigFile -ErrorAction SilentlyContinue
    } catch { }
}

Move-Item -Force $Tmp $Destination
Write-Host "installed local-mind to $Destination"

# --- PATH wiring ---
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$CleanDir = $InstallDir.TrimEnd("\")
if (($UserPath -split ";" | ForEach-Object { $_.TrimEnd("\") }) -notcontains $CleanDir) {
    [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$UserPath", "User")
    Write-Host "added $InstallDir to PATH"
}
if (($env:PATH -split ";" | ForEach-Object { $_.TrimEnd("\") }) -notcontains $CleanDir) {
    $env:PATH = "$InstallDir;$env:PATH"
}

Write-Host ""
Write-Host "next steps:"
Write-Host "  local-mind init       # register the UserPromptSubmit hook"
Write-Host "  local-mind rebuild    # build the index from your notes"
