param(
    [ValidateSet('amd64','386')]
    [string]$Arch,
    [switch]$Console
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$repo = Split-Path -Parent $root

if (-not $Arch -or $Arch -eq '') {
    if ($env:GOARCH -and $env:GOARCH -ne '') {
        $Arch = $env:GOARCH
    } else {
        $Arch = 'amd64'
    }
}

# Resolve VLC SDK paths bundled in the repo
if ($Arch -eq '386') {
    $vlcBase = Join-Path $repo 'third_party\vlc\nupkg\build\x86'
} else {
    $vlcBase = Join-Path $repo 'third_party\vlc\nupkg\build\x64'
}
$include = Join-Path $vlcBase 'include'

# Validate presence
if (!(Test-Path (Join-Path $include 'vlc\vlc.h'))) {
    Write-Error "VLC headers not found at: $include. Expected vlc/vlc.h."
}
if (!(Test-Path (Join-Path $vlcBase 'libvlc.lib'))) {
    Write-Error "VLC import library not found: $vlcBase\\libvlc.lib"
}

# Configure environment for libvlc-go cgo build
$env:CGO_ENABLED = '1'
$env:GOOS = 'windows'
$env:GOARCH = $Arch

# Paths for libvlc-go autodetection
$env:LIBVLC_INCLUDE_PATH = $include
$env:LIBVLC_LIB_PATH = $vlcBase

# Explicit CGO flags (more reliable on Windows)
$env:CGO_CFLAGS = "-I$include"
$env:CGO_LDFLAGS = "-L$vlcBase -lvlc"

# Add VLC dir to PATH for linker/runtime resolution
$env:Path = "$vlcBase;" + $env:Path

# Compose build command
$ldflags = @()
if (-not $Console) {
    $ldflags = @('-ldflags','-H=windowsgui')
}

$exe = Join-Path $repo 'miniradio.exe'

Write-Host "Building MiniRadio for $env:GOOS/$env:GOARCH using VLC SDK at $vlcBase"

Push-Location $repo
try {
    # Tidy modules just in case
    go mod tidy

    # Build
    if ($Console) {
        go build -v -o $exe ./cmd/miniradio
    } else {
        go build -v @ldflags -o $exe ./cmd/miniradio
    }

    $iconSource = Join-Path $repo 'images\radio64.ico'
    if (Test-Path $iconSource) {
        go run ./cmd/seticon -exe $exe -icon $iconSource
    } else {
        Write-Warning "Icon not found at $iconSource; executable will use the default icon."
    }
}
finally {
    Pop-Location
}

# Copy runtime DLLs next to the exe for convenience
$dlls = @('libvlc.dll','libvlccore.dll')
foreach ($dll in $dlls) {
    $src = Join-Path $vlcBase $dll
    if (Test-Path $src) {
        Copy-Item $src -Destination $repo -Force
    }
}

# Copy a minimal set of VLC plugins next to the exe for runtime (audio-only)
$pluginsSrc = Join-Path (Join-Path $repo 'third_party\vlc\vlc-3.0.20') 'plugins'
if (Test-Path $pluginsSrc) {
    $pluginsDst = Join-Path $repo 'plugins'
    if (Test-Path $pluginsDst) { Remove-Item $pluginsDst -Recurse -Force }
    New-Item -ItemType Directory -Path $pluginsDst | Out-Null

    # Whitelist only the plugin categories required for MiniRadio audio playback
    # access         : http/https access modules
    # stream_filter  : helpers used by access/http
    # demux          : mp3/aac/flac/ogg demuxers
    # codec          : decoders (mp3, aac/aacp, flac, opus/vorbis)
    # packetizer     : packetizers used by demuxers/codecs
    # audio_output   : Windows audio backends (WASAPI/DirectSound)
    # audio_filter   : equalizer and basic audio filters
    # playlist       : .m3u/.m3u8/.pls resolution
    # misc           : misc helpers (kept for safety)
    # logger         : logger interface plugin (enables --extraintf=logger / --file-logging)
    $pluginWhitelist = @('access','stream_filter','demux','codec','packetizer','audio_output','audio_filter','playlist','misc','logger')
    $copied = @()

    foreach ($dir in $pluginWhitelist) {
        $srcDir = Join-Path $pluginsSrc $dir
        if (Test-Path $srcDir) {
            $dstDir = Join-Path $pluginsDst $dir
            Copy-Item $srcDir -Destination $dstDir -Recurse -Force
            $copied += $dir
        }
    }

    if ($copied.Count -gt 0) {
        Write-Host ("Plugins copied (whitelist): " + ($copied -join ', '))
        Write-Host "Destination: $pluginsDst"
    } else {
        Write-Warning "No whitelisted plugin folders were found under $pluginsSrc"
    }
} else {
    Write-Warning "VLC plugins directory not found: $pluginsSrc"
}

Write-Host "Build finished: $exe"
