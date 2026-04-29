package workload

import "testing"

func TestTopProcessesSelection(t *testing.T) {
	procs := []ProcessInfo{
		{Pid: 100, Name: "alpha", CPUPct: 10, MemMB: 200},
		{Pid: 200, Name: "bravo", CPUPct: 50, MemMB: 50},
		{Pid: 300, Name: "charlie", CPUPct: 30, MemMB: 800},
		{Pid: 400, Name: "delta", CPUPct: 80, MemMB: 100},
		{Pid: 500, Name: "echo", CPUPct: 5, MemMB: 1000},
	}

	t.Run("top 3 by CPU returns highest descending", func(t *testing.T) {
		got := selectTopByCPU(procs, 3)
		wantPids := []int32{400, 200, 300}
		if len(got) != 3 {
			t.Fatalf("expected 3 results, got %d", len(got))
		}
		for i, want := range wantPids {
			if got[i].Pid != want {
				t.Errorf("position %d: expected pid=%d, got pid=%d (proc=%+v)", i, want, got[i].Pid, got[i])
			}
		}
	})

	t.Run("top 3 by mem returns highest descending", func(t *testing.T) {
		got := selectTopByMem(procs, 3)
		wantPids := []int32{500, 300, 100}
		if len(got) != 3 {
			t.Fatalf("expected 3 results, got %d", len(got))
		}
		for i, want := range wantPids {
			if got[i].Pid != want {
				t.Errorf("position %d: expected pid=%d, got pid=%d", i, want, got[i].Pid)
			}
		}
	})

	t.Run("ties broken by smaller pid (pinned contract)", func(t *testing.T) {
		tied := []ProcessInfo{
			{Pid: 999, Name: "later", CPUPct: 50, MemMB: 0},
			{Pid: 100, Name: "earlier", CPUPct: 50, MemMB: 0},
			{Pid: 50, Name: "earliest", CPUPct: 30, MemMB: 0},
		}
		got := selectTopByCPU(tied, 2)
		if len(got) != 2 {
			t.Fatalf("expected 2 results, got %d", len(got))
		}
		if got[0].Pid != 100 || got[1].Pid != 999 {
			t.Errorf("expected tie order [100, 999] (smaller pid wins), got [%d, %d]", got[0].Pid, got[1].Pid)
		}
	})

	t.Run("n larger than input returns all", func(t *testing.T) {
		got := selectTopByCPU(procs, 99)
		if len(got) != len(procs) {
			t.Errorf("n > len(input): expected %d results, got %d", len(procs), len(got))
		}
	})

	t.Run("n=0 returns nil", func(t *testing.T) {
		if got := selectTopByCPU(procs, 0); got != nil {
			t.Errorf("expected nil for n=0, got %v", got)
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		if got := selectTopByCPU(nil, 5); got != nil {
			t.Errorf("expected nil for empty input, got %v", got)
		}
	})
}
