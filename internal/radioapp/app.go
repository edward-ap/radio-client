// Package radioapp wires the UI, player, and configuration layers together to
// present the MiniRadio desktop application window.
package radioapp

import (
	"context"
	"fmt"
	"html"
	"image/color"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	config "github.com/edward-ap/miniradio/internal/config"
	eqmodel "github.com/edward-ap/miniradio/internal/equalizer"
	windowpos "github.com/edward-ap/miniradio/internal/platform/win/windowpos"
	playerpkg "github.com/edward-ap/miniradio/internal/player"
	ui "github.com/edward-ap/miniradio/internal/ui"
)

// PlayerState mirrors runtime information emitted from the player component so
// UI widgets can reflect it without querying libVLC directly.
type PlayerState struct {
	CurrentURL string
	IsPlaying  bool
	Volume     int
	IsMuted    bool
	CurrentEQ  string
}

// DrawerKind enumerates the available slide-up panels.
type DrawerKind int

const (
	// DrawerNone indicates that no sliding panel is open.
	DrawerNone DrawerKind = iota
	// DrawerSettings shows station presets and metadata options.
	DrawerSettings
	// DrawerEqualizer shows the EQ sliders and preset list.
	DrawerEqualizer
)

// UIState stores high-level visual selections so they can be persisted or
// toggled from handlers.
type UIState struct {
	ActiveDrawer     DrawerKind
	SelectedStation  int
	SelectedEQPreset string
	ShowMetadata     bool
}

// App owns the fyne application, main window, player, drawers, and widgets.
// It orchestrates interactions between user input, playback, metadata, and
// configuration persistence.
type App struct {
	fa     fyne.App
	w      fyne.Window
	player *playerpkg.Player
	config *config.Config

	playerState PlayerState
	uiState     UIState

	// Equalizer
	eq            *Equalizer
	customEQIndex map[string]int // name -> index in Config.CustomEQPresets
	drawerMode    string         // "", "settings", "equalizer"
	eqModel       *eqmodel.EQModel

	// UI elements
	playBtn     *widget.Button
	settingsBtn *widget.Button
	eqBtn       *widget.Button
	centerLbl   *widget.Label
	centerWrap  fyne.CanvasObject
	leftSide    fyne.CanvasObject
	rightSide   fyne.CanvasObject

	// button background highlights
	settingsBg *canvas.Rectangle
	eqBtnBg    *canvas.Rectangle

	// ticker visuals
	tickerBg *canvas.Rectangle

	// volume controls
	volBtn    *widget.Button
	volSlider *ui.MiniThumbSlider

	// stream indicator (circle that animates when active)
	ind *ui.StreamIndicator

	// ticker controller
	ticker *ui.TickerController

	shortcutCatcher *shortcutCatcher

	// root containers
	root     fyne.CanvasObject
	topFixed fyne.CanvasObject
	// drawer persistent area in Bottom of root border
	drawerBox *fyne.Container
	drawerBg  *canvas.Rectangle
	// legacy holder (unused after refactor), kept for compatibility
	bottomHolder *fyne.Container

	// drawer state
	drawerShown   bool
	drawer        fyne.CanvasObject
	drawerHeight  float32
	drawerContent fyne.CanvasObject
	baseHeight    float32
	pendingDrawer string

	// controls inside settings drawer for single-select behavior
	settingsRadios [10]*widget.Check
	// keep references to EQ preset selects in Settings for synchronization
	eqPresetSelects [10]*widget.Select
	// reference to EQ drawer preset select for cross-sync
	eqDrawerPreset *widget.Select
	// EQ drawer UI helpers
	eqSliderValues  []float64
	eqPreampSlider  *ui.VerticalSlider
	eqBandSliders   []*ui.VerticalSlider
	eqNameEntry     *widget.Entry
	eqSaveButton    *widget.Button
	eqDeleteButton  *widget.Button
	eqCustomNames   []string
	eqCustomMap     map[string]config.EQPresetData
	eqSlidersSilent bool
	silentUpdating  bool
	// after first successful Play we allow radios interaction even when stopped later
	playedOnce bool
}

