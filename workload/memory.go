package workload

import (
	"bufio"
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

const (
	pageSize uint64  = 4096
	mb       float64 = 1024 * 1024
)

func sampleMemory(ctx context.Context, prev *Snapshot, now time.Time) (map[string]any, swapCounters) {
	out := map[string]any{}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		out["mem_used_mb"] = math.Round(float64(vm.Used) / mb)
		out["mem_available_mb"] = math.Round(float64(vm.Available) / mb)
		out["mem_total_mb"] = math.Round(float64(vm.Total) / mb)
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		out["swap_used_mb"] = round2(float64(sw.Used) / mb)
	}

	swap := readVMStat()
	if prev != nil {
		dt := now.Sub(prev.Taken)
		out["swap_in_mb_per_sec"] = round2(rateDelta(swap.pswpin, prev.PSwpIn, 0, dt) * float64(pageSize) / mb)
		out["swap_out_mb_per_sec"] = round2(rateDelta(swap.pswpout, prev.PSwpOut, 0, dt) * float64(pageSize) / mb)
	}
	return out, swap
}

type swapCounters struct {
	pswpin  uint64
	pswpout uint64
}

func readVMStat() swapCounters {
	f, err := os.Open("/proc/vmstat")
	if err != nil {
		return swapCounters{}
	}
	defer f.Close()
	var s swapCounters
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, val, ok := splitFirstField(line)
		if !ok {
			continue
		}
		v, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "pswpin":
			s.pswpin = v
		case "pswpout":
			s.pswpout = v
		}
	}
	return s
}

func splitFirstField(line string) (key, val string, ok bool) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return "", "", false
	}
	return line[:idx], strings.TrimSpace(line[idx+1:]), true
}

func bytesToMB(b uint64) float64 {
	return float64(b) / mb
}
