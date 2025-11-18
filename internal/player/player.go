// Package player wraps libVLC playback and auxiliary metadata watchers into a
// single serialised component tailored for the MiniRadio UI.
package player

import (
	"context"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	vlc "github.com/adrg/libvlc-go/v3"

	metadata "github.com/edward-ap/miniradio/internal/metadata"
)

const (
	// stableWindow smooths metadata updates by waiting for repeated values.
	stableWindow = 6 * time.Second
	// nearEndThreshold is the remaining duration that counts as "almost done".
	nearEndThreshold = 10 * time.Second
)

// Player is a thread-safe wrapper around libVLC that serializes every call,
// stabilizes "Now Playing" metadata, and manages optional ICY watchers.
type Player struct {
	p      *vlc.Player
	media  *vlc.Media
	volume int
	muted  bool

	stream    string
	isPlaying bool

	onNow     func(string)
	onStation func(string)

	// title stabilization state
	currentTitle     string
	pendingTitle     string
	firstSeenPending time.Time

	// single lock guarding all C/libVLC invocations
	vlcMu sync.Mutex

	// metadata polling management (VLC-safe ticker)
	metaCancel context.CancelFunc
	metaWG     sync.WaitGroup

	// ICY watcher (pure Go, no libVLC)
	icyCancel          context.CancelFunc
	icyWG              sync.WaitGroup
	metaProvider       metadata.Provider
	metadataHintType   string
	metadataHintURL    string
	onMetadataResolved func(string, string)

	// internal lock for Player fields (not for libVLC)
	mu sync.Mutex

	vlcMajor     int
	parseTimeout int // ms
}

// stdLogger adapts the standard log package to the metadata.Logger interface.
type stdLogger struct{}

func (stdLogger) Printf(format string, args ...any) {
	log.Printf(format, args...)
}

// NewPlayer constructs a Player with sane defaults but does not initialize
// libVLC. Call Init before attempting playback.
func NewPlayer() *Player {
	pm := 70
	return &Player{
		volume:       pm,
		parseTimeout: 4000,
		metaProvider: metadata.NewProvider(nil, stdLogger{}),
	}
}

// SetMetadataHint provides previously discovered metadata strategy information
// and a callback that fires when the ICY watcher discovers a better source.
func (pl *Player) SetMetadataHint(metaType, metaURL string, onResolved func(string, string)) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.metadataHintType = metaType
	pl.metadataHintURL = metaURL
	pl.onMetadataResolved = onResolved
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func parseVlcMajor(ver string) int {
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return 0
	}
	cut := ver
	if i := strings.IndexAny(ver, ". "); i >= 0 {
		cut = ver[:i]
	}
	m, _ := strconv.Atoi(cut)
	return m
}

// Init configures libVLC (plugin path, caching arguments) and applies the
// initial volume/mute state. It must be called before Load/Play.
func (pl *Player) Init(volume int, muted bool) error {
	// 1) Provide plugin path via ENV (without --plugin-path)
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		plugins := filepath.Join(base, "plugins")
		if st, err := os.Stat(plugins); err == nil && st.IsDir() {
			// let libVLC 3 load plugins from this location
			_ = os.Setenv("VLC_PLUGIN_PATH", plugins)
		}
	}

	// 2) Initialize libVLC (without --plugin-path)
	pl.vlcMu.Lock()
	args := []string{
		"--no-video",
		"--no-color",
		"--network-caching=1500", // 1.5s for HLS/stream buffering
		"--live-caching=1500",
		"--http-reconnect",
	}
	if traceLogEnabled.Load() {
		// Verbose/file logging only when explicitly enabled via CLI flag
		// Note: do not use --extraintf=logger (interface plugin) — file logging works with --file-logging alone.
		args = append(args,
			"--verbose=2",
			"--file-logging",
			"--log-verbose=2",
			"--logfile=vlc.log",
		)
	}
	err := vlc.Init(args...)
	pl.vlcMu.Unlock()
	if err != nil {
		return fmt.Errorf("libvlc init failed: %w", err)
	}

	verInfo := vlc.Version()
	ver := verInfo.String()
	pl.vlcMajor = parseVlcMajor(ver)

	pl.vlcMu.Lock()
	player, err := vlc.NewPlayer()
	pl.vlcMu.Unlock()
	if err != nil {
		pl.vlcMu.Lock()
		vlc.Release()
		pl.vlcMu.Unlock()
		return fmt.Errorf("new vlc player failed: %w", err)
	}
	pl.p = player

	// 3) Apply initial volume/mute
	pl.volume = clamp(volume, 0, 100)
	pl.vlcMu.Lock()
	_ = pl.p.SetVolume(pl.volume)
	if muted {
		pl.p.ToggleMute()
		pl.muted = true
	}
	pl.vlcMu.Unlock()

	return nil
}