// NewApp wires configuration, player, and fyne scaffolding into a ready-to-run
// App instance.
func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		log.Println("config load error:", err)
		cfg = &config.Config{CurrentURL: config.DefaultTestURL, Volume: config.DefaultVolume, WindowW: config.DefaultWidth, WindowH: config.DefaultHeight}
	}

	fa := app.NewWithID(config.AppID)
	// Switch to dark theme early for better contrast
	fa.Settings().SetTheme(theme.DarkTheme())
	// Set application icon
	if AppIcon != nil {
		fa.SetIcon(AppIcon)
	}
	w := fa.NewWindow("MiniRadio")
	w.SetMaster()
	w.SetPadded(false)
	// Disable window resizing/maximize; keep narrow strip height on start
	w.SetFixedSize(true)
	if AppIcon != nil {
		w.SetIcon(AppIcon)
	}
	w.Resize(fyne.NewSize(float32(cfg.WindowW), float32(config.DefaultHeight)))

	p := playerpkg.NewPlayer()

	app := &App{
		fa:     fa,
		w:      w,
		player: p,
		config: cfg,
		playerState: PlayerState{
			CurrentURL: cfg.CurrentURL,
			IsPlaying:  false,
			Volume:     cfg.Volume,
			IsMuted:    cfg.Muted,
			CurrentEQ:  EQNameOff,
		},
		uiState: UIState{
			ActiveDrawer:     DrawerNone,
			SelectedStation:  cfg.LastPreset,
			SelectedEQPreset: EQNameOff,
			ShowMetadata:     true,
		},
	}
	if idx := cfg.LastPreset; idx >= 0 && idx < len(cfg.Presets) {
		name := strings.TrimSpace(cfg.Presets[idx].EQ)
		if name == "" {
			name = EQNameOff
		}
		app.playerState.CurrentEQ = name
		app.uiState.SelectedEQPreset = name
	}

	app.buildUI()
	app.restoreWindowPlacement()
	if app.ticker != nil {
		app.ticker.SetText("Initializing VLC…")
	}

	// Initialize VLC asynchronously to avoid blocking UI startup
	go func() {
		if err := p.Init(cfg.Volume, cfg.Muted); err != nil {
			ui.CallOnMain(func() {
				// Show friendly message about VLC runtime layout (DLLs + plugins)
				dialog.ShowError(fmt.Errorf("cannot initialize VLC: %w\n\nPlace next to .exe: libvlc.dll, libvlccore.dll, and the plugins\\ folder (copied entirely from VLC).\nAlternatively install VLC, add its bin to PATH, and point plugins via VLC_PLUGIN_PATH or rely on the system path.\nArchitectures must match (x64 ⇔ x64).", err), w)
				if app.ticker != nil {
					app.ticker.SetText("VLC init failed")
				}
			})
			return
		}

		// Set callbacks after init
		p.SetOnNow(func(s string) {
			if app.ticker != nil {
				msg := s
				if strings.TrimSpace(msg) == "" {
					msg = "Streaming…"
				}
				ui.CallOnMain(func() { app.ticker.SetText(msg) })
			}
		})
		p.SetOnStation(func(name string) {
			n := strings.TrimSpace(html.UnescapeString(name))
			title := "MiniRadio"
			if n != "" {
				title = "MiniRadio — " + n
			}
			ui.CallOnMain(func() { w.SetTitle(title) })
		})

		// apply initial volume/mute after init
		_ = p.SetVolume(cfg.Volume)
		if cfg.Muted {
			_ = p.SetMute(true)
		}

		// Initialize Equalizer after VLC is ready
		if e, err := NewEqualizer(); err == nil {
			app.eq = e
			_ = app.player.SetAudioEqualizer(app.eq.eq)
			ui.CallOnMain(func() { app.rebuildCustomEQIndex() })
		}

		ui.CallOnMain(func() {
			if app.ticker != nil {
				app.ticker.SetText("Ready")
			}
		})
	}()

	// window close handler: save size & position, release resources
	w.SetCloseIntercept(func() {
		sz := w.Canvas().Size()
		cfg.WindowW = int(sz.Width)
		// Always persist the base strip height, not current window height that may include drawer
		if app != nil && app.baseHeight > 0 {
			cfg.WindowH = int(app.baseHeight)
		} else {
			cfg.WindowH = config.DefaultHeight
		}
		app.captureWindowPlacement()
		// save config
		_ = cfg.Save()
		if app.ticker != nil {
			app.ticker.Close()
		}
		if app.eq != nil {
			app.eq.Release()
		}
		if app.player != nil {
			app.player.Release()
		}
		// libvlc-go requires global release; done in Player.Release internally after player release as well.
		w.Close()
		fa.Quit()
	})

	// keyboard shortcuts
	w.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
		app.handleShortcutKey(ke)
	})

	return app
}

