package radioapp

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	vlc "github.com/adrg/libvlc-go/v3"

	config "github.com/edward-ap/miniradio/internal/config"
	eqmodel "github.com/edward-ap/miniradio/internal/equalizer"
	playerpkg "github.com/edward-ap/miniradio/internal/player"
	ui "github.com/edward-ap/miniradio/internal/ui"
)

const (
	// EQNameFlat is the canonical neutral preset.
	EQNameFlat = "Flat"
	// EQNameAuto is a legacy alias maintained for backward compatibility.
	EQNameAuto = "Auto"
	// EQNameManual is assigned when the user customizes slider values.
	EQNameManual = "Manual"
	// EQNameOff removes EQ processing entirely.
	EQNameOff = "Off"
)

// Equalizer wraps libVLC's equalizer, preset metadata, and slider ranges so the
// UI can present user-defined and built-in options consistently.
type Equalizer struct {
	eq        *vlc.Equalizer
	presets   []string
	bandsHz   []float64
	bandCount int
	preampMin float64
	preampMax float64
	ampMin    float64
	ampMax    float64
}

// NewEqualizer instantiates a libVLC equalizer and caches preset/band metadata
// for later use in the UI.
func NewEqualizer() (*Equalizer, error) {
	// VLC presets
	n := int(vlc.EqualizerPresetCount())
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		name := vlc.EqualizerPresetName(uint(i))
		names = append(names, name)
	}
	// Bands and frequencies
	bc := int(vlc.EqualizerBandCount())
	hz := make([]float64, 0, bc)
	for i := 0; i < bc; i++ {
		f := vlc.EqualizerBandFrequency(uint(i))
		hz = append(hz, f)
	}
	// Start with Flat
	eq, err := vlc.NewEqualizer()
	if err != nil {
		return nil, err
	}
	return &Equalizer{
		eq:        eq,
		presets:   names,
		bandsHz:   hz,
		bandCount: bc,
		preampMin: -20, preampMax: 20,
		ampMin: -20, ampMax: 20,
	}, nil
}

// Release frees the underlying libVLC equalizer, if any.
func (e *Equalizer) Release() {
	if e.eq != nil {
		e.eq.Release()
		e.eq = nil
	}
}

// PresetNamesWithFlat returns the preset order used in the EQ drawer:
// Flat, VLC built-ins (excluding duplicates), and lastly custom entries.
func (e *Equalizer) PresetNamesWithFlat(custom []string) []string {
	// For the EQ drawer: do not include Auto or Off; include exactly one Flat at the top,
	// then VLC built-ins (excluding duplicate Flat/Auto), then custom presets.
	out := []string{EQNameFlat}
	for _, name := range e.presets {
		if strings.EqualFold(name, EQNameFlat) || strings.EqualFold(name, EQNameAuto) {
			continue
		}
		out = append(out, name)
	}
	out = append(out, custom...)
	return out
}

// PresetNamesForSettings returns options for the Settings drawer dropdown:
// Off, Flat, VLC built-ins (without duplicates), followed by custom entries.
func (e *Equalizer) PresetNamesForSettings(custom []string) []string {
	out := []string{EQNameOff, EQNameFlat}
	for _, name := range e.presets {
		if strings.EqualFold(name, EQNameFlat) || strings.EqualFold(name, EQNameAuto) {
			continue
		}
		out = append(out, name)
	}
	out = append(out, custom...)
	return out
}

// Bands reports the slider band center frequencies and count.
func (e *Equalizer) Bands() ([]float64, int) { return e.bandsHz, e.bandCount }

// Range reports allowed dB ranges for preamp and per-band sliders.
func (e *Equalizer) Range() (preMin, preMax, ampMin, ampMax float64) {
	return e.preampMin, e.preampMax, e.ampMin, e.ampMax
}

