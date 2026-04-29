package workload

import (
	"testing"
	"time"
)

func TestRateDeltaMath(t *testing.T) {
	const sec = time.Second

	t.Run("monotonic delta computes rate", func(t *testing.T) {
		got := rateDelta(1500, 500, 0, sec)
		if got != 1000 {
			t.Errorf("expected 1000, got %v", got)
		}
	})

	t.Run("zero interval returns zero (no panic on divide)", func(t *testing.T) {
		got := rateDelta(100, 0, 0, 0)
		if got != 0 {
			t.Errorf("expected 0, got %v", got)
		}
	})

	t.Run("negative interval returns zero", func(t *testing.T) {
		got := rateDelta(100, 0, 0, -sec)
		if got != 0 {
			t.Errorf("expected 0, got %v", got)
		}
	})

	t.Run("counter wrap with bound recovers positive delta", func(t *testing.T) {
		// prev = max - 100; curr = 50; wrapBound = max.
		// True energy consumed since prev: (max - prev) + curr = 100 + 50 = 150.
		// Naive (curr - prev) on uint64 underflows to a huge positive value.
		const wrapBound uint64 = 1_000_000
		got := rateDelta(50, wrapBound-100, wrapBound, sec)
		if got != 150 {
			t.Errorf("expected 150, got %v (naive subtraction would underflow)", got)
		}
	})

	t.Run("counter wrap without bound returns zero", func(t *testing.T) {
		// curr < prev but no wrapBound supplied — caller has no way to
		// disambiguate "counter wrapped" from "counter reset"; emit zero
		// rather than guess. Pins the contract that wrap recovery is opt-in.
		got := rateDelta(50, 100, 0, sec)
		if got != 0 {
			t.Errorf("expected 0 when curr<prev and no wrapBound, got %v", got)
		}
	})
}
