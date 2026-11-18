package config

import "testing"

func TestNormalizeOpacity(t *testing.T) {
	for in, want := range map[int]int{
		100: 100, 70: 70, 74: 70, 75: 80, 30: 30, 20: 30, 0: 30, -5: 30, 150: 100,
	} {
		if got := NormalizeOpacity(in); got != want {
			t.Errorf("NormalizeOpacity(%d) = %d, want %d", in, got, want)
		}
	}
}