// ApplyPresetName selects the given preset name (built-in, manual, or custom)
// and applies it to the provided Player. EQNameOff disables EQ entirely.
func (e *Equalizer) ApplyPresetName(player *playerpkg.Player, name string, custom map[string]config.EQPresetData) error {
	name = strings.TrimSpace(name)
	// Off disables EQ entirely for the player
	if strings.EqualFold(name, EQNameOff) {
		if e.eq != nil {
			_ = e.eq.Release()
			e.eq = nil
		}
		return player.SetAudioEqualizer(nil)
	}
	// legacy Auto behaves like Flat (enabled EQ with all 0 dB)
	if name == "" || strings.EqualFold(name, EQNameAuto) || strings.EqualFold(name, EQNameFlat) {
		newEq, err := vlc.NewEqualizer()
		if err != nil {
			return err
		}
		if e.eq != nil {
			_ = e.eq.Release()
		}
		e.eq = newEq
		return player.SetAudioEqualizer(e.eq)
	}
	if data, ok := custom[name]; ok {
		newEq, err := vlc.NewEqualizer()
		if err != nil {
			return err
		}
		_ = newEq.SetPreampValue(float64(data.Preamp))
		for i, v := range data.Bands {
			if i >= e.bandCount {
				break
			}
			_ = newEq.SetAmpValueAtIndex(float64(v), uint(i))
		}
		if e.eq != nil {
			_ = e.eq.Release()
		}
		e.eq = newEq
		return player.SetAudioEqualizer(e.eq)
	}
	// built-in VLC preset by name
	idx := -1
	for i, s := range e.presets {
		if strings.EqualFold(s, name) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		newEq, err := vlc.NewEqualizerFromPreset(uint(idx))
		if err != nil {
			return err
		}
		if e.eq != nil {
			_ = e.eq.Release()
		}
		e.eq = newEq
		return player.SetAudioEqualizer(e.eq)
	}
	// fallback to Flat
	newEq, err := vlc.NewEqualizer()
	if err != nil {
		return err
	}
	if e.eq != nil {
		_ = e.eq.Release()
	}
	e.eq = newEq
	return player.SetAudioEqualizer(e.eq)
}

// SetPreamp updates the global EQ preamp, clamping to supported ranges and
// applying the change to libVLC.
func (e *Equalizer) SetPreamp(player *playerpkg.Player, db float64) error {
	_ = e.eq.SetPreampValue(db)
	return player.SetAudioEqualizer(e.eq)
}

// SetBand adjusts a specific band slider and pushes the change to libVLC.
func (e *Equalizer) SetBand(player *playerpkg.Player, band int, db float64) error {
	if band < 0 || band >= e.bandCount {
		return nil
	}
	_ = e.eq.SetAmpValueAtIndex(db, uint(band))
	return player.SetAudioEqualizer(e.eq)
}

// BuildEqualizerDrawer builds the EQ drawer UI and returns the content and preferred height.
func BuildEqualizerDrawer(a *App) (fyne.CanvasObject, float32) {
	if a.eq == nil {
		return widget.NewLabel("Equalizer unavailable"), 200
	}

	a.rebuildCustomPresetLists()
	_, bandCount := a.eq.Bands()
	a.eqSliderValues = make([]float64, bandCount+1)
	a.eqBandSliders = make([]*ui.VerticalSlider, bandCount)
	a.eqPreampSlider = nil
	a.eqNameEntry = nil
	a.eqSaveButton = nil
	a.eqDeleteButton = nil
	a.eqSlidersSilent = false

	defaults := eqmodel.DefaultPresets()
	current := eqmodel.EQPreset{
		Name:   EQNameFlat,
		Preamp: 0,
		Gains:  make([]float64, bandCount),
	}
	a.eqModel = &eqmodel.EQModel{
		Presets: defaults,
		Current: current,
	}

	sliders := a.buildEQSlidersSection(a.eqModel)
	presets := a.buildEQPresetsSection(a.eqModel)
	buttons := a.buildEQButtonsSection(a.eqModel)

	header := container.NewBorder(nil, nil,
		widget.NewLabel("Presets"),
		buttons,
		presets,
	)
	content := container.NewVBox(header, widget.NewSeparator(), sliders)
	return content, 260
}

// rebuildCustomPresetLists refreshes the cached map and slice of custom EQ
// presets from config so dropdowns remain in sync with persisted data.
func (a *App) rebuildCustomPresetLists() {
	if a == nil || a.config == nil {
		return
	}
	a.eqCustomMap = map[string]config.EQPresetData{}
	a.eqCustomNames = make([]string, 0, len(a.config.CustomEQPresets))
	for _, p := range a.config.CustomEQPresets {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		a.eqCustomNames = append(a.eqCustomNames, p.Name)
		a.eqCustomMap[p.Name] = p
	}
}