func keyToPresetIndex(key fyne.KeyName) int {
	switch key {
	case fyne.Key1:
		return 0
	case fyne.Key2:
		return 1
	case fyne.Key3:
		return 2
	case fyne.Key4:
		return 3
	case fyne.Key5:
		return 4
	case fyne.Key6:
		return 5
	case fyne.Key7:
		return 6
	case fyne.Key8:
		return 7
	case fyne.Key9:
		return 8
	case fyne.Key0:
		return 9
	}
	return -1
}

// handleShortcutKey centralizes keyboard shortcuts regardless of which widget
// currently owns focus.
func (a *App) handleShortcutKey(ke *fyne.KeyEvent) {
	if ke == nil {
		return
	}
	switch ke.Name {
	case fyne.KeySpace:
		a.togglePlay()
	case fyne.KeyUp:
		a.changeVolume(+10)
	case fyne.KeyDown:
		a.changeVolume(-10)
	case fyne.KeyPlus:
		a.changeVolume(+1)
	case fyne.KeyMinus:
		a.changeVolume(-1)
	case fyne.KeyAsterisk:
		a.toggleMute()
	case fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5, fyne.Key6, fyne.Key7, fyne.Key8, fyne.Key9, fyne.Key0:
		idx := keyToPresetIndex(ke.Name)
		a.activatePreset(idx)
	}
}

// Run finalizes layout construction and enters the fyne event loop.
func (a *App) Run() {
	a.w.ShowAndRun()
}

// buildUI builds the narrow strip UI using dedicated helpers for clarity.
// buildUI initializes drawers and the top control bar that make up the single
// MiniRadio window.
func (a *App) buildUI() {
	// The base (strip) height is fixed and should never include drawer height.
	a.baseHeight = config.DefaultHeight
	a.buildDrawers()
	a.buildMainWindow()
}

// buildMainWindow wires the control bar into a border layout and applies the
// desired window size before showing it.
func (a *App) buildMainWindow() {
	a.topFixed = a.buildControlBar()
	a.root = container.NewBorder(a.topFixed, a.drawerBox, nil, nil, nil)
	a.w.SetContent(a.root)
	a.ensureShortcutFocus()

	ui.CallOnMain(func() {
		targetW := float32(a.config.WindowW)
		if targetW < config.MinWindowWidth {
			targetW = config.MinWindowWidth
		}
		h := a.topFixed.MinSize().Height
		a.w.Resize(fyne.NewSize(targetW, h))
		a.w.Canvas().Refresh(a.root)
	})
}

