package workload

import (
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

type Snapshot struct {
	Taken   time.Time
	Disks   map[string]disk.IOCountersStat
	Nets    map[string]net.IOCountersStat
	RAPL    map[string]uint64
	PSwpIn  uint64
	PSwpOut uint64
}

// rateDelta returns (curr-prev)/seconds, recovering from counter wrap as
// (wrapBound-prev)+curr when curr<prev and a wrapBound is supplied. Naive
// uint64 subtraction would underflow into a giant positive spike. With no
// wrapBound, a backwards counter returns 0 rather than a guess.
func rateDelta(curr, prev, wrapBound uint64, interval time.Duration) float64 {
	if interval <= 0 {
		return 0
	}
	var diff uint64
	switch {
	case curr >= prev:
		diff = curr - prev
	case wrapBound > 0:
		diff = (wrapBound - prev) + curr
	default:
		return 0
	}
	return float64(diff) / interval.Seconds()
}