// Release frees VLC resources and stops any background watchers/tickers.
func (pl *Player) Release() {
	// stop ICY watcher
	pl.stopICYWatcher()

	// stop metadata ticker (safe ticker)
	if pl.metaCancel != nil {
		pl.metaCancel()
		pl.metaWG.Wait()
		pl.metaCancel = nil
	}

	pl.vlcMu.Lock()
	if pl.p != nil {
		_ = pl.p.Stop()
		pl.p.Release()
		pl.p = nil
		pl.isPlaying = false
	}
	if pl.media != nil {
		pl.media.Release()
		pl.media = nil
	}
	vlc.Release()
	pl.vlcMu.Unlock()
}

// SetOnNow registers a callback that receives stabilized track titles.
func (pl *Player) SetOnNow(fn func(string)) {
	pl.mu.Lock()
	pl.onNow = fn
	pl.mu.Unlock()
}

// SetOnStation registers a callback fired when station names are discovered.
func (pl *Player) SetOnStation(fn func(string)) {
	pl.mu.Lock()
	pl.onStation = fn
	pl.mu.Unlock()
}

// IsPlaying reports whether libVLC currently plays audio.
func (pl *Player) IsPlaying() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.isPlaying
}

// SetAudioEqualizer applies the provided equalizer to the underlying VLC player in a thread-safe way.
func (pl *Player) SetAudioEqualizer(eq *vlc.Equalizer) error {
	pl.vlcMu.Lock()
	defer pl.vlcMu.Unlock()
	if pl.p == nil {
		return fmt.Errorf("vlc player not initialized")
	}
	return pl.p.SetEqualizer(eq)
}

// Load prepares the VLC media instance for the provided URL but does not start
// playback. It also resets metadata trackers and begins parsing the stream in
// the background so metadata reads are safe.
func (pl *Player) Load(ctx context.Context, url string) error {
	// if currently playing stop the previous ICY watcher before switching media
	if pl.IsPlaying() {
		pl.stopICYWatcher()
	}
	// reset title stabilization
	pl.currentTitle, pl.pendingTitle = "", ""
	pl.firstSeenPending = time.Time{}
	// stop previous metadata poller
	if pl.metaCancel != nil {
		pl.metaCancel()
		pl.metaWG.Wait()
		pl.metaCancel = nil
	}

	pl.vlcMu.Lock()
	// release previous media
	if pl.media != nil {
		pl.media.Release()
		pl.media = nil
	}

	// sanitize URL: trim spaces/CRLF/tabs that may sneak in from clipboard or inputs
	u := strings.TrimSpace(url)
	u = strings.TrimLeft(u, "\r\n\t ")
	u = strings.TrimRight(u, "\r\n\t ")

	m, err := vlc.NewMediaFromURL(u)
	if err != nil {
		pl.vlcMu.Unlock()
		return fmt.Errorf("new media from url failed: %w", err)
	}

	// media options: enable metadata, robust demux, user-agent/referrer, and sane caching/reconnect
	_ = m.AddOptions(
		":metadata-network-access=1",
		":icy-metadata=1",
		":demux=any",
		":http-user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		":http-referrer=https://www.bbc.co.uk/sounds",
		":network-caching=1500",
		":live-caching=1500",
		":http-reconnect",
	)

	if err := pl.p.SetMedia(m); err != nil {
		m.Release()
		pl.vlcMu.Unlock()
		return fmt.Errorf("set media failed: %w", err)
	}

	pl.media = m
	pl.stream = url
	pl.vlcMu.Unlock()

	// Ask libVLC to parse media before reading metadata for safety.
	// This reduces the risk of libVLC returning an invalid pointer to a Meta string.
	// (On VLC3 this behaves reliably; on VLC4 we skip metadata anyway).
	go func(mm *vlc.Media, timeout int) {
		// run in background without blocking UI
		_ = mm.ParseWithOptions(timeout, vlc.MediaParseNetwork, vlc.MediaFetchNetwork)
	}(m, pl.parseTimeout)

	return nil
}

// Play starts playback for the last loaded media and launches the ICY watcher.
func (pl *Player) Play() error {
	pl.vlcMu.Lock()
	err := pl.p.Play()
	pl.vlcMu.Unlock()
	if err != nil {
		return fmt.Errorf("play failed: %w", err)
	}
	pl.mu.Lock()
	pl.isPlaying = true
	cb := pl.onNow
	u := pl.stream
	pl.mu.Unlock()

	// Emit immediate status; ICY watcher updates once metadata arrives
	if cb != nil {
		cb("Streaming…")
	}
	// Start ICY watcher alongside VLC playback
	pl.startICYWatcher(u)
	return nil
}