// buildControlBar constructs the narrow playback strip that houses controls,
// ticker, buttons, and overlays.
func (a *App) buildControlBar() fyne.CanvasObject {
	if a.shortcutCatcher == nil {
		a.shortcutCatcher = newShortcutCatcher(a.handleShortcutKey)
	}

	barHeight := a.baseHeight
	darkBg := color.NRGBA{0x1a, 0x1a, 0x1a, 0xFF}

	// --- PLAY ---------------------------------------------------------

	playIcon := theme.MediaPlayIcon()
	a.playBtn = widget.NewButtonWithIcon("", playIcon, func() { a.togglePlay() })
	a.playBtn.Importance = widget.LowImportance

	playBg := canvas.NewRectangle(darkBg)
	playBg.SetMinSize(fyne.NewSize(36, barHeight))

	playWrap := container.NewMax(
		playBg,
		container.NewCenter(a.playBtn),
	)

	leftBlock := container.NewHBox(
		playWrap,
		widget.NewSeparator(),
	)

	// --- CENTER: indicator + ticker -----------------------------------

	a.centerLbl = widget.NewLabel("…")
	a.centerLbl.Truncation = fyne.TextTruncateClip
	a.centerLbl.Alignment = fyne.TextAlignLeading

	a.tickerBg = canvas.NewRectangle(color.NRGBA{0x00, 0x99, 0xFF, 0x40})
	bgHeight := barHeight - 6
	if bgHeight < 1 {
		bgHeight = 1
	}
	a.tickerBg.SetMinSize(fyne.NewSize(1, bgHeight))

	a.ind = ui.NewStreamIndicator(14)
	gap := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	gap.SetMinSize(fyne.NewSize(6, 1))

	labelWrap := container.NewMax(a.centerLbl)
	a.centerWrap = labelWrap
	a.ticker = ui.NewTickerController(a.centerLbl, a.centerWrap)

	centerRow := container.NewHBox(
		a.ind.CanvasObject(),
		gap,
		labelWrap,
	)

	tickerPadTop := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	tickerPadTop.SetMinSize(fyne.NewSize(1, 3))
	tickerPadBottom := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	tickerPadBottom.SetMinSize(fyne.NewSize(1, 3))
	centerBgWrap := container.NewBorder(
		tickerPadTop,
		tickerPadBottom,
		nil,
		nil,
		a.tickerBg,
	)

	centerContent := container.NewMax(
		centerBgWrap,
		container.NewPadded(centerRow),
	)

	// --- RIGHT: громкость + настройки + EQ ----------------------------

	a.volBtn = widget.NewButtonWithIcon("", theme.VolumeUpIcon(), func() { a.toggleMute() })
	a.volBtn.Importance = widget.LowImportance

	a.volSlider = ui.NewMiniThumbSlider(0, 100)
	a.volSlider.Step = 1
	a.volSlider.Value = float64(a.config.Volume)
	a.updateVolumeIcon()

	var saveTimer *time.Timer
	a.volSlider.OnChanged = func(v float64) {
		vv := int(v + 0.5)
		a.ensureVolumeUnmuted()
		if a != nil && a.player != nil {
			_ = a.player.SetVolume(vv)
		}
		a.config.Volume = vv
		a.playerState.Volume = vv
		a.updateVolumeIcon()
		if saveTimer != nil {
			saveTimer.Stop()
		}
		saveTimer = time.AfterFunc(400*time.Millisecond, func() { _ = a.config.Save() })
	}

	longVol := container.New(
		layout.NewGridWrapLayout(fyne.NewSize(110, a.volSlider.MinSize().Height)),
		a.volSlider,
	)

	// лёгкий сдвиг вниз, чтобы не висело к верхнему краю
	topPad := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	topPad.SetMinSize(fyne.NewSize(1, 3))
	volCentered := container.NewBorder(topPad, nil, nil, nil, longVol)

	a.settingsBtn = widget.NewButtonWithIcon("", theme.SettingsIcon(), func() { a.toggleSettingsDrawer() })
	a.settingsBtn.Importance = widget.LowImportance

	a.eqBtn = widget.NewButtonWithIcon("", theme.MediaMusicIcon(), func() { a.toggleEQDrawer() })
	a.eqBtn.Importance = widget.LowImportance

	a.settingsBg = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	settingsWrap := container.NewMax(a.settingsBg, a.settingsBtn)

	a.eqBtnBg = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	eqWrap := container.NewMax(a.eqBtnBg, a.eqBtn)

	rightPanel := container.NewHBox(
		a.volBtn,
		volCentered,
		widget.NewSeparator(),
		settingsWrap,
		eqWrap,
	)

	rightBg := canvas.NewRectangle(darkBg)
	rightBg.SetMinSize(fyne.NewSize(1, barHeight))

	rightBlock := container.NewMax(
		rightBg,
		container.NewPadded(rightPanel),
	)

	// --- Вся верхняя панель -------------------------------------------

	topContent := container.NewBorder(
		nil, nil,
		leftBlock,
		rightBlock,
		centerContent,
	)

	// лёгкий 1px отступ сверху (если не нужен – можно убрать этот VBox)
	spacerTop := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	spacerTop.SetMinSize(fyne.NewSize(1, 1))
	topWithOffset := container.NewVBox(spacerTop, topContent)

	// shortcut overlay
	a.shortcutCatcher.Resize(fyne.NewSize(1, 1))
	a.shortcutCatcher.Move(fyne.NewPos(-5, -5))
	topOverlay := container.NewStack(topWithOffset, container.NewPadded(a.shortcutCatcher))

	// общий фон панели (можно включать синий для дебага)
	spTop := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	// spTop := canvas.NewRectangle(color.NRGBA{0x00, 0x40, 0x80, 0xFF}) // debug
	spTop.SetMinSize(fyne.NewSize(1, barHeight))

	return container.NewMax(spTop, topOverlay)
}

