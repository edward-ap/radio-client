package ui

import (
	"math"
	"testing"
)

func TestNormalizeSliderValueClamp(t *testing.T) {
	tests := []struct {
		name  string
		min   float64
		max   float64
		step  float64
		value float64
		want  float64
	}{
		{name: "below min", min: 0, max: 10, step: 0, value: -5, want: 0},
		{name: "above max", min: 0, max: 10, step: 0, value: 25, want: 10},
		{name: "max <= min", min: 5, max: 5, step: 0, value: 7, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSliderValue(tt.min, tt.max, tt.step, tt.value)
			if got != tt.want {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestNormalizeSliderValueStep(t *testing.T) {
	tests := []struct {
		name  string
		min   float64
		max   float64
		step  float64
		value float64
		want  float64
	}{
		{name: "round negative towards grid", min: -10, max: 10, step: 2.5, value: -6.2, want: -5},
		{name: "round positive towards grid", min: -10, max: 10, step: 2.5, value: 6.1, want: 5},
		{name: "clamp after rounding", min: -10, max: 10, step: 2.5, value: 8.9, want: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSliderValue(tt.min, tt.max, tt.step, tt.value)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
		})
	}
}
