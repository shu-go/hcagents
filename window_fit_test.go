package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/shu-go/hcagents/internal/config"
)

func TestFitBounds(t *testing.T) {
	area := application.Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	win := application.Rect{X: 100, Y: 500, Width: 280, Height: 300} // bottom at 800
	tests := []struct {
		name string
		b    application.Rect
		want int
		grow string
		out  application.Rect
	}{
		{"up grows", win, 400, config.GrowUp, application.Rect{X: 100, Y: 400, Width: 280, Height: 400}},
		{"up shrinks", win, 100, config.GrowUp, application.Rect{X: 100, Y: 700, Width: 280, Height: 100}},
		{"up stops at the top", win, 1000, config.GrowUp, application.Rect{X: 100, Y: 0, Width: 280, Height: 800}},
		{"down grows", win, 400, config.GrowDown, application.Rect{X: 100, Y: 500, Width: 280, Height: 400}},
		{"down shrinks", win, 100, config.GrowDown, application.Rect{X: 100, Y: 500, Width: 280, Height: 100}},
		{"down stops at the bottom", win, 1000, config.GrowDown, application.Rect{X: 100, Y: 500, Width: 280, Height: 540}},
		{"never below the minimum", win, 5, config.GrowDown, application.Rect{X: 100, Y: 500, Width: 280, Height: minFitHeight}},
		{"unchanged", win, 300, config.GrowUp, win},
	}
	for _, tt := range tests {
		if got := fitBounds(tt.b, area, tt.want, tt.grow); got != tt.out {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.out)
		}
	}
}