// buildDrawers initializes the persistent bottom container used for settings
// and EQ drawers, keeping them ready to slide in when requested.
func (a *App) buildDrawers() {
	a.drawerBg = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	a.drawerBg.SetMinSize(fyne.NewSize(1, 0))

	a.drawerBox = container.NewMax(a.drawerBg)
	a.drawerBox.Resize(fyne.NewSize(a.w.Canvas().Size().Width, 0))

	a.drawer = nil
	a.drawerShown = false
	a.drawerHeight = 0
	a.pendingDrawer = ""
}

// ensureShortcutFocus makes sure the invisible shortcut catcher keeps focus so
// global key handling works even when drawers are closed.
func (a *App) ensureShortcutFocus() {
	if a == nil || a.w == nil || a.shortcutCatcher == nil {
		return
	}
	ui.CallOnMain(func() {
		if a.w != nil && a.shortcutCatcher != nil {
			a.w.Canvas().Focus(a.shortcutCatcher)
		}
	})
}

type shortcutCatcher struct {
	widget.BaseWidget
	onKey func(*fyne.KeyEvent)
}

func newShortcutCatcher(handler func(*fyne.KeyEvent)) *shortcutCatcher {
	c := &shortcutCatcher{onKey: handler}
	c.ExtendBaseWidget(c)
	return c
}

func (s *shortcutCatcher) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	rect.SetMinSize(fyne.NewSize(1, 1))
	return widget.NewSimpleRenderer(rect)
}

func (s *shortcutCatcher) MinSize() fyne.Size {
	return fyne.NewSize(1, 1)
}

func (s *shortcutCatcher) Resize(size fyne.Size) {
	s.BaseWidget.Resize(fyne.NewSize(1, 1))
}

func (s *shortcutCatcher) FocusGained() {}

func (s *shortcutCatcher) FocusLost() {}

func (s *shortcutCatcher) TypedKey(ev *fyne.KeyEvent) {
	if s.onKey != nil {
		s.onKey(ev)
	}
}

func (s *shortcutCatcher) TypedRune(r rune) {}

// togglePlay starts or stops playback depending on the current player state,
// updating button labels and metadata watchers accordingly.
func (a *App) togglePlay() {
	if a.player == nil {
		dialog.ShowError(fmt.Errorf("VLC not initialized"), a.w)
		return
	}
	if a.player.IsPlaying() {
		a.player.Stop()
		a.playBtn.SetIcon(theme.MediaPlayIcon())
		if a.ind != nil {
			a.ind.SetActive(false)
		}
		a.UpdateTicker("Stopped")
		return
	}
	url := a.config.CurrentURL
	if url == "" {
		dialog.ShowInformation("Stream URL", "URL is empty. Open Settings to configure the stream.", a.w)
		return
	}
	if a.player != nil {
		if idx := a.currentPresetIndex(); idx >= 0 && idx < len(a.config.Presets) {
			p := a.config.Presets[idx]
			metaType := strings.TrimSpace(p.MetadataType)
			metaURL := strings.TrimSpace(p.MetadataURL)
			idxCopy := idx
			a.player.SetMetadataHint(metaType, metaURL, func(t, u string) {
				a.handleMetadataDiscovered(idxCopy, t, u)
			})
		}
	}
	// Before loading/playing, apply EQ for the active station (Settings EQ preset)
	if a.eq != nil {
		idx := a.currentPresetIndex()
		if idx >= 0 && idx < len(a.config.Presets) {
			name := a.config.Presets[idx].EQ
			if strings.TrimSpace(name) == "" {
				name = EQNameOff
			}
			customMap := map[string]config.EQPresetData{}
			for _, ce := range a.config.CustomEQPresets {
				customMap[strings.TrimSpace(ce.Name)] = ce
			}
			_ = a.eq.ApplyPresetName(a.player, name, customMap)
		}
	}
	// Reset title to default; new stream may not provide station metadata
	ui.CallOnMain(func() { a.w.SetTitle("MiniRadio") })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.player.Load(ctx, url); err != nil {
		dialog.ShowError(err, a.w)
		a.UpdateTicker("Failed to load")
		return
	}
	if err := a.player.Play(); err != nil {
		dialog.ShowError(err, a.w)
		a.UpdateTicker("Failed to play")
		return
	}
	// Mark that the user has successfully started playback at least once
	a.playedOnce = true
	// If settings drawer is open, enable its radio controls now and sync selection
	if a.drawerShown {
		a.silentUpdating = true
		for i, c := range a.settingsRadios {
			if c != nil {
				c.Enable()
				c.SetChecked(i == a.config.LastPreset)
			}
		}
		a.silentUpdating = false
	}
	a.playBtn.SetIcon(theme.MediaStopIcon())
	if a.ind != nil {
		a.ind.SetActive(true)
	}
	a.UpdateTicker("Streaming…")
}