// buildEQSlidersSection renders the vertical slider grid plus dB scale.
// buildEQSlidersSection renders the vertical slider grid plus dB scale.
func (a *App) buildEQSlidersSection(eqModel *eqmodel.EQModel) fyne.CanvasObject {
	preMin, preMax, ampMin, ampMax := a.eq.Range()
	preamp := ui.NewVerticalSlider(preMin, preMax)
	preamp.Step = 0.5
	a.eqPreampSlider = preamp
	preamp.OnChanged = func(v float64) {
		if a.eqSlidersSilent {
			return
		}
		a.updateEQSliderState(0, v)
		_ = a.eq.SetPreamp(a.player, v)
		if a.eqDrawerPreset != nil {
			a.silentUpdating = true
			a.eqDrawerPreset.SetSelected(EQNameManual)
			a.silentUpdating = false
		}
		if a.eqSaveButton != nil {
			a.eqSaveButton.Enable()
		}
		if a.eqDeleteButton != nil {
			a.eqDeleteButton.Disable()
		}
		a.updateEQButtonsForSelection(EQNameManual)
	}

	bandsHz, _ := a.eq.Bands()
	bandCols := []fyne.CanvasObject{}
	freqLabels := []fyne.CanvasObject{}
	for i := 0; i < len(a.eqBandSliders); i++ {
		s := ui.NewVerticalSlider(ampMin, ampMax)
		s.Step = 0.5
		idx := i
		s.OnChanged = func(v float64) {
			if a.eqSlidersSilent {
				return
			}
			a.updateEQSliderState(idx+1, v)
			_ = a.eq.SetBand(a.player, idx, v)
			if a.eqDrawerPreset != nil {
				a.silentUpdating = true
				a.eqDrawerPreset.SetSelected(EQNameManual)
				a.silentUpdating = false
			}
			if a.eqSaveButton != nil {
				a.eqSaveButton.Enable()
			}
			if a.eqDeleteButton != nil {
				a.eqDeleteButton.Disable()
			}
			a.updateEQButtonsForSelection(EQNameManual)
		}
		a.eqBandSliders[i] = s
		const bandCellW = float32(22)
		sz := s.MinSize()
		cell := container.New(layout.NewGridWrapLayout(fyne.NewSize(bandCellW, sz.Height)), s)
		bandCols = append(bandCols, cell)
		lbl := widget.NewLabel(hzLabel(bandsHz[i]))
		lbl.Alignment = fyne.TextAlignCenter
		padR := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
		padR.SetMinSize(fyne.NewSize(4, 1))
		wrap := container.NewBorder(nil, nil, nil, padR, container.NewCenter(lbl))
		labelCell := container.New(layout.NewGridWrapLayout(fyne.NewSize(bandCellW, wrap.MinSize().Height)), wrap)
		freqLabels = append(freqLabels, labelCell)
	}

	rot := ui.NewRotatedStaticLabel("Pre-amp")
	var bandHeight float32
	if len(bandCols) > 0 {
		bandHeight = bandCols[0].MinSize().Height
	}
	if bandHeight <= 0 {
		bandHeight = preamp.MinSize().Height
	}
	rot.SetTargetSize(32, bandHeight)
	rotCanvas := rot.CanvasObject()
	rotCell := container.New(layout.NewGridWrapLayout(fyne.NewSize(rotCanvas.MinSize().Width, bandHeight)),
		container.NewCenter(rotCanvas))
	preampCell := container.New(layout.NewGridWrapLayout(fyne.NewSize(40, bandHeight)), preamp)
	preampRow := container.NewHBox(rotCell, preampCell)
	scaleCol := container.NewVBox(
		layout.NewSpacer(),
		makeDBScaleColumnFixed(bandHeight),
		layout.NewSpacer(),
	)
	preCol := container.NewVBox(preampRow)
	bandsGrid := container.NewGridWithColumns(len(a.eqBandSliders), bandCols...)
	labelsGrid := container.NewGridWithColumns(len(a.eqBandSliders), freqLabels...)
	centerArea := container.NewVBox(bandsGrid, labelsGrid)
	leftBox := container.NewHBox(preCol, widget.NewSeparator(), scaleCol, widget.NewSeparator())
	return container.NewBorder(nil, nil, leftBox, nil, centerArea)
}

