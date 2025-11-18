// Package equalizer exposes helper types for EQ preset management, including
// cloning helpers used independently from the widget layer.
package equalizer

import "strings"

// EQPreset describes a group of gain values together with the preamp level.
// Gains holds the values for band sliders in order from low to high frequency.
type EQPreset struct {
	Name   string
	Gains  []float64
	Preamp float64
}

// EQModel keeps the available presets together with the current slider state.
type EQModel struct {
	Presets []EQPreset
	Current EQPreset
}

// Internal copy of the default presets. The values are intentionally simple:
// they serve as well-known shapes for tests (flat, bass, treble and vocal tilt).
var defaultPresets = []EQPreset{
	{
		Name:   "Flat",
		Preamp: 0,
		Gains:  []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
	{
		Name:   "Bass Boost",
		Preamp: 2,
		Gains:  []float64{5, 4, 3, 2, 1, 0, -1, -2, -3, -4},
	},
	{
		Name:   "Treble Boost",
		Preamp: 1,
		Gains:  []float64{-4, -3, -2, -1, 0, 1, 2, 3, 4, 5},
	},
	{
		Name:   "Vocal Boost",
		Preamp: 0,
		Gains:  []float64{-2, -1, 1, 3, 4, 3, 1, -1, -2, -3},
	},
}

// DefaultPresets returns a deep copy of bundled EQ presets so callers can safely
// modify individual entries without affecting the global defaults.
func DefaultPresets() []EQPreset {
	out := make([]EQPreset, len(defaultPresets))
	for i, p := range defaultPresets {
		out[i] = clonePreset(p)
	}
	return out
}

// FindPresetByName performs a case-insensitive lookup across default presets.
func FindPresetByName(name string) (EQPreset, bool) {
	for _, p := range defaultPresets {
		if strings.EqualFold(p.Name, name) {
			return clonePreset(p), true
		}
	}
	return EQPreset{}, false
}

// ApplyPresetToSliders writes the preset's values into the provided slider slice.
// Sliders use the following layout:
//   index 0    -> preamp value
//   indices 1+ -> band gain sliders, ordered from low to high frequency
// The slice is modified in place.
func ApplyPresetToSliders(p EQPreset, sliders []float64) {
	if len(sliders) == 0 {
		return
	}
	sliders[0] = p.Preamp
	bands := len(sliders) - 1
	for i := 0; i < bands; i++ {
		val := 0.0
		if i < len(p.Gains) {
			val = p.Gains[i]
		}
		sliders[i+1] = val
	}
}

// ExtractPresetFromSliders builds a preset from the current slider positions
// using the same layout that ApplyPresetToSliders expects.
func ExtractPresetFromSliders(sliders []float64) EQPreset {
	if len(sliders) == 0 {
		return EQPreset{}
	}
	out := EQPreset{
		Preamp: sliders[0],
		Gains:  make([]float64, len(sliders)-1),
	}
	copy(out.Gains, sliders[1:])
	return out
}

// clonePreset performs a deep copy so callers can mutate the clone without
// affecting stored presets.
func clonePreset(p EQPreset) EQPreset {
	clone := EQPreset{
		Name:   p.Name,
		Preamp: p.Preamp,
		Gains:  make([]float64, len(p.Gains)),
	}
	copy(clone.Gains, p.Gains)
	return clone
}
