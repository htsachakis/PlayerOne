<#
.SYNOPSIS
    Builds PlayerOne and assembles a self-contained portable folder.

.DESCRIPTION
    `wails build` produces PlayerOne.exe on its own, which finds mpv only on
    PATH. This script builds it and then copies bin/ alongside, producing a
    folder that runs anywhere without anything installed.

    Pass -Installer to also build the NSIS installer, which needs NSIS on PATH
    (winget install NSIS.NSIS).

.EXAMPLE
    pwsh scripts/package.ps1

.EXAMPLE
    pwsh scripts/package.ps1 -Installer -Zip
#>
[CmdletBinding()]
param(
    [switch]$Installer,
    [switch]$Zip,
    [string]$Version = '1.0.0'
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    function Write-Step($message) {
        Write-Host "==> $message" -ForegroundColor Cyan
    }

    if (-not (Test-Path 'bin/mpv.exe')) {
        throw 'bin/mpv.exe is missing. Run scripts/fetch-tools.ps1 first.'
    }

    Write-Step 'Building'

    # Compiled in so the running application can report its version, and so the
    # updater has something to compare against.
    $commit = ''
    try { $commit = (git rev-parse HEAD 2>$null) } catch { }
    $ldflags = @(
        "-X playerone/internal/version.Version=$Version"
        "-X playerone/internal/version.Commit=$commit"
        "-X playerone/internal/version.Date=$(Get-Date -Format 'yyyy-MM-dd')"
    ) -join ' '

    $buildArgs = @('build', '-platform', 'windows/amd64', '-clean', '-ldflags', $ldflags)

    # wails.json carries the version the installer reports to Windows, in Add or
    # Remove Programs and in the executable's file properties. It is stamped for
    # the build and put back afterwards, so the number cannot drift from the one
    # compiled in and the working tree is left clean either way.
    $wailsJson = 'wails.json'
    $originalWails = Get-Content $wailsJson -Raw
    (ConvertFrom-Json $originalWails) | ForEach-Object {
        $_.info.productVersion = $Version
        $_ | ConvertTo-Json -Depth 10 | Set-Content $wailsJson -Encoding utf8
    }
    if ($Installer) {
        if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
            # The Windows installer for NSIS does not add itself to PATH.
            $nsis = 'C:\Program Files (x86)\NSIS'
            if (Test-Path $nsis) {
                $env:PATH = "$nsis;$env:PATH"
            } else {
                throw 'NSIS was not found. Install it with: winget install NSIS.NSIS'
            }
        }
        $buildArgs += '-nsis'
    }

    try {
        & wails @buildArgs
        if ($LASTEXITCODE -ne 0) { throw 'The build failed.' }
    }
    finally {
        # Restored even when the build fails, so a failure never leaves the
        # repository with a modified wails.json.
        Set-Content $wailsJson -Value $originalWails -NoNewline -Encoding utf8
    }

    $stage = "dist/PlayerOne-$Version-windows-amd64"
    Write-Step "Assembling $stage"

    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path "$stage/bin" | Out-Null

    Copy-Item build/bin/PlayerOne.exe $stage
    Copy-Item README.md $stage
    Copy-Item LICENSE $stage
    Copy-Item NOTICE $stage
    Copy-Item docs/third-party.md "$stage/THIRD-PARTY.md"

    # The engine travels with the executable: PlayerOne looks in bin/ beside
    # itself before falling back to PATH.
    foreach ($tool in 'mpv.exe', 'ffmpeg.exe', 'ffprobe.exe', 'd3dcompiler_43.dll') {
        $source = "bin/$tool"
        if (Test-Path $source) { Copy-Item $source "$stage/bin" }
    }

    if ($Zip) {
        $archive = "dist/PlayerOne-$Version-windows-amd64-portable.zip"
        Write-Step "Compressing to $archive"
        if (Test-Path $archive) { Remove-Item $archive }
        Compress-Archive -Path "$stage/*" -DestinationPath $archive
    }

    if ($Installer) {
        $built = Get-ChildItem build/bin -Filter '*installer.exe' -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($built) {
            Move-Item $built.FullName "dist/PlayerOne-$Version-windows-amd64-installer.exe" -Force
        }
    }

    Write-Step 'Done'
    Get-ChildItem dist -Recurse -File |
        Where-Object { $_.DirectoryName -eq (Resolve-Path 'dist').Path -or $_.Name -eq 'PlayerOne.exe' } |
        ForEach-Object { '{0,-58} {1,12:N0} bytes' -f $_.Name, $_.Length }

    Write-Host ''
    Write-Host "Run it: $stage\PlayerOne.exe" -ForegroundColor Green
}
finally {
    Pop-Location
}