// Stop halts playback, stops metadata pollers, and clears pending titles.
func (pl *Player) Stop() {
	// stop ICY watcher
	pl.stopICYWatcher()

	// stop metadata ticker (safe ticker)
	if pl.metaCancel != nil {
		pl.metaCancel()
		pl.metaWG.Wait()
		pl.metaCancel = nil
	}

	pl.vlcMu.Lock()
	_ = pl.p.Stop()
	pl.vlcMu.Unlock()

	pl.mu.Lock()
	pl.isPlaying = false
	pl.currentTitle, pl.pendingTitle = "", ""
	pl.firstSeenPending = time.Time{}
	pl.mu.Unlock()
}

// ToggleMute flips the mute state, returning the new muted flag and volume.
func (pl *Player) ToggleMute() (bool, int) {
	pl.vlcMu.Lock()
	pl.p.ToggleMute()
	pl.vlcMu.Unlock()

	pl.mu.Lock()
	pl.muted = !pl.muted
	v := pl.volume
	pl.mu.Unlock()
	return pl.muted, v
}

// SetVolume clamps and applies an absolute volume level (0-100).
func (pl *Player) SetVolume(v int) error {
	v = clamp(v, 0, 100)
	pl.vlcMu.Lock()
	err := pl.p.SetVolume(v)
	pl.vlcMu.Unlock()

	pl.mu.Lock()
	pl.volume = v
	pl.mu.Unlock()
	return err
}

// SetMute ensures libVLC reflects the requested mute state.
func (pl *Player) SetMute(m bool) error {
	pl.vlcMu.Lock()
	if m != pl.muted {
		pl.p.ToggleMute()
	}
	pl.vlcMu.Unlock()

	pl.mu.Lock()
	pl.muted = m
	pl.mu.Unlock()
	return nil
}

// Vol adjusts the current volume by the provided delta (clamped to 0-100).
func (pl *Player) Vol(delta int) int {
	pl.mu.Lock()
	newV := clamp(pl.volume+delta, 0, 100)
	pl.volume = newV
	pl.mu.Unlock()

	pl.vlcMu.Lock()
	_ = pl.p.SetVolume(newV)
	pl.vlcMu.Unlock()
	return newV
}

// remainingDur returns remaining playback duration if available.
func (pl *Player) remainingDur() (time.Duration, bool) {
	// Fallback implementation: libvlc-go v3.* Player may not expose Length/Time in this build.
	// Return false so stabilizer only uses time window without near-end gating.
	return 0, false
}

// handleICYTitle applies stabilization rules and emits onNow when appropriate.
// Important: some stations send StreamTitle only once per change. To avoid
// waiting for a second identical notification, we arm a delayed apply when a
// new pending title is first seen. If more updates arrive with the same title,
// the delayed apply will still succeed; if a different title arrives, the
// pending state is replaced and the previous delayed apply becomes a no‑op.
func (pl *Player) handleICYTitle(raw string) {
	title := strings.TrimSpace(html.UnescapeString(raw))
	if title == "" {
		return
	}

	// fast path: ignore duplicates
	pl.mu.Lock()
	if title == pl.currentTitle {
		pl.mu.Unlock()
		return
	}

	now := time.Now()
	// New candidate?
	if title != pl.pendingTitle {
		pl.pendingTitle = title
		pl.firstSeenPending = now
		// Arm a one-shot delayed apply after stableWindow
		pending := pl.pendingTitle
		pl.mu.Unlock()

		go func() {
			time.Sleep(stableWindow)
			// Re-check state after delay
			pl.mu.Lock()
			// If pending changed or we already applied a different current title, skip
			if pl.pendingTitle != pending || pl.currentTitle == pending {
				pl.mu.Unlock()
				return
			}
			// Optional near-end gating (disabled if remainingDur not available)
			if rem, ok := pl.remainingDur(); ok && rem > nearEndThreshold {
				pl.mu.Unlock()
				return
			}
			pl.currentTitle = pending
			out := pl.currentTitle
			cbLocal := pl.onNow
			pl.mu.Unlock()
			if cbLocal != nil {
				cbLocal(out)
			}
		}()
		return
	}
	// Same pending seen again — if the stable window elapsed, apply immediately
	first := pl.firstSeenPending
	pl.mu.Unlock()

	if time.Since(first) < stableWindow {
		return
	}
	if rem, ok := pl.remainingDur(); ok && rem > nearEndThreshold {
		return
	}
	pl.mu.Lock()
	pl.currentTitle = pl.pendingTitle
	applied := pl.currentTitle
	cbNow := pl.onNow
	pl.mu.Unlock()
	if cbNow != nil {
		cbNow(applied)
	}
}

// ----------------- Metadata (VLC3) -----------------

