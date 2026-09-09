<#
.SYNOPSIS
    Downloads mpv and ffmpeg into PlayerOne's bin folder.

.DESCRIPTION
    PlayerOne drives mpv for playback and ffmpeg/ffprobe for subtitle extraction
    and file details. It looks for them in bin/ before falling back to PATH, so
    running this script once gives a self-contained development checkout.

    The same script runs in CI, so a release ships exactly the binaries a
    developer tested against.

    Nothing here is committed: bin/ is gitignored, because these are large
    third-party binaries with their own licences (see docs/third-party.md).

.PARAMETER Force
    Re-download even if the tools are already present.

.EXAMPLE
    pwsh scripts/fetch-tools.ps1
#>
[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root 'bin'
$work = Join-Path ([System.IO.Path]::GetTempPath()) "playerone-tools-$(Get-Random)"

New-Item -ItemType Directory -Force -Path $bin | Out-Null
New-Item -ItemType Directory -Force -Path $work | Out-Null

function Write-Step($message) {
    Write-Host "==> $message" -ForegroundColor Cyan
}

# 7-Zip is needed because the official mpv builds are distributed as .7z.
function Resolve-SevenZip {
    $candidates = @(
        'C:\Program Files\7-Zip\7z.exe',
        'C:\Program Files (x86)\7-Zip\7z.exe'
    )
    foreach ($path in $candidates) {
        if (Test-Path $path) { return $path }
    }

    $command = Get-Command 7z -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }

    throw '7-Zip is required to unpack the mpv build. Install it (winget install 7zip.7zip) and run this script again.'
}

function Get-LatestAsset($repo, $pattern) {
    $headers = @{ 'User-Agent' = 'PlayerOne-build' }

    # CI sets GITHUB_TOKEN. Using it lifts the anonymous API rate limit, which a
    # busy shared runner can otherwise be sitting against already.
    if ($env:GITHUB_TOKEN) {
        $headers['Authorization'] = "Bearer $env:GITHUB_TOKEN"
    }

    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $headers

    $asset = $release.assets | Where-Object { $_.name -like $pattern } | Select-Object -First 1
    if (-not $asset) {
        throw "No asset matching '$pattern' in the latest release of $repo."
    }
    return $asset
}

# --- mpv ---------------------------------------------------------------------

if ($Force -or -not (Test-Path (Join-Path $bin 'mpv.exe'))) {
    Write-Step 'Fetching mpv (shinchiro Windows build, the one mpv.io links to)'

    $asset = Get-LatestAsset 'shinchiro/mpv-winbuild-cmake' 'mpv-x86_64-*.7z'
    $archive = Join-Path $work 'mpv.7z'

    Write-Host "    $($asset.name)"
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive

    $sevenZip = Resolve-SevenZip
    & $sevenZip e -y -o"$bin" $archive mpv.exe mpv.com d3dcompiler_43.dll | Out-Null
    & $sevenZip e -y -o"$(Join-Path $bin 'mpv')" $archive 'mpv/fonts.conf' | Out-Null

    if ($LASTEXITCODE -ne 0) { throw 'Unpacking mpv failed.' }
} else {
    Write-Step 'mpv is already present (use -Force to re-download)'
}

# --- ffmpeg ------------------------------------------------------------------

if ($Force -or -not (Test-Path (Join-Path $bin 'ffmpeg.exe'))) {
    Write-Step 'Fetching ffmpeg and ffprobe'

    # BtbN's builds are the ones the FFmpeg project links to for Windows.
    $asset = Get-LatestAsset 'BtbN/FFmpeg-Builds' 'ffmpeg-master-latest-win64-gpl.zip'
    $archive = Join-Path $work 'ffmpeg.zip'

    Write-Host "    $($asset.name)"
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive

    $extracted = Join-Path $work 'ffmpeg'
    Expand-Archive -Path $archive -DestinationPath $extracted -Force

    foreach ($tool in 'ffmpeg.exe', 'ffprobe.exe') {
        $found = Get-ChildItem -Path $extracted -Filter $tool -Recurse | Select-Object -First 1
        if (-not $found) { throw "$tool was not found in the downloaded archive." }
        Copy-Item $found.FullName (Join-Path $bin $tool) -Force
    }
} else {
    Write-Step 'ffmpeg is already present (use -Force to re-download)'
}

Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue

Write-Step 'Done. bin/ now contains:'
Get-ChildItem $bin -File | ForEach-Object {
    '{0,-24} {1,10:N0} bytes' -f $_.Name, $_.Length
}

& (Join-Path $bin 'mpv.com') --version | Select-Object -First 1
