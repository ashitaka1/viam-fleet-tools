package workload

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

func sampleDisk(ctx context.Context, prev *Snapshot, now time.Time) (map[string]any, map[string]disk.IOCountersStat) {
	curr, err := disk.IOCountersWithContext(ctx)
	if err != nil || len(curr) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if prev == nil || prev.Disks == nil {
		return out, curr
	}
	dt := now.Sub(prev.Taken)
	rxBps := map[string]any{}
	wxBps := map[string]any{}
	rxIops := map[string]any{}
	wxIops := map[string]any{}
	utilPct := map[string]any{}
	for name, c := range curr {
		p, ok := prev.Disks[name]
		if !ok {
			continue
		}
		rxBps[name] = round2(rateDelta(c.ReadBytes, p.ReadBytes, 0, dt) / mb)
		wxBps[name] = round2(rateDelta(c.WriteBytes, p.WriteBytes, 0, dt) / mb)
		rxIops[name] = round2(rateDelta(c.ReadCount, p.ReadCount, 0, dt))
		wxIops[name] = round2(rateDelta(c.WriteCount, p.WriteCount, 0, dt))
		busyMs := rateDelta(c.IoTime, p.IoTime, 0, dt)
		utilPct[name] = round2(clamp01(busyMs/1000.0) * 100.0)
	}
	if len(rxBps) > 0 {
		out["disk_read_mbps"] = rxBps
		out["disk_write_mbps"] = wxBps
		out["disk_read_iops"] = rxIops
		out["disk_write_iops"] = wxIops
		out["disk_util_pct"] = utilPct
	}
	return out, curr
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