// buildEQPresetsSection assembles the preset dropdown and name entry.
// buildEQPresetsSection assembles the preset dropdown and name entry field.
func (a *App) buildEQPresetsSection(eqModel *eqmodel.EQModel) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Preset name")
	a.eqNameEntry = nameEntry

	options := a.eq.PresetNamesWithFlat(a.eqCustomNames)
	presetSel := widget.NewSelect(options, func(name string) {
		if a.silentUpdating {
			return
		}
		_ = a.eq.ApplyPresetName(a.player, name, a.eqCustomMap)
		a.applySelectedPreset(name)
	})
	a.eqDrawerPreset = presetSel
	presetSel.PlaceHolder = EQNameFlat

	activeName := EQNameFlat
	if idx := a.currentPresetIndex(); idx >= 0 && idx < len(a.config.Presets) {
		if s := strings.TrimSpace(a.config.Presets[idx].EQ); s != "" {
			activeName = s
		}
	}
	if strings.EqualFold(activeName, EQNameOff) {
		activeName = EQNameFlat
	}
	a.silentUpdating = true
	presetSel.SetSelected(activeName)
	a.silentUpdating = false
	_ = a.eq.ApplyPresetName(a.player, activeName, a.eqCustomMap)
	a.applySelectedPreset(activeName)

	return container.NewGridWithColumns(2, presetSel, nameEntry)
}

// buildEQButtonsSection renders Save/Delete controls for custom presets.
// buildEQButtonsSection renders Save/Delete controls for custom presets.
func (a *App) buildEQButtonsSection(eqModel *eqmodel.EQModel) fyne.CanvasObject {
	saveBtn := widget.NewButton("Save Preset As…", func() {
		if a.eqNameEntry == nil {
			return
		}
		n := strings.TrimSpace(a.eqNameEntry.Text)
		if n == "" {
			return
		}
		preset := eqmodel.ExtractPresetFromSliders(a.eqSliderValues)
		preset.Name = n
		data := presetToConfigData(preset)
		if idx, ok := a.customEQIndex[n]; ok {
			a.config.CustomEQPresets[idx] = data
		} else {
			a.config.CustomEQPresets = append(a.config.CustomEQPresets, data)
			a.rebuildCustomEQIndex()
		}
		_ = a.config.Save()
		a.rebuildCustomPresetLists()
		a.refreshEQPresetOptions(n)
		if a.eqDrawerPreset != nil {
			a.silentUpdating = true
			a.eqDrawerPreset.SetSelected(n)
			a.silentUpdating = false
		}
		a.applySelectedPreset(n)
	})
	saveBtn.Disable()
	a.eqSaveButton = saveBtn

	delBtn := widget.NewButton("Delete", func() {
		if a.eqDrawerPreset == nil {
			return
		}
		sel := a.eqDrawerPreset.Selected
		if idx, ok := a.customEQIndex[sel]; ok {
			a.config.CustomEQPresets = append(a.config.CustomEQPresets[:idx], a.config.CustomEQPresets[idx+1:]...)
			a.rebuildCustomEQIndex()
			_ = a.config.Save()
			a.rebuildCustomPresetLists()
			a.refreshEQPresetOptions(EQNameFlat)
			if a.eqDrawerPreset != nil {
				a.silentUpdating = true
				a.eqDrawerPreset.SetSelected(EQNameFlat)
				a.silentUpdating = false
			}
			_ = a.eq.ApplyPresetName(a.player, EQNameFlat, a.eqCustomMap)
			a.applySelectedPreset(EQNameFlat)
			for i := range a.eqPresetSelects {
				if selw := a.eqPresetSelects[i]; selw != nil {
					if strings.EqualFold(strings.TrimSpace(selw.Selected), strings.TrimSpace(sel)) {
						a.silentUpdating = true
						selw.SetSelected(EQNameFlat)
						a.silentUpdating = false
					}
				}
			}
			if idxActive := a.currentPresetIndex(); idxActive >= 0 && idxActive < len(a.config.Presets) {
				if strings.EqualFold(strings.TrimSpace(a.config.Presets[idxActive].EQ), strings.TrimSpace(sel)) {
					a.config.Presets[idxActive].EQ = EQNameFlat
					_ = a.config.Save()
					_ = a.eq.ApplyPresetName(a.player, EQNameFlat, a.eqCustomMap)
				}
			}
		}
	})
	delBtn.Disable()
	a.eqDeleteButton = delBtn

	if a.eqDrawerPreset != nil {
		a.updateEQButtonsForSelection(a.eqDrawerPreset.Selected)
	}

	return container.NewHBox(saveBtn, delBtn)
}

