package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func newTestGate(delay time.Duration) (*clickGate, *atomic.Int32, *atomic.Int32) {
	var singles, doubles atomic.Int32
	g := &clickGate{
		delay:  delay,
		single: func() { singles.Add(1) },
		double: func() { doubles.Add(1) },
	}
	return g, &singles, &doubles
}

func TestClickGateSingle(t *testing.T) {
	g, singles, _ := newTestGate(20 * time.Millisecond)
	g.click()
	if singles.Load() != 0 {
		t.Fatal("a click acted before the double-click time")
	}
	time.Sleep(80 * time.Millisecond)
	if singles.Load() != 1 {
		t.Errorf("singles = %d, want 1", singles.Load())
	}
}

func TestClickGateDouble(t *testing.T) {
	g, singles, doubles := newTestGate(20 * time.Millisecond)
	g.click() // Windows: click, double click, click
	g.doubleClick()
	g.click()
	time.Sleep(80 * time.Millisecond)
	if singles.Load() != 0 || doubles.Load() != 1 {
		t.Errorf("singles = %d, doubles = %d, want 0, 1", singles.Load(), doubles.Load())
	}
	// the next click is a click again
	g.click()
	time.Sleep(80 * time.Millisecond)
	if singles.Load() != 1 {
		t.Errorf("singles = %d, want 1", singles.Load())
	}
}

func TestClickGateNoDelay(t *testing.T) {
	g, singles, _ := newTestGate(0)
	g.click()
	if singles.Load() != 1 {
		t.Errorf("singles = %d, want 1 at once", singles.Load())
	}
}
