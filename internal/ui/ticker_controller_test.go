package ui

import "testing"

func TestTickerNeedsScroll(t *testing.T) {
	tests := []struct {
		name          string
		textWidth     float32
		viewportWidth float32
		want          bool
	}{
		{name: "fits exactly", textWidth: 120, viewportWidth: 120, want: false},
		{name: "slightly bigger but within epsilon", textWidth: 100.3, viewportWidth: 100, want: false},
		{name: "clearly needs scroll", textWidth: 150, viewportWidth: 120, want: true},
		{name: "negative viewport treated as zero", textWidth: 1, viewportWidth: -5, want: true},
		{name: "zero text width", textWidth: 0, viewportWidth: 200, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tickerNeedsScroll(tt.textWidth, tt.viewportWidth)
			if got != tt.want {
				t.Fatalf("tickerNeedsScroll(%v, %v) = %v, want %v", tt.textWidth, tt.viewportWidth, got, tt.want)
			}
		})
	}
}