// applySelectedPreset updates sliders, buttons, and the VLC equalizer when a
// preset is chosen from the dropdown.
func (a *App) applySelectedPreset(name string) {
	if a.eqModel == nil {
		return
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = EQNameFlat
	}
	bands := len(a.eqSliderValues) - 1
	if bands < 0 {
		bands = 0
	}
	output := eqmodel.EQPreset{Name: cleanName, Gains: make([]float64, bands)}
	if preset, ok := eqmodel.FindPresetByName(cleanName); ok && !strings.EqualFold(cleanName, EQNameManual) {
		output = ensurePresetLength(preset, bands)
	} else {
		switch {
		case strings.EqualFold(cleanName, EQNameFlat):
			output.Preamp = 0
		case strings.EqualFold(cleanName, EQNameManual):
			output.Preamp = a.eqModel.Current.Preamp
			copy(output.Gains, a.eqModel.Current.Gains)
		case func() bool {
			for k := range a.eqCustomMap {
				if strings.EqualFold(strings.TrimSpace(k), cleanName) {
					return true
				}
			}
			return false
		}():
			for k, v := range a.eqCustomMap {
				if strings.EqualFold(strings.TrimSpace(k), cleanName) {
					p := eqmodel.EQPreset{Name: cleanName, Preamp: float64(v.Preamp), Gains: make([]float64, bands)}
					for i := 0; i < len(p.Gains) && i < len(v.Bands); i++ {
						p.Gains[i] = float64(v.Bands[i])
					}
					output = p
					break
				}
			}
		default:
			if a.eq != nil && a.eq.eq != nil {
				if pv, err := a.eq.eq.PreampValue(); err == nil {
					output.Preamp = pv
				}
				for i := 0; i < len(output.Gains); i++ {
					if v, err := a.eq.eq.AmpValueAtIndex(uint(i)); err == nil {
						output.Gains[i] = v
					}
				}
			}
		}
	}
	a.eqModel.Current = output
	eqmodel.ApplyPresetToSliders(output, a.eqSliderValues)
	a.applySliderValuesToWidgets()
	a.updateEQButtonsForSelection(cleanName)

	idx := a.currentPresetIndex()
	if idx >= 0 && idx < len(a.config.Presets) {
		a.config.Presets[idx].EQ = cleanName
		_ = a.config.Save()
		if sel := a.eqPresetSelects[idx]; sel != nil {
			a.silentUpdating = true
			sel.SetSelected(cleanName)
			a.silentUpdating = false
		}
	}
}

// refreshEQPresetOptions rebuilds dropdown options after custom preset edits.
func (a *App) refreshEQPresetOptions(selected string) {
	drawerOpts := a.eq.PresetNamesWithFlat(a.eqCustomNames)
	if a.eqDrawerPreset != nil {
		a.eqDrawerPreset.Options = drawerOpts
		a.eqDrawerPreset.Refresh()
		if selected != "" {
			a.silentUpdating = true
			a.eqDrawerPreset.SetSelected(selected)
			a.silentUpdating = false
		}
	}
	settingsOpts := a.eq.PresetNamesForSettings(a.eqCustomNames)
	for i := range a.eqPresetSelects {
		if sel := a.eqPresetSelects[i]; sel != nil {
			sel.Options = settingsOpts
			sel.Refresh()
		}
	}
}

// updateEQSliderState mirrors slider values into the eqModel struct so it can
// be serialized or compared with presets.
func (a *App) updateEQSliderState(index int, value float64) {
	if index < 0 || index >= len(a.eqSliderValues) {
		return
	}
	a.eqSliderValues[index] = value
	if a.eqModel == nil {
		return
	}
	if index == 0 {
		a.eqModel.Current.Preamp = value
		return
	}
	if len(a.eqModel.Current.Gains) < len(a.eqSliderValues)-1 {
		gains := make([]float64, len(a.eqSliderValues)-1)
		copy(gains, a.eqModel.Current.Gains)
		a.eqModel.Current.Gains = gains
	}
	a.eqModel.Current.Gains[index-1] = value
}

// applySliderValuesToWidgets pushes stored eqModel values back into the slider
// widgets, used when presets change programmatically.
func (a *App) applySliderValuesToWidgets() {
	if a.eqPreampSlider == nil || len(a.eqSliderValues) == 0 {
		return
	}
	a.eqSlidersSilent = true
	a.eqPreampSlider.SetValue(a.eqSliderValues[0])
	for i, slider := range a.eqBandSliders {
		if slider != nil && i+1 < len(a.eqSliderValues) {
			slider.SetValue(a.eqSliderValues[i+1])
		}
	}
	a.eqSlidersSilent = false
}

