package workload

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

func sampleNetwork(ctx context.Context, prev *Snapshot, now time.Time) (map[string]any, map[string]net.IOCountersStat) {
	stats, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return map[string]any{}, nil
	}
	curr := map[string]net.IOCountersStat{}
	for _, s := range stats {
		if s.Name == "lo" {
			continue
		}
		curr[s.Name] = s
	}
	out := map[string]any{}
	if prev == nil || prev.Nets == nil {
		return out, curr
	}
	dt := now.Sub(prev.Taken)
	rxMbps := map[string]any{}
	txMbps := map[string]any{}
	rxPps := map[string]any{}
	txPps := map[string]any{}
	for name, c := range curr {
		p, ok := prev.Nets[name]
		if !ok {
			continue
		}
		rxMbps[name] = round2(rateDelta(c.BytesRecv, p.BytesRecv, 0, dt) / mb)
		txMbps[name] = round2(rateDelta(c.BytesSent, p.BytesSent, 0, dt) / mb)
		rxPps[name] = round2(rateDelta(c.PacketsRecv, p.PacketsRecv, 0, dt))
		txPps[name] = round2(rateDelta(c.PacketsSent, p.PacketsSent, 0, dt))
	}
	if len(rxMbps) > 0 {
		out["net_rx_mbps"] = rxMbps
		out["net_tx_mbps"] = txMbps
		out["net_rx_pps"] = rxPps
		out["net_tx_pps"] = txPps
	}
	return out, curr
}
