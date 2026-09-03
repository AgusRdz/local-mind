$ErrorActionPreference = "Stop"

# local-mind installer (Windows PowerShell).
#
# local-mind is a PRIVATE repo, so release assets require authentication.
# Prefers the GitHub CLI (`gh`, using your existing login) and falls back to
# Invoke-WebRequest with a token from $env:GITHUB_TOKEN / $env:GH_TOKEN.

$Repo = "AgusRdz/local-mind"
$InstallDir = if ($env:LOCAL_MIND_INSTALL_DIR) { $env:LOCAL_MIND_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\local-mind" }

$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) { "arm64" } else { "amd64" }
$Binary = "local-mind-windows-$Arch.exe"

function Have-Gh {
    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { return $false }
    gh auth status 2>$null | Out-Null
    return ($LASTEXITCODE -eq 0)
}

$UseGh = Have-Gh
$Token = if ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } else { $env:GH_TOKEN }

# --- resolve version ---
$Version = $env:LOCAL_MIND_VERSION
if (-not $Version) {
    if ($UseGh) {
        $Version = (gh release view --repo $Repo --json tagName -q .tagName 2>$null)
    } elseif ($Token) {
        $rel = Invoke-RestMethod -Headers @{ Authorization = "Bearer $Token" } "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $rel.tag_name
    } else {
        Write-Error "need gh CLI logged in, or `$env:GITHUB_TOKEN set (private repo)"; exit 1
    }
}
if (-not $Version) { Write-Error "failed to determine latest version"; exit 1 }

Write-Host "installing local-mind $Version (windows/$Arch)..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Work = Join-Path ([System.IO.Path]::GetTempPath()) ("lm-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $Work | Out-Null

try {
    if ($UseGh) {
        gh release download $Version --repo $Repo --dir $Work --pattern $Binary --pattern "checksums.txt" --pattern "checksums.txt.sig" 2>$null
        if ($LASTEXITCODE -ne 0) {
            gh release download $Version --repo $Repo --dir $Work --pattern $Binary --pattern "checksums.txt"
        }
    } else {
        $base = "https://api.github.com/repos/$Repo/releases"
        $tagRel = Invoke-RestMethod -Headers @{ Authorization = "Bearer $Token" } "$base/tags/$Version"
        function Get-Asset($name) {
            $asset = $tagRel.assets | Where-Object { $_.name -eq $name } | Select-Object -First 1
            if (-not $asset) { return $false }
            Invoke-WebRequest -Headers @{ Authorization = "Bearer $Token"; Accept = "application/octet-stream" } `
                -Uri $asset.url -OutFile (Join-Path $Work $name)
            return $true
        }
        if (-not (Get-Asset $Binary)) { Write-Error "failed to download $Binary"; exit 1 }
        if (-not (Get-Asset "checksums.txt")) { Write-Error "failed to download checksums.txt"; exit 1 }
        Get-Asset "checksums.txt.sig" | Out-Null
    }

    $BinPath = Join-Path $Work $Binary
    if (-not (Test-Path $BinPath)) { Write-Error "binary not found after download"; exit 1 }

    # --- verify SHA256 ---
    $checks = Get-Content (Join-Path $Work "checksums.txt")
    $line = $checks | Where-Object { $_ -match "\s$([regex]::Escape($Binary))$" } | Select-Object -First 1
    if (-not $line) { Write-Error "checksum not found for $Binary"; exit 1 }
    $Expected = ($line -split '\s+')[0].Trim().ToLower()
    $Actual = (Get-FileHash -Algorithm SHA256 $BinPath).Hash.ToLower()
    if ($Actual -ne $Expected) { Write-Error "checksum mismatch: expected $Expected, got $Actual"; exit 1 }

    # --- optional signature verification ---
    $sigFile = Join-Path $Work "checksums.txt.sig"
    $pubKey = Join-Path $PSScriptRoot "public_key.pem"
    if ((Test-Path $sigFile) -and (Test-Path $pubKey) -and (Get-Command openssl -ErrorAction SilentlyContinue)) {
        $sigBin = Join-Path $Work "checksums.txt.sig.bin"
        $hex = (Get-Content $sigFile -Raw).Trim()
        [System.IO.File]::WriteAllBytes($sigBin, ($hex -split '(..)' | Where-Object { $_ } | ForEach-Object { [Convert]::ToByte($_, 16) }))
        & openssl pkeyutl -verify -pubin -inkey $pubKey -rawin -in (Join-Path $Work "checksums.txt") -sigfile $sigBin *> $null
        if ($LASTEXITCODE -eq 0) { Write-Host "signature verified" } else { Write-Warning "signature verification failed" }
    }

    $Destination = Join-Path $InstallDir "local-mind.exe"
    Move-Item -Force $BinPath $Destination
    Write-Host "installed local-mind to $Destination"
} finally {
    Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}

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
