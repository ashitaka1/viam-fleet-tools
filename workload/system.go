package workload

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
)

func sampleSystem(ctx context.Context, now time.Time) map[string]any {
	out := map[string]any{
		"timestamp": now.UTC().Format(time.RFC3339Nano),
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		out["uptime_sec"] = up
	}
	if avg, err := load.AvgWithContext(ctx); err == nil {
		out["load_avg_1"] = round2(avg.Load1)
		out["load_avg_5"] = round2(avg.Load5)
		out["load_avg_15"] = round2(avg.Load15)
	}
	return out
}
