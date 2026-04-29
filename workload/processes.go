package workload

import (
	"context"
	"slices"

	"github.com/shirou/gopsutil/v4/process"
)

type ProcessInfo struct {
	Pid    int32
	Name   string
	CPUPct float64
	MemMB  float64
}

func selectTopByCPU(procs []ProcessInfo, n int) []ProcessInfo {
	return selectTopBy(procs, n, func(p ProcessInfo) float64 { return p.CPUPct })
}

func selectTopByMem(procs []ProcessInfo, n int) []ProcessInfo {
	return selectTopBy(procs, n, func(p ProcessInfo) float64 { return p.MemMB })
}

// selectTopBy returns the top n by field, descending, with smaller pid
// breaking ties — without that tiebreak top-N lists flicker poll-to-poll
// for processes with identical CPU/mem values.
func selectTopBy(procs []ProcessInfo, n int, field func(ProcessInfo) float64) []ProcessInfo {
	if n <= 0 || len(procs) == 0 {
		return nil
	}
	cp := slices.Clone(procs)
	slices.SortStableFunc(cp, func(a, b ProcessInfo) int {
		if fa, fb := field(a), field(b); fa != fb {
			if fa > fb {
				return -1
			}
			return 1
		}
		return int(a.Pid) - int(b.Pid)
	})
	if n > len(cp) {
		n = len(cp)
	}
	return cp[:n]
}

func processInfoMaps(procs []ProcessInfo) []any {
	out := make([]any, len(procs))
	for i, p := range procs {
		out[i] = map[string]any{
			"pid":     p.Pid,
			"name":    p.Name,
			"cpu_pct": round2(p.CPUPct),
			"mem_mb":  round2(p.MemMB),
		}
	}
	return out
}

func sampleTopProcesses(ctx context.Context, n int) map[string]any {
	if n <= 0 {
		return map[string]any{}
	}
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return map[string]any{}
	}
	infos := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		// Per-process errors (process exited mid-scan, permissions) are
		// expected and not actionable; skip rather than fail the sample.
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		cpuPct, err := p.CPUPercentWithContext(ctx)
		if err != nil {
			continue
		}
		memInfo, err := p.MemoryInfoWithContext(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, ProcessInfo{
			Pid:    p.Pid,
			Name:   name,
			CPUPct: cpuPct,
			MemMB:  bytesToMB(memInfo.RSS),
		})
	}
	return map[string]any{
		"top_procs_by_cpu": processInfoMaps(selectTopByCPU(infos, n)),
		"top_procs_by_mem": processInfoMaps(selectTopByMem(infos, n)),
	}
}
