package equalizer

import "testing"

func TestDefaultPresetsCount(t *testing.T) {
	presets := DefaultPresets()
	const expected = 4
	if len(presets) != expected {
		t.Fatalf("expected %d presets, got %d", expected, len(presets))
	}
	for i, p := range presets {
		if p.Name == "" {
			t.Fatalf("preset %d has empty name", i)
		}
		if len(p.Gains) != 10 {
			t.Fatalf("preset %s expected 10 gains, got %d", p.Name, len(p.Gains))
		}
	}
}

func TestFindPresetByName(t *testing.T) {
	p, ok := FindPresetByName("bass boost")
	if !ok {
		t.Fatalf("expected to find Bass Boost preset")
	}
	if p.Preamp == 0 || p.Gains[0] == 0 {
		t.Fatalf("expected non-zero values for Bass Boost preset")
	}
	if _, ok := FindPresetByName("non-existent"); ok {
		t.Fatalf("unexpected preset found")
	}
}

func TestApplyPresetToSliders(t *testing.T) {
	p := EQPreset{
		Name:   "Test",
		Preamp: 3.5,
		Gains:  []float64{1, 2, 3},
	}
	sliders := make([]float64, len(p.Gains)+1)
	ApplyPresetToSliders(p, sliders)
	if sliders[0] != p.Preamp {
		t.Fatalf("preamp mismatch: want %.1f got %.1f", p.Preamp, sliders[0])
	}
	for i, g := range p.Gains {
		if sliders[i+1] != g {
			t.Fatalf("gain %d mismatch: want %.1f got %.1f", i, g, sliders[i+1])
		}
	}
}

func TestCustomPresetFromSliders(t *testing.T) {
	sliders := []float64{1.5, -2, -1, 0, 1}
	p := ExtractPresetFromSliders(sliders)
	if p.Preamp != sliders[0] {
		t.Fatalf("expected preamp %.1f got %.1f", sliders[0], p.Preamp)
	}
	if len(p.Gains) != len(sliders)-1 {
		t.Fatalf("expected %d gains got %d", len(sliders)-1, len(p.Gains))
	}
	for i, g := range p.Gains {
		if g != sliders[i+1] {
			t.Fatalf("gain %d mismatch: want %.1f got %.1f", i, sliders[i+1], g)
		}
	}
}