// updateEQButtonsForSelection toggles Save/Delete buttons depending on whether
// the selection is manual or matches a custom preset.
func (a *App) updateEQButtonsForSelection(name string) {
	target := strings.TrimSpace(name)
	if a.eqDeleteButton != nil {
		_, ok := a.eqCustomMap[target]
		if !ok {
			for k := range a.eqCustomMap {
				if strings.EqualFold(strings.TrimSpace(k), target) {
					ok = true
					break
				}
			}
		}
		if ok {
			a.eqDeleteButton.Enable()
		} else {
			a.eqDeleteButton.Disable()
		}
	}
	if a.eqSaveButton != nil {
		if strings.EqualFold(target, EQNameManual) {
			a.eqSaveButton.Enable()
		} else {
			a.eqSaveButton.Disable()
		}
	}
}

// presetToConfigData converts an eqmodel preset into the config storage shape.
func presetToConfigData(p eqmodel.EQPreset) config.EQPresetData {
	data := config.EQPresetData{
		Name:   p.Name,
		Preamp: float32(p.Preamp),
		Bands:  make([]float32, len(p.Gains)),
	}
	for i := range p.Gains {
		data.Bands[i] = float32(p.Gains[i])
	}
	return data
}

// ensurePresetLength resizes gain arrays so they match the number of VLC bands.
func ensurePresetLength(p eqmodel.EQPreset, bands int) eqmodel.EQPreset {
	if bands <= 0 {
		p.Gains = []float64{}
		return p
	}
	if len(p.Gains) == bands {
		return p
	}
	gains := make([]float64, bands)
	copy(gains, p.Gains)
	p.Gains = gains
	return p
}

// hzLabel formats frequency like 32 64 125 250 500 1K 2K 4K 8K 16KHz
func hzLabel(f float64) string {
	if f >= 995 { // treat as KHz steps
		k := int(f/1000 + 0.5)
		if k >= 16 { // last value: append Hz
			return fmt.Sprintf("%dKHz", k)
		}
		return fmt.Sprintf("%dK", k)
	}
	return fmt.Sprintf("%d", int(f+0.5))
}

const (
	dbScaleColumnWidth     float32 = 56
	dbScaleTopLabelText            = "+15 dB"
	dbScaleMidLabelText            = "0 dB"
	dbScaleBottomLabelText         = "-15 dB"
)

// centerXInColumn computes the x-offset needed to center a label in a column.
func centerXInColumn(columnWidth float32, label *widget.Label) float32 {
	labelSize := label.MinSize()
	return (columnWidth - labelSize.Width) / 2
}

// nonNegativePosition clamps coordinates to stay within the column bounds.
func nonNegativePosition(position float32) float32 {
	if position < 0 {
		return 0
	}
	return position
}

// makeDBScaleColumnFixed renders a dB scale column of fixed visual height so it
// aligns with the vertical band sliders. Labels are placed proportionally:
// +15 dB at the top edge, 0 dB at mid-height (aligned with slider knob row),
// -15 dB at the bottom edge. The column width is compact.
func makeDBScaleColumnFixed(height float32) fyne.CanvasObject {
	top := widget.NewLabel(dbScaleTopLabelText)
	mid := widget.NewLabel(dbScaleMidLabelText)
	bot := widget.NewLabel(dbScaleBottomLabelText)

	// place inside a without-layout container so we can position labels manually
	bg := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	bg.SetMinSize(fyne.NewSize(dbScaleColumnWidth, height))
	lay := container.NewWithoutLayout(bg, top, mid, bot)

	// compute positions centered horizontally; use each label's min size
	topSize := top.MinSize()
	midSize := mid.MinSize()
	botSize := bot.MinSize()

	xTop := centerXInColumn(dbScaleColumnWidth, top)
	xMid := centerXInColumn(dbScaleColumnWidth, mid)
	xBot := centerXInColumn(dbScaleColumnWidth, bot)

	// vertical coords and corrections for label height
	yTop := float32(0) - 25
	yMid := nonNegativePosition(height/2 - midSize.Height/2 - 20)
	yBot := nonNegativePosition(height - botSize.Height - 10)

	top.Move(fyne.NewPos(xTop, yTop))
	top.Resize(topSize)

	mid.Move(fyne.NewPos(xMid, yMid))
	mid.Resize(midSize)

	bot.Move(fyne.NewPos(xBot, yBot))
	bot.Resize(botSize)

	// Wrap into a fixed-size grid cell so surrounding layout keeps our height
	wrap := container.New(layout.NewGridWrapLayout(fyne.NewSize(dbScaleColumnWidth, height)), lay)
	return wrap
}