// changeVolume increments/decrements the slider and player volume in tandem.
func (a *App) changeVolume(delta int) {
	if a.player == nil {
		return
	}
	a.ensureVolumeUnmuted()
	v := a.player.Vol(delta)
	a.config.Volume = v
	// sync UI controls
	if a.volSlider != nil {
		a.volSlider.SetValue(float64(v))
	}
	a.updateVolumeIcon()
	_ = a.config.Save()
}

// toggleMute flips muting both in the player and the UI toggle button.
func (a *App) toggleMute() {
	if a.player == nil {
		return
	}
	target := !a.config.Muted
	_ = a.player.SetMute(target)
	a.config.Muted = target
	a.updateVolumeIcon()
	_ = a.config.Save()
}

// ensureVolumeUnmuted automatically unmutes playback when the user drags the
// slider after previously muting.
func (a *App) ensureVolumeUnmuted() {
	if a == nil || a.config == nil || !a.config.Muted {
		return
	}
	if a.player != nil {
		_ = a.player.SetMute(false)
	}
	a.config.Muted = false
}

// updateVolumeIcon adjusts the mute button icon to represent the current level.
func (a *App) updateVolumeIcon() {
	if a.volBtn == nil || a.config == nil {
		return
	}
	icon := theme.VolumeUpIcon()
	if a.config.Muted || a.config.Volume <= 0 {
		icon = theme.VolumeMuteIcon()
	}
	a.volBtn.SetIcon(icon)
}

// rebuildCustomEQIndex rebuilds the lookup map for user EQ presets to keep the
// settings drawer in sync with config changes.
func (a *App) rebuildCustomEQIndex() {
	a.customEQIndex = map[string]int{}
	for i, p := range a.config.CustomEQPresets {
		a.customEQIndex[strings.TrimSpace(p.Name)] = i
	}
}

// toggleEQDrawer toggles the EQ drawer visibility or opens it if closed.
func (a *App) toggleEQDrawer() {
	target := "equalizer"
	if a.pendingDrawer == target {
		target = ""
	}
	a.setDrawerTarget(target)
}

// toggleSettingsDrawer toggles the station/settings drawer visibility.
func (a *App) toggleSettingsDrawer() {
	target := "settings"
	if a.pendingDrawer == target {
		target = ""
	}
	a.setDrawerTarget(target)
}

// restoreWindowPlacement loads persisted window coordinates when supported.
func (a *App) restoreWindowPlacement() {
	if a == nil || a.w == nil || a.config == nil || !a.config.WindowPosValid {
		return
	}
	try := func() bool {
		return windowpos.ApplyWindowPosition(a.w, a.config.WindowX, a.config.WindowY)
	}
	if try() {
		return
	}
	go func() {
		const attempts = 10
		for i := 0; i < attempts; i++ {
			time.Sleep(150 * time.Millisecond)
			if !a.config.WindowPosValid {
				return
			}
			if try() {
				return
			}
		}
	}()
}

// captureWindowPlacement stores window coordinates before toggling drawers so
// the window returns to the user's preferred spot.
func (a *App) captureWindowPlacement() {
	if a == nil || a.w == nil || a.config == nil {
		return
	}
	if x, y, ok := windowpos.GetWindowPosition(a.w); ok {
		a.config.WindowX = x
		a.config.WindowY = y
		a.config.WindowPosValid = true
	}
}

// setDrawerTarget updates pendingDrawer and schedules animation changes.
func (a *App) setDrawerTarget(mode string) {
	a.pendingDrawer = mode
	a.applyDrawerTarget()
}

// applyDrawerTarget materializes pending drawer changes (open/close/switch).
func (a *App) applyDrawerTarget() {
	desired := a.pendingDrawer
	if desired == "" {
		if a.drawerShown {
			a.closeDrawer()
		}
		return
	}
	if a.drawerShown {
		if a.drawerMode == desired {
			return
		}
		a.closeDrawer()
	}
	content, target := a.buildDrawerContent(desired)
	if content == nil || target <= 0 {
		if desired == a.pendingDrawer {
			a.pendingDrawer = ""
		}
		return
	}
	a.openDrawer(content, target, desired)
}

