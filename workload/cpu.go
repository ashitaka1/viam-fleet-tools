package workload

import (
	"context"

	"github.com/shirou/gopsutil/v4/cpu"
	"slices"
)

func sampleCPU(ctx context.Context) map[string]any {
	out := map[string]any{}

	if perCore, err := cpu.PercentWithContext(ctx, 0, true); err == nil && len(perCore) > 0 {
		out["cpu_pct_per_core"] = floatsRound2ToAny(perCore)
		out["cpu_pct_max_core"] = round2(slices.Max(perCore))
		out["cpu_pct_avg"] = round2(meanFloat(perCore))
	}
	if freqs, err := cpu.InfoWithContext(ctx); err == nil && len(freqs) > 0 {
		mhzs := make([]float64, 0, len(freqs))
		for _, f := range freqs {
			if f.Mhz > 0 {
				mhzs = append(mhzs, f.Mhz)
			}
		}
		if len(mhzs) > 0 {
			out["cpu_freq_mhz_avg"] = round2(meanFloat(mhzs))
			out["cpu_freq_mhz_max"] = round2(slices.Max(mhzs))
		}
	}
	return out
}