// startMetaPoll reads Meta only after media parsing completes successfully.
func (pl *Player) startMetaPoll() {
	pl.mu.Lock()
	onNow := pl.onNow
	pl.mu.Unlock()
	if onNow == nil {
		return
	}

	pl.vlcMu.Lock()
	media := pl.media // snapshot
	pl.vlcMu.Unlock()
	if media == nil {
		return
	}

	// stop previous poller
	if pl.metaCancel != nil {
		pl.metaCancel()
		pl.metaWG.Wait()
		pl.metaCancel = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	pl.metaCancel = cancel
	pl.metaWG.Add(1)

	go func() {
		defer pl.metaWG.Done()

		// give media time to parse; first pass happens after the parse timeout
		first := time.NewTimer(time.Duration(pl.parseTimeout+300) * time.Millisecond)
		ticker := time.NewTicker(5 * time.Second)
		defer func() {
			first.Stop()
			ticker.Stop()
		}()

		last := ""

		readAndEmit := func() {
			// verify media reference is still current
			pl.vlcMu.Lock()
			curr := pl.media
			pl.vlcMu.Unlock()
			if curr == nil || curr != media {
				return
			}

			// read parse status; Meta is safer to access after DONE
			status, _ := media.ParseStatus()
			if status != vlc.MediaParseDone {
				return
			}

			var sNow, title, artist string
			pl.vlcMu.Lock()
			sNow, _ = media.Meta(vlc.MediaNowPlaying)
			if sNow == "" {
				title, _ = media.Meta(vlc.MediaTitle)
				artist, _ = media.Meta(vlc.MediaArtist)
			}
			pl.vlcMu.Unlock()

			s := sNow
			if s == "" {
				s = strings.Trim(strings.TrimSpace(artist+" - "+title), " -")
			}
			if s != "" && s != "-" && s != last {
				last = s
				pl.mu.Lock()
				cb := pl.onNow
				pl.mu.Unlock()
				if cb != nil {
					cb(s)
				}
			}
		}

		// first attempt occurs after waiting for parse
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			readAndEmit()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				readAndEmit()
			}
		}
	}()
}

// ----------------- Safe mode (without Meta) -----------------

func (pl *Player) startSafeTickerOnly() {
	// stop previous poller
	if pl.metaCancel != nil {
		pl.metaCancel()
		pl.metaWG.Wait()
		pl.metaCancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	pl.metaCancel = cancel
	pl.metaWG.Add(1)

	go func() {
		defer pl.metaWG.Done()
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		alt := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pl.mu.Lock()
				cb := pl.onNow
				pl.mu.Unlock()
				if cb != nil {
					if alt {
						cb("Streaming…")
					} else {
						cb("Playing…")
					}
					alt = !alt
				}
			}
		}
	}()
}

// ----------------- ICY watcher integration -----------------

// startICYWatcher starts a background goroutine that reads ICY metadata from the same stream URL
// and invokes the onNow callback when StreamTitle changes. It does not affect VLC playback.
func (pl *Player) startICYWatcher(url string) {
	// stop previous watcher if any
	if pl.icyCancel != nil {
		pl.icyCancel()
		pl.icyWG.Wait()
		pl.icyCancel = nil
	}
	if url == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	pl.icyCancel = cancel
	pl.icyWG.Add(1)

	pl.mu.Lock()
	cbStation := pl.onStation
	hint := metadata.StrategyHint{Type: pl.metadataHintType, URL: pl.metadataHintURL}
	resolvedCb := pl.onMetadataResolved
	pl.mu.Unlock()

	go func() {
		defer pl.icyWG.Done()
		didSendStation := false
		provider := pl.metaProvider
		if provider == nil {
			provider = metadata.NewProvider(nil, stdLogger{})
		}
		provider.Watch(ctx, url, hint, func(info metadata.Info) {
			// push station name once when available (HTML-unescaped)
			if !didSendStation && cbStation != nil {
				name := strings.TrimSpace(info.Station)
				if name != "" {
					didSendStation = true
					cbStation(html.UnescapeString(name))
				}
			}
			// stabilize and emit Now Playing title
			if s := strings.TrimSpace(info.Title); s != "" {
				pl.handleICYTitle(s)
			}
		}, func(sel metadata.StrategyHint) {
			pl.mu.Lock()
			pl.metadataHintType = sel.Type
			pl.metadataHintURL = sel.URL
			pl.mu.Unlock()
			if resolvedCb != nil {
				resolvedCb(sel.Type, sel.URL)
			}
		})
	}()
}

// stopICYWatcher cancels and waits for the ICY watcher goroutine to exit.
func (pl *Player) stopICYWatcher() {
	if pl.icyCancel != nil {
		pl.icyCancel()
		pl.icyWG.Wait()
		pl.icyCancel = nil
	}
}