// buildDrawerContent returns the widget tree and height for the requested
// drawer type.
func (a *App) buildDrawerContent(mode string) (fyne.CanvasObject, float32) {
	switch mode {
	case "settings":
		return BuildSettingsDrawer(a)
	case "equalizer":
		if a.eq == nil {
			return nil, 0
		}
		return BuildEqualizerDrawer(a)
	}
	return nil, 0
}

// applyDrawerHighlight paints the correct button background based on drawer
// type.
func (a *App) applyDrawerHighlight(mode string) {
	if a.settingsBg != nil {
		a.settingsBg.FillColor = color.NRGBA{0, 0, 0, 0}
		a.settingsBg.Refresh()
	}
	if a.eqBtnBg != nil {
		a.eqBtnBg.FillColor = color.NRGBA{0, 0, 0, 0}
		a.eqBtnBg.Refresh()
	}
}

// clearDrawerHighlight removes all button highlights, used when drawers close.
func (a *App) clearDrawerHighlight() {
	if a.settingsBg != nil {
		a.settingsBg.FillColor = color.NRGBA{0, 0, 0, 0}
		a.settingsBg.Refresh()
	}
	if a.eqBtnBg != nil {
		a.eqBtnBg.FillColor = color.NRGBA{0, 0, 0, 0}
		a.eqBtnBg.Refresh()
	}
}

// openDrawer attaches the provided content and animates the drawer open.
func (a *App) openDrawer(content fyne.CanvasObject, height float32, mode string) {
	a.drawerContent = content
	a.drawerBox.Objects = []fyne.CanvasObject{a.drawerBg, a.drawerContent}
	a.drawerBox.Refresh()
	a.applyDrawerHighlight(mode)
	a.resizeWindowForDrawer(height)
	a.setDrawerHeight(height)
	a.drawerShown = true
	a.drawerHeight = height
	a.drawerMode = mode
}

// closeDrawer collapses the drawer and clears its content container.
func (a *App) closeDrawer() {
	a.clearDrawerHighlight()
	a.setDrawerHeight(0)
	ui.CallOnMain(func() {
		a.drawerBox.Objects = []fyne.CanvasObject{a.drawerBg}
		a.drawerBox.Refresh()
	})
	a.resizeWindowForDrawer(0)
	a.drawer = nil
	a.drawerContent = nil
	a.drawerShown = false
	a.drawerHeight = 0
	a.eqDrawerPreset = nil
	a.drawerMode = ""
}

// setDrawerHeight resizes the drawer container and refreshes the layout.
func (a *App) setDrawerHeight(h float32) {
	ui.CallOnMain(func() {
		w := a.w.Canvas().Size().Width
		a.drawerBox.Resize(fyne.NewSize(w, h))
		a.drawerBg.Resize(fyne.NewSize(w, h))
		a.drawerBox.Refresh()
		a.drawerHeight = h
		if a.root != nil {
			a.root.Refresh()
		}
	})
}

// resizeWindowForDrawer resizes the window so that its height matches
// the top strip (topFixed) plus the requested drawer height.
// resizeWindowForDrawer tweaks the fyne window height when drawers open to
// avoid clipping content.
func (a *App) resizeWindowForDrawer(extra float32) {
	ui.CallOnMain(func() {
		if a == nil || a.w == nil || a.topFixed == nil {
			return
		}
		canvas := a.w.Canvas()
		if canvas == nil {
			return
		}

		// Base strip height = MinSize of the top container.
		topH := a.topFixed.MinSize().Height
		if topH <= 0 {
			// Fallback: use configured base height if MinSize is not ready yet.
			topH = a.baseHeight
			if topH <= 0 {
				topH = float32(config.DefaultHeight)
			}
		}

		newH := topH
		if extra > 0 {
			newH += extra
		}

		sz := canvas.Size()
		a.w.Resize(fyne.NewSize(sz.Width, newH))
		canvas.Refresh(a.root)
	})
}

