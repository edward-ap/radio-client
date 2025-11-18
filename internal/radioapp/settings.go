package radioapp

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	config "github.com/edward-ap/miniradio/internal/config"
)

// BuildSettingsDrawer constructs the preset editor drawer, including station
// selection, URL entries, and EQ dropdowns. The returned CanvasObject is sized
// for two columns of presets, plus the preferred height.
func BuildSettingsDrawer(a *App) (fyne.CanvasObject, float32) {
	a.resetSettingsControls()

	left := a.buildSettingsColumn(0, 4)
	right := a.buildSettingsColumn(5, 9)
	grid := container.NewGridWithColumns(2, left, right)

	bg := canvas.NewRectangle(color.NRGBA{0x20, 0x20, 0x20, 0xFF})
	content := container.NewMax(bg, grid)
	return content, 240
}

// resetSettingsControls clears per-row widget references so the drawer can be
// rebuilt without reusing stale pointers.
func (a *App) resetSettingsControls() {
	for i := range a.settingsRadios {
		a.settingsRadios[i] = nil
	}
	for i := range a.eqPresetSelects {
		a.eqPresetSelects[i] = nil
	}
}

// buildSettingsColumn yields one half of the drawer (5 rows) including header.
func (a *App) buildSettingsColumn(from, to int) fyne.CanvasObject {
	rows := []fyne.CanvasObject{buildSettingsHeader()}
	for i := from; i <= to; i++ {
		rows = append(rows, a.buildSettingsRow(i))
	}
	return container.NewVBox(rows...)
}

// buildSettingsHeader renders the "Key / Stream URL / EQ Preset" header row.
func buildSettingsHeader() fyne.CanvasObject {
	keyHdr := widget.NewLabelWithStyle("Key", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	urlHdr := widget.NewLabelWithStyle("Stream URL", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	eqHdr := widget.NewLabelWithStyle("EQ Preset", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	left := container.New(layout.NewGridWrapLayout(fyne.NewSize(46, keyHdr.MinSize().Height)), keyHdr)
	right := container.New(layout.NewGridWrapLayout(fyne.NewSize(100, eqHdr.MinSize().Height)), eqHdr)
	return container.NewBorder(nil, nil, left, right, urlHdr)
}

// buildSettingsRow assembles a single preset row with radio, entry, and select.
func (a *App) buildSettingsRow(i int) fyne.CanvasObject {
	p := &a.config.Presets[i]
	radio := a.buildPresetRadio(i)

	urlEntry := widget.NewEntry()
	urlEntry.SetText(p.URL)
	var saveTimer *time.Timer
	urlEntry.OnChanged = func(s string) {
		oldTrim := strings.TrimSpace(p.URL)
		newTrim := strings.TrimSpace(s)
		if oldTrim != newTrim {
			p.MetadataType = ""
			p.MetadataURL = ""
		}
		p.URL = s
		if saveTimer != nil {
			saveTimer.Stop()
		}
		saveTimer = time.AfterFunc(400*time.Millisecond, func() { _ = a.config.Save() })
	}

	eqSelect := a.buildEQSelect(i, p)

	left := container.New(layout.NewGridWrapLayout(fyne.NewSize(46, radio.MinSize().Height)), radio)
	right := container.New(layout.NewGridWrapLayout(fyne.NewSize(100, eqSelect.MinSize().Height)), eqSelect)
	return container.NewBorder(nil, nil, left, right, urlEntry)
}

// buildPresetRadio returns the single-select control used to activate stations.
func (a *App) buildPresetRadio(i int) *widget.Check {
	label := settingsPresetLabel(i)
	p := widget.NewCheck(label, nil)
	radiosEnabled := func() bool { return a.playedOnce || (a.player != nil && a.player.IsPlaying()) }

	p.OnChanged = func(b bool) {
		defer a.ensureShortcutFocus()
		if a.silentUpdating {
			return
		}
		if !radiosEnabled() {
			p.SetChecked(false)
			return
		}
		if b {
			a.silentUpdating = true
			for j, c := range a.settingsRadios {
				if j != i && c != nil {
					c.SetChecked(false)
				}
			}
			a.silentUpdating = false
			a.activatePreset(i)
			return
		}
		if a.config.LastPreset == i {
			a.silentUpdating = true
			p.SetChecked(true)
			a.silentUpdating = false
		}
	}

	if radiosEnabled() {
		a.silentUpdating = true
		p.SetChecked(a.config.LastPreset == i)
		a.silentUpdating = false
	} else {
		p.Disable()
	}

	a.settingsRadios[i] = p
	return p
}

// settingsPresetLabel maps preset index to the keyboard shortcut shown in UI.
func settingsPresetLabel(i int) string {
	switch i {
	case 0:
		return "1"
	case 1:
		return "2"
	case 2:
		return "3"
	case 3:
		return "4"
	case 4:
		return "5"
	case 5:
		return "6"
	case 6:
		return "7"
	case 7:
		return "8"
	case 8:
		return "9"
	case 9:
		return "0"
	default:
		return fmt.Sprintf("%d", i)
	}
}

// buildEQSelect returns the dropdown used inside Settings for EQ selection.
func (a *App) buildEQSelect(idx int, preset *config.Preset) *widget.Select {
	options := a.settingsEQOptions()
	sel := widget.NewSelect(options, func(s string) {
		if a.silentUpdating {
			return
		}
		preset.EQ = s
		_ = a.config.Save()

		activeIdx := a.currentPresetIndex()
		if a.eq == nil || activeIdx != idx {
			return
		}
		custom := make(map[string]config.EQPresetData)
		for _, ce := range a.config.CustomEQPresets {
			custom[strings.TrimSpace(ce.Name)] = ce
		}
		_ = a.eq.ApplyPresetName(a.player, s, custom)

		if a.drawerMode == "equalizer" && a.eqDrawerPreset != nil {
			a.silentUpdating = true
			if strings.EqualFold(s, EQNameOff) {
				a.eqDrawerPreset.SetSelected(EQNameFlat)
			} else {
				a.eqDrawerPreset.SetSelected(s)
			}
			a.silentUpdating = false
		}
	})

	if strings.TrimSpace(preset.EQ) == "" || strings.EqualFold(strings.TrimSpace(preset.EQ), EQNameAuto) {
		preset.EQ = EQNameOff
	}
	sel.Selected = preset.EQ
	a.eqPresetSelects[idx] = sel
	return sel
}

// settingsEQOptions lists Off + Flat + built-ins + custom presets for settings.
func (a *App) settingsEQOptions() []string {
	if a.eq == nil {
		return []string{EQNameOff}
	}
	custom := make([]string, 0, len(a.config.CustomEQPresets))
	for _, ce := range a.config.CustomEQPresets {
		custom = append(custom, ce.Name)
	}
	return a.eq.PresetNamesForSettings(custom)
}