// activatePreset updates selection state, applies metadata hints, and loads
// the referenced station in the player.
func (a *App) activatePreset(idx int) {
	if idx < 0 || idx >= len(a.config.Presets) {
		return
	}
	p := a.config.Presets[idx]
	if p.URL == "" {
		return
	}
	if a.player != nil {
		metaType := strings.TrimSpace(p.MetadataType)
		metaURL := strings.TrimSpace(p.MetadataURL)
		idxCopy := idx
		a.player.SetMetadataHint(metaType, metaURL, func(t, u string) {
			a.handleMetadataDiscovered(idxCopy, t, u)
		})
	}
	a.config.CurrentURL = p.URL
	a.config.LastPreset = idx
	_ = a.config.Save()
	// reset window title to default; new station may not provide metadata
	ui.CallOnMain(func() { a.w.SetTitle("MiniRadio") })
	// sync drawer radios if open
	if a.drawerShown {
		a.silentUpdating = true
		for i, c := range a.settingsRadios {
			if c != nil {
				c.SetChecked(i == idx)
			}
		}
		a.silentUpdating = false
	}
	a.UpdateTicker(fmt.Sprintf("Preset %d: %s", idx+1, nonEmpty(p.Name, p.URL)))
	// Apply EQ preset, if available
	if a.eq != nil {
		custom := map[string]config.EQPresetData{}
		for _, ce := range a.config.CustomEQPresets {
			custom[strings.TrimSpace(ce.Name)] = ce
		}
		_ = a.eq.ApplyPresetName(a.player, p.EQ, custom)
	}
	// auto play selected preset
	if a.player.IsPlaying() {
		a.player.Stop()
	}
	// Simulate click play
	a.togglePlay()
}

func nonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// currentPresetIndex returns index of the currently active preset (station) if known, else -1.
// currentPresetIndex finds the preset slot currently selected in the UI.
func (a *App) currentPresetIndex() int {
	if a == nil || a.config == nil {
		return -1
	}
	idx := a.config.LastPreset
	if idx >= 0 && idx < len(a.config.Presets) {
		// if it has URL, accept
		if strings.TrimSpace(a.config.Presets[idx].URL) != "" {
			return idx
		}
	}
	// fallback: try to match current URL
	cur := strings.TrimSpace(a.config.CurrentURL)
	if cur != "" {
		for i := range a.config.Presets {
			if strings.TrimSpace(a.config.Presets[i].URL) == cur {
				return i
			}
		}
	}
	return -1
}

// handleMetadataDiscovered keeps the config and active station in sync with new
// metadata endpoint hints emitted by the player.
func (a *App) handleMetadataDiscovered(idx int, metaType, metaURL string) {
	if a == nil || a.config == nil {
		return
	}
	if idx < 0 || idx >= len(a.config.Presets) {
		return
	}
	typ := strings.ToUpper(strings.TrimSpace(metaType))
	url := strings.TrimSpace(metaURL)
	if typ == "" || url == "" {
		return
	}
	p := &a.config.Presets[idx]
	if strings.EqualFold(strings.TrimSpace(p.MetadataType), typ) && strings.TrimSpace(p.MetadataURL) == url {
		return
	}
	p.MetadataType = typ
	p.MetadataURL = url
	_ = a.config.Save()
}

// UpdateTicker sets ticker text via controller (UI thread safe).
// UpdateTicker refreshes the center label and ticker content from metadata
// callbacks (invoked by player package).
func (a *App) UpdateTicker(text string) {
	if a.ticker != nil {
		ui.CallOnMain(func() { a.ticker.SetText(text) })
	}
	// brief flash effect on ticker background to draw attention (UI thread safe)
	if a.tickerBg != nil {
		ui.CallOnMain(func() {
			// brighter cyan
			a.tickerBg.FillColor = color.NRGBA{0x00, 0xCC, 0xFF, 0x60}
			a.tickerBg.Refresh()
		})
		time.AfterFunc(180*time.Millisecond, func() {
			ui.CallOnMain(func() {
				// restore
				a.tickerBg.FillColor = color.NRGBA{0x00, 0x99, 0xFF, 0x40}
				a.tickerBg.Refresh()
			})
		})
	}
}

// ShowToast shows temporary ticker message (same channel as ticker) in a UI-safe way.
// ShowToast displays a transient dialog-like notification near the window.
func (a *App) ShowToast(text string) {
	if a.ticker != nil {
		ui.CallOnMain(func() { a.ticker.SetText(text) })
	}
}
